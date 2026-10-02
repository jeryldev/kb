package board

import (
	"regexp"
	"strings"
)

// Item is one card. Its first line is kept as three parts, so that kb can
// change one without disturbing the others:
//
//   - [ ] Fix #ui login on the web app #bug #priority/high ^889962bb
//     └──────── prose ────────────┘ └── trailing ─────┘ └─ id ─┘
//
// The prose is the title as written, tags in the middle of a sentence
// included. The trailing tokens are the run of tags, inline fields, dates
// and Tasks emoji at the end of the line, in their order.
type Item struct {
	ID    string
	Check rune
	// Title is the prose with escapes removed (\# reads as #).
	Title string
	// Tags are every #tag on the first line, as the plugin reads them,
	// including the priority tag; Labels leaves that out.
	Tags        []string
	Description string

	bullet       string // "-", "*" or "+"
	prose        string
	trailing     []string
	idOnLastLine bool
	raw          []string
	firstDirty   bool
	descDirty    bool
}

var (
	itemRe    = regexp.MustCompile(`^([-*+]) (?:\[(.)\] ?)?(.*)$`)
	blockIDRe = regexp.MustCompile(`\s\^([A-Za-z0-9-]+)\s*$`)
	// tokenRe matches one token kb keeps whole: a tag, an inline field, a
	// plugin date or time, a Tasks emoji (with its date, when it takes one).
	tokenPattern = `#[^\s#\[\]]+|\[[^\[\]:]+?::[^\]]*\]|@@?\{[^}]*\}|@\[\[[^\]]*\]\]|[🔺⏫🔼🔽⏬]|[📅⏳🛫➕✅❌]\s*\d{4}-\d{2}-\d{2}`
	trailingRe   = regexp.MustCompile(`(?:^|\s)(` + tokenPattern + `)\s*$`)
	tagRe        = regexp.MustCompile(`(?:^|\s)#([^\s#\[\]]+)`)
	fieldRe      = regexp.MustCompile(`^\[([^\[\]:]+?)::\s*([^\]]*?)\s*\]$`)
	bareHashRe   = regexp.MustCompile(`(^|[^\\])#([^\s#])`)
)

const priorityTag = "priority/"

func (it *Item) decode() {
	m := itemRe.FindStringSubmatch(it.raw[0])
	it.bullet = m[1]
	it.Check = ' '
	if m[2] != "" {
		it.Check = []rune(m[2])[0]
	}
	text := m[3]
	if id := blockIDRe.FindStringSubmatch(text); id != nil {
		it.ID = id[1]
		text = text[:len(text)-len(id[0])]
	}
	for {
		tok := trailingRe.FindStringSubmatchIndex(text)
		if tok == nil {
			break
		}
		it.trailing = append([]string{text[tok[2]:tok[3]]}, it.trailing...)
		text = text[:tok[0]]
	}
	it.prose = strings.TrimSpace(text)
	it.Title = strings.ReplaceAll(it.prose, `\#`, "#")
	for _, t := range tagRe.FindAllStringSubmatch(m[3], -1) {
		it.Tags = append(it.Tags, t[1])
	}

	var desc []string
	for _, line := range it.raw[1:] {
		desc = append(desc, dedent(line))
	}
	// The plugin also takes a block id from the end of the card's last line
	// (and moves it to the first line when it saves).
	if it.ID == "" && len(desc) > 0 {
		last := desc[len(desc)-1]
		if id := blockIDRe.FindStringSubmatch(last); id != nil {
			it.ID = id[1]
			it.idOnLastLine = true
			desc[len(desc)-1] = last[:len(last)-len(id[0])]
		}
	}
	it.Description = strings.Join(desc, "\n")
}

