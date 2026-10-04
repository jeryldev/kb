package board

import (
	"unicode/utf8"

	"github.com/jeryldev/kb/internal/model"
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

	lead         string // up to three spaces before the list marker
	bullet       string // "-", "*", "+", or an ordered marker such as "1."
	prose        string
	trailing     []string
	idOnLastLine bool
	derived      bool // the id is not in the file; kb derived it
	fromBr       bool // the first line held more lines joined by <br>
	twin         bool // another card in the lane has its title, and no id
	raw          []string
	firstDirty   bool
	descDirty    bool
}

var (
	// itemRe reads a list item's first line as CommonMark does: up to three
	// spaces, a bullet or an ordered marker, then spaces or a tab (or
	// nothing, for an item whose text starts on the next line).
	// A box is a box only with a space after it: "[x]foo" is text.
	itemRe    = regexp.MustCompile(`^( {0,3})([-*+]|\d{1,9}[.)])(?:[ \t]+(?:\[(.)\](?:[ \t]|$))?(.*))?$`)
	blockIDRe = regexp.MustCompile(`\s\^([A-Za-z0-9-]+)\s*$`)
	// tokenRe matches one token kb keeps whole: a tag, an inline field, a
	// plugin date or time, a Tasks emoji (with its date, when it takes one).
	tokenPattern = `#` + tagChars + `+|\[[^\[\]:]+?::[^\]]*\]|@@?\{[^}]*\}|@\[\[[^\]]*\]\]|[🔺⏫🔼🔽⏬]|[📅⏳🛫➕✅❌]\s*\d{4}-\d{2}-\d{2}`
	trailingRe   = regexp.MustCompile(`(?:^|\s)(` + tokenPattern + `)\s*$`)
	tagRe        = regexp.MustCompile(`(?:^|[` + jsSpace + `])#(` + tagChars + `+)`)
	tagWordRe    = regexp.MustCompile(`^` + tagChars + `+`)
	jsSpaceRe    = regexp.MustCompile(`^[` + jsSpace + `]$`)
	notTagChar   = regexp.MustCompile(`[` + tagStop + `]+`)
	fieldRe      = regexp.MustCompile(`^\[([^\[\]:]+?)::\s*([^\]]*?)\s*\]$`)
)

const priorityTag = "priority/"

// tagStop is what ends a tag and tagChars what a tag can hold, as the
// plugin's tag parser (src/parsers/extensions/tag.ts) reads them: a space
// or any of this punctuation ends it, so #c++ is the tag "c".
//
// The spaces are JavaScript's \s, which the plugin's tests use: they take
// in the no-break space and the ideographic space, which Go's \s does not.
const (
	jsSpace  = `\t\n\v\f\r \x{00a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`
	tagStop  = jsSpace + `\x{2000}-\x{206F}\x{2E00}-\x{2E7F}'!"#$%&()*+,.:;<=>?@^\x60{|}~\[\]\\`
	tagChars = `[^` + tagStop + `]`
)

// firstLine splits a card's first line into its lead, marker, checkbox and
// text. A line that is not a list item (which the parser never hands kb)
// is all text, so a board kb misreads still cannot crash it.
func firstLine(line string) (lead, bullet, check, text string) {
	m := itemRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", "", strings.TrimSpace(line)
	}
	return m[1], m[2], m[3], m[4]
}

func (it *Item) decode() {
	lead, bullet, check, text := firstLine(it.raw[0])
	line := text
	it.lead, it.bullet = lead, bullet
	it.Check = ' '
	if check != "" {
		it.Check = []rune(check)[0]
	}
	// Older plugin versions joined a card's lines with <br> on one line.
	lines := strings.Split(text, "<br>")
	it.fromBr = len(lines) > 1
	for _, line := range it.raw[1:] {
		lines = append(lines, dedent(line))
	}
	// The card's id is the last block id the plugin reads in it (see
	// caret.go), which the plugin moves to the first line when it saves.
	// One that ends the card or its first line is not part of the text.
	if k, at, id := lastCaretID(lines); k >= 0 {
		it.ID = id
		if k == len(lines)-1 || k == 0 {
			lines[k] = strings.TrimRight(lines[k][:at], " \t")
			it.idOnLastLine = k > 0
		}
	}
	if m := pluginFirstLineID.FindStringSubmatchIndex(lines[0]); m != nil {
		if it.ID == "" {
			it.ID = lines[0][m[2]:m[3]]
		}
		lines[0] = lines[0][:m[0]]
	}
	text = lines[0]
	if it.fromBr {
		text = strings.TrimSpace(text)
	}
	desc := lines[1:]
	cardLines(lines, func(k int, l string) {
		if k > 0 {
			desc[k-1] = unescapeCarets(l)
		}
	})
	it.Description = strings.Join(desc, "\n")
	for {
		tok := trailingRe.FindStringSubmatchIndex(text)
		if tok == nil {
			break
		}
		it.trailing = append([]string{text[tok[2]:tok[3]]}, it.trailing...)
		text = text[:tok[0]]
	}
	it.prose = strings.TrimSpace(text)
	it.Title = unescape(it.prose)
	it.Tags = findTags(line)
}

// maskCode blanks out the code spans and wikilinks in s, byte for byte, so
// what is found in the result at an index is at that index in s, and a #
// or ^ inside them is not seen.
func maskCode(s string) string {
	b := []byte(s)
	blank := func(from, to int) {
		for i := from; i < to; i++ {
			b[i] = ' '
		}
	}
	for _, sp := range model.CodeSpans(s) {
		blank(sp[0], sp[1])
	}
	for _, m := range wikilinkSpan.FindAllStringIndex(s, -1) {
		blank(m[0], m[1])
	}
	return string(b)
}