// RawText is the card's text as the plugin holds it (its titleRaw): the
// first line after the checkbox and the continuation lines, dedented one
// level, without the block id.
func (it *Item) RawText() string {
	m := itemRe.FindStringSubmatch(it.raw[0])
	lines := []string{m[3]}
	for _, l := range it.raw[1:] {
		lines = append(lines, dedent(l))
	}
	if blockIDRe.MatchString(lines[0]) {
		lines[0] = blockIDRe.ReplaceAllString(lines[0], "")
	} else if n := len(lines) - 1; n > 0 {
		lines[n] = blockIDRe.ReplaceAllString(lines[n], "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// SetTitle replaces the prose and keeps everything else on the line. A #
// in the new title is escaped (\#) so it does not become a tag, unless it
// spells one of the card's labels, which keeps tags the title already had
// in the middle of a sentence.
func (it *Item) SetTitle(title string) {
	keep := map[string]bool{}
	for _, t := range it.Tags {
		keep[t] = true
	}
	it.prose = bareHashRe.ReplaceAllStringFunc(title, func(m string) string {
		sub := bareHashRe.FindStringSubmatch(m)
		rest := title[strings.Index(title, m)+len(sub[1])+1:]
		word := rest
		if i := strings.IndexAny(rest, " \t#[]"); i >= 0 {
			word = rest[:i]
		}
		if keep[word] {
			return m
		}
		return sub[1] + `\#` + sub[2]
	})
	it.Title = title
	it.firstDirty = true
}

// Labels are the card's tags other than its priority.
func (it *Item) Labels() []string {
	var out []string
	for _, t := range it.Tags {
		if !strings.HasPrefix(t, priorityTag) {
			out = append(out, t)
		}
	}
	return out
}

// SetLabels makes the card's labels exactly these. A label is written as a
// #tag, so spaces become dashes ("some label" is #some-label). Labels it no
// longer has are removed wherever they are on the line; new ones go after
// the existing trailing labels, before the priority tag and other tokens.
func (it *Item) SetLabels(labels []string) {
	want := map[string]bool{}
	var order []string
	for _, l := range labels {
		l = NormalizeLabel(l)
		if l != "" && !want[l] {
			want[l] = true
			order = append(order, l)
		}
	}
	have := map[string]bool{}
	for _, t := range it.Labels() {
		have[t] = true
		if !want[t] {
			it.removeTag(t)
		}
	}
	insertAt := 0
	for i, tok := range it.trailing {
		if strings.HasPrefix(tok, "#") && !strings.HasPrefix(tok, "#"+priorityTag) {
			insertAt = i + 1
		}
	}
	var added []string
	for _, l := range order {
		if !have[l] {
			added = append(added, "#"+l)
		}
	}
	it.trailing = append(it.trailing[:insertAt], append(added, it.trailing[insertAt:]...)...)
	it.retag()
}

// NormalizeLabel turns a label into the tag kb writes for it.
func NormalizeLabel(l string) string {
	l = strings.TrimPrefix(strings.TrimSpace(l), "#")
	return strings.Join(strings.Fields(l), "-")
}

// Priority is "urgent", "high", "medium" or "low": from a #priority/ tag,
// else a [priority:: …] field or Tasks emoji (as the Tasks plugin writes),
// else medium.
func (it *Item) Priority() string {
	for _, t := range it.Tags {
		if strings.HasPrefix(t, priorityTag) {
			return strings.TrimPrefix(t, priorityTag)
		}
	}
	if v := it.Field("priority"); v != "" {
		switch v {
		case "highest":
			return "urgent"
		case "lowest":
			return "low"
		}
		return v
	}
	for _, tok := range it.trailing {
		switch tok {
		case "🔺":
			return "urgent"
		case "⏫":
			return "high"
		case "🔼":
			return "medium"
		case "🔽", "⏬":
			return "low"
		}
	}
	return "medium"
}

// SetPriority writes the priority as a #priority/ tag, replacing any other
// way the card stated one. Medium is the default and is not written.
func (it *Item) SetPriority(p string) {
	for _, t := range it.Tags {
		if strings.HasPrefix(t, priorityTag) {
			it.removeTag(t)
		}
	}
	var kept []string
	for _, tok := range it.trailing {
		if strings.ContainsAny(tok, "🔺⏫🔼🔽⏬") || fieldKey(tok) == "priority" {
			continue
		}
		kept = append(kept, tok)
	}
	it.trailing = kept
	if p != "" && p != "medium" {
		it.trailing = append(it.trailing, "#"+priorityTag+p)
	}
	it.retag()
}

// Field is the value of an inline [key:: value] field on the card.
func (it *Item) Field(key string) string {
	for _, tok := range it.trailing {
		if m := fieldRe.FindStringSubmatch(tok); m != nil && strings.EqualFold(strings.TrimSpace(m[1]), key) {
			return m[2]
		}
	}
	return ""
}

// SetField sets an inline field; an empty value removes it.
func (it *Item) SetField(key, value string) {
	it.firstDirty = true
	for i, tok := range it.trailing {
		if fieldKey(tok) == strings.ToLower(key) {
			if value == "" {
				it.trailing = append(it.trailing[:i], it.trailing[i+1:]...)
			} else {
				it.trailing[i] = "[" + key + ":: " + value + "]"
			}
			return
		}
	}
	if value != "" {
		it.trailing = append(it.trailing, "["+key+":: "+value+"]")
	}
}

func fieldKey(tok string) string {
	if m := fieldRe.FindStringSubmatch(tok); m != nil {
		return strings.ToLower(strings.TrimSpace(m[1]))
	}
	return ""
}

// SetDescription replaces the card's continuation lines.
func (it *Item) SetDescription(d string) {
	it.Description = d
	it.descDirty = true
	it.firstDirty = it.firstDirty || it.idOnLastLine
}

// removeTag drops a tag from the prose and the trailing tokens.
func (it *Item) removeTag(tag string) {
	var kept []string
	for _, tok := range it.trailing {
		if tok != "#"+tag {
			kept = append(kept, tok)
		}
	}
	it.trailing = kept
	re := regexp.MustCompile(`(^|\s)#` + regexp.QuoteMeta(tag) + `(\s|$)`)
	it.prose = strings.TrimSpace(re.ReplaceAllString(it.prose, "$1$2"))
	it.prose = strings.Join(strings.Fields(it.prose), " ")
	it.Title = strings.ReplaceAll(it.prose, `\#`, "#")
}

func (it *Item) retag() {
	it.firstDirty = true
	it.Tags = nil
	for _, t := range tagRe.FindAllStringSubmatch(it.prose+" "+strings.Join(it.trailing, " "), -1) {
		it.Tags = append(it.Tags, t[1])
	}
}

func (it *Item) dirty() bool { return it.firstDirty || it.descDirty }

// render writes the card. An unchanged card is its lines as read; an
// edited first line keeps the description lines as read unless they were
// edited too. indent is the board's continuation indent (a tab or four
// spaces).
func (it *Item) render(indent string) []string {
	if !it.dirty() {
		return it.raw
	}
	first := it.raw[0]
	if it.firstDirty {
		bullet := it.bullet
		if bullet == "" {
			bullet = "-"
		}
		parts := []string{bullet + " [" + string(it.Check) + "]"}
		if it.prose != "" {
			parts = append(parts, it.prose)
		}
		parts = append(parts, it.trailing...)
		parts = append(parts, "^"+it.ID)
		first = strings.Join(parts, " ")
	}
	lines := []string{first}
	switch {
	case it.descDirty:
		if it.Description != "" {
			for _, d := range strings.Split(it.Description, "\n") {
				lines = append(lines, indent+d)
			}
		}
	case len(it.raw) > 1:
		rest := append([]string{}, it.raw[1:]...)
		if it.idOnLastLine && it.firstDirty {
			n := len(rest) - 1
			rest[n] = blockIDRe.ReplaceAllString(rest[n], "")
		}
		lines = append(lines, rest...)
	}
	return lines
}