// findTags are the #tags in s as the plugin reads them: not in code spans
// or wikilinks.
func findTags(s string) []string {
	var out []string
	for _, t := range tagRe.FindAllStringSubmatch(maskCode(s), -1) {
		out = append(out, t[1])
	}
	return out
}

// tagAt reports whether a tag starts at the # at index i of s (whose code
// and links maskCode has blanked), and the tag's word.
func tagAt(masked string, i int) (string, bool) {
	if masked[i] != '#' {
		return "", false
	}
	if i > 0 {
		prev, _ := utf8.DecodeLastRuneInString(masked[:i])
		if !jsSpaceRe.MatchString(string(prev)) {
			return "", false
		}
	}
	word := tagWordRe.FindString(masked[i+1:])
	return word, word != ""
}

// RawText is the card's text as the plugin holds it (its titleRaw): the
// first line after the checkbox and the continuation lines, dedented one
// level, without the block id.
func (it *Item) RawText() string {
	_, _, _, text := firstLine(it.raw[0])
	lines := []string{text}
	for _, l := range it.raw[1:] {
		lines = append(lines, dedent(l))
	}
	// The plugin leaves out a block id that ends the card, then a " ^id"
	// ending the first line.
	if k, at, _ := lastCaretID(lines); k == len(lines)-1 {
		lines[k] = lines[k][:at]
	}
	lines[0] = pluginFirstLineID.ReplaceAllString(strings.TrimRight(lines[0], " \t"), "")
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
	// Each # that would start a tag is judged by its own word; one in a
	// code span or wikilink is left alone, where a backslash is text.
	masked := maskCode(title)
	var b strings.Builder
	for i := 0; i < len(title); i++ {
		if word, ok := tagAt(masked, i); ok && !keep[word] {
			b.WriteByte('\\')
		}
		b.WriteByte(title[i])
	}
	it.prose = escapeCaretWord(b.String())
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
	// A label spelled as a priority tag is a priority (see SetPriority).
	for l := range want {
		if strings.HasPrefix(l, priorityTag) {
			delete(want, l)
		}
	}
	var kept []string
	for _, l := range order {
		if want[l] {
			kept = append(kept, l)
		}
	}
	order = kept
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

// NormalizeLabel turns a label into the tag kb writes for it: spaces and
// characters a tag cannot hold become dashes ("some label" is
// #some-label, "q&a" is #q-a).
func NormalizeLabel(l string) string {
	l = notTagChar.ReplaceAllString(strings.TrimPrefix(strings.TrimSpace(l), "#"), "-")
	return strings.Trim(l, "-")
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
	// Remove it where it is a tag (not in code or a link), with a space
	// beside it, last first so earlier indexes stay right.
	masked := maskCode(it.prose)
	for i := len(it.prose) - 1; i >= 0; i-- {
		word, ok := tagAt(masked, i)
		end := i + 1 + len(word)
		if !ok || word != tag {
			continue
		}
		start := i
		if start > 0 && it.prose[start-1] == ' ' {
			start--
		} else if end < len(it.prose) && it.prose[end] == ' ' {
			end++
		}
		it.prose = it.prose[:start] + it.prose[end:]
		masked = masked[:start] + masked[end:]
	}
	it.prose = strings.TrimSpace(it.prose)
	it.Title = unescape(it.prose)
}

// escapeCaretWord escapes a ^word ending s, which the plugin would take as
// the card's block id and drop from its text.
func escapeCaretWord(s string) string {
	return blockIDRe.ReplaceAllStringFunc(s, func(m string) string {
		i := strings.Index(m, "^")
		return m[:i] + `\` + m[i:]
	})
}

// unescape is a title as read: \# is #, and a \^ that kb escaped is ^.
func unescape(prose string) string {
	return unescapeCarets(strings.ReplaceAll(prose, `\#`, "#"))
}

func (it *Item) retag() {
	it.firstDirty = true
	it.Tags = findTags(it.prose + " " + strings.Join(it.trailing, " "))
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
		parts := []string{it.lead + bullet + " [" + string(it.Check) + "]"}
		if it.prose != "" {
			parts = append(parts, it.prose)
		}
		parts = append(parts, it.trailing...)
		parts = append(parts, "^"+it.ID)
		first = strings.Join(parts, " ")
	}
	lines := []string{first}
	switch {
	// Lines read from <br> are not lines of the file: an edited first
	// line writes them as description lines.
	case it.descDirty || (it.fromBr && it.firstDirty):
		if it.Description != "" {
			desc := append([]string{""}, strings.Split(it.Description, "\n")...)
			cardLines(desc, func(k int, l string) {
				if k > 0 {
					desc[k] = escapeCarets(l)
				}
			})
			for _, d := range desc[1:] {
				lines = append(lines, indent+d)
			}
		}
	case len(it.raw) > 1:
		rest := append([]string{}, it.raw[1:]...)
		if it.idOnLastLine && it.firstDirty {
			n := len(rest) - 1
			if at, _ := caretID(rest[n]); at >= 0 {
				rest[n] = strings.TrimRight(rest[n][:at], " \t")
			}
		}
		lines = append(lines, rest...)
	}
	return lines
}
