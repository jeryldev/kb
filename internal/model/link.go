package model

import (
	"regexp"
	"strings"
	"time"
)

type Link struct {
	ID         string
	SourceType string
	SourceID   string
	TargetType string
	TargetID   string
	// TargetRef is the link as written, e.g. "Some Note" for [[Some Note]].
	TargetRef string
	Context   string
	CreatedAt time.Time
}

type ParsedLink struct {
	TargetType string
	TargetRef  string
	Display    string
	Context    string
}

var wikilinkRe = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)

func ParseWikilinks(text string) []ParsedLink {
	matches := linkMatches(text)
	if matches == nil {
		return nil
	}

	var links []ParsedLink
	for _, match := range matches {
		inner := text[match[2]:match[3]]

		ref, _, display, hasDisplay := splitLink(inner)
		if !hasDisplay {
			display = inner
		}

		targetType := "note"
		if strings.HasPrefix(ref, "card:") {
			targetType = "card"
			ref = ref[5:]
		} else if strings.HasPrefix(ref, "board:") {
			targetType = "board"
			ref = ref[6:]
		}

		if display == inner && strings.Contains(inner, ":") {
			display = ref
		}

		lineStart := strings.LastIndex(text[:match[0]], "\n") + 1
		lineEnd := strings.Index(text[match[1]:], "\n")
		if lineEnd == -1 {
			lineEnd = len(text)
		} else {
			lineEnd += match[1]
		}
		context := text[lineStart:lineEnd]

		links = append(links, ParsedLink{
			TargetType: targetType,
			TargetRef:  ref,
			Display:    display,
			Context:    context,
		})
	}

	return links
}

// splitLink splits a wikilink's inside at its first pipe into target and
// display text. In a table the pipe is written \| so the table keeps its
// columns; sep is the pipe as written, to write it back the same way.
func splitLink(inner string) (target, sep, display string, hasDisplay bool) {
	i := strings.Index(inner, "|")
	if i < 0 {
		return inner, "", "", false
	}
	if i > 0 && inner[i-1] == '\\' {
		return inner[:i-1], `\|`, inner[i+1:], true
	}
	return inner[:i], "|", inner[i+1:], true
}

// Wikilink is one [[link]] or ![[embed]] in a text.
type Wikilink struct {
	Target     string
	Display    string
	HasDisplay bool
	// Embed is ![[...]]; File is an embed of a file other than a note
	// (![[diagram.png]]).
	Embed, File bool
}

// ReplaceLinks replaces each wikilink and embed in text, the "!" of an
// embed included, with what replace returns for it. Links never span
// lines, and links in code are left alone.
func ReplaceLinks(text string, replace func(Wikilink) string) string {
	var b strings.Builder
	last := 0
	for _, m := range wikilinkMatches(text) {
		target, _, display, hasDisplay := splitLink(text[m.inner[0]:m.inner[1]])
		b.WriteString(text[last:m.start])
		b.WriteString(replace(Wikilink{Target: target, Display: display, HasDisplay: hasDisplay, Embed: m.embed, File: m.file}))
		last = m.end
	}
	b.WriteString(text[last:])
	return b.String()
}

var (
	fenceRe   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	fileExtRe = regexp.MustCompile(`\.([A-Za-z0-9]+)$`)
)

// OpeningFence is the fence a line opens a code block with, or "". A
// backtick fence's info string cannot hold a backtick (CommonMark 4.5).
func OpeningFence(line string) string {
	m := fenceRe.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
	if m == nil || (m[1][0] == '`' && strings.Contains(m[2], "`")) {
		return ""
	}
	return m[1]
}

// ClosesFence reports whether line closes a block opened with fence: the
// same character, at least as many, and nothing after but spaces.
func ClosesFence(line, fence string) bool {
	m := fenceRe.FindStringSubmatch(strings.TrimRight(line, "\r\n"))
	return m != nil && m[1][0] == fence[0] && len(m[1]) >= len(fence) && strings.TrimSpace(m[2]) == ""
}

// escaped reports whether the byte at i is preceded by an odd number of
// backslashes.
func escaped(s string, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// CodeSpans are the byte ranges of a line's code spans: a run of backticks
// up to the next run of exactly as many (CommonMark 6.1).
func CodeSpans(line string) [][2]int {
	var out [][2]int
	run := func(i int) int {
		n := 0
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		return n
	}
	for i := 0; i < len(line); {
		n := run(i)
		if n == 0 {
			i++
			continue
		}
		// A backslash before a backtick makes it text, not an opener.
		if escaped(line, i) {
			i++
			continue
		}
		end := -1
		for j := i + n; j < len(line); {
			if m := run(j); m == 0 {
				j++
			} else if m == n {
				end = j + m
				break
			} else {
				j += m
			}
		}
		if end < 0 {
			i += n
			continue
		}
		out = append(out, [2]int{i, end})
		i = end
	}
	return out
}

type wikilinkMatch struct {
	start, end  int // the whole match, an embed's "!" included
	inner       [2]int
	embed, file bool
}

// linkMatches finds the wikilinks in text that are links, as Obsidian
// treats them: not inside inline code or a fenced code block, and not an
// embed of a file other than a note (![[diagram.png]]). Each is the
// [[...]] and its inside, as regexp submatch indexes.
func linkMatches(text string) [][]int {
	var out [][]int
	for _, m := range wikilinkMatches(text) {
		if !m.file {
			bracket := m.start
			if m.embed {
				bracket++
			}
			out = append(out, []int{bracket, m.end, m.inner[0], m.inner[1]})
		}
	}
	return out
}

var (
	// A footnote definition ([^1]: text) holds indented paragraphs as a
	// list item does.
	listItemRe   = regexp.MustCompile(`^ {0,3}(?:[-*+]|\d{1,9}[.)]|\[\^[^\]]+\]:)(?:[ \t]|$)`)
	listMarkerRe = regexp.MustCompile(`^(?:[-*+]|\d{1,9}[.)])(?:[ \t]|$)`)
	headingLine  = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]|$)`)
	breakLine    = regexp.MustCompile(`^ {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$`)
)

// indentWidth is a line's indent in columns, a tab reaching the next
// multiple of four, as CommonMark counts it.
func indentWidth(line string) int {
	w := 0
	for _, r := range line {
		switch r {
		case ' ':
			w++
		case '\t':
			w += 4 - w%4
		default:
			return w
		}
	}
	return w
}

// codeRanges are the byte ranges of text that are code, as CommonMark
// reads it: fenced blocks (inside a list item too, indented with it),
// indented code blocks (four columns after a blank line, outside a list),
// and code spans, which may run across the lines of one paragraph but not
// past a heading, a thematic break, a table row or a new list item.
func codeRanges(text string) [][2]int {
	var code [][2]int
	offset := 0
	fence, fenceStart := "", 0
	inList, afterBlank := false, true
	paraStart, paraEnd := -1, -1
	spans := func(from, to int) {
		for _, m := range CodeSpans(text[from:to]) {
			code = append(code, [2]int{from + m[0], from + m[1]})
		}
	}
	endParagraph := func() {
		if paraStart >= 0 {
			spans(paraStart, paraEnd)
		}
		paraStart = -1
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		indent := indentWidth(line)
		blank := strings.TrimSpace(line) == ""
		bare := strings.TrimRight(line, "\r\n")
		switch {
		case fence != "":
			if ClosesFence(line, fence) || (inList && ClosesFence(trimmed, fence)) {
				code = append(code, [2]int{fenceStart, offset + len(line)})
				fence = ""
			}
		case OpeningFence(line) != "" || (inList && indent > 3 && OpeningFence(trimmed) != ""):
			endParagraph()
			if indent == 0 {
				inList = false // a fence at the margin ends a list
			}
			fence, fenceStart = OpeningFence(trimmed), offset
		case blank:
			endParagraph()
		case indent >= 4 && afterBlank && !inList && paraStart < 0:
			code = append(code, [2]int{offset, offset + len(line)})
		case headingLine.MatchString(bare) || strings.HasPrefix(trimmed, "|"):
			// A heading or a table row is a block of its own line.
			endParagraph()
			spans(offset, offset+len(line))
		case breakLine.MatchString(bare):
			// A thematic break, or a setext heading's underline: either
			// way the paragraph ends.
			endParagraph()
		default:
			if listItemRe.MatchString(line) || (inList && listMarkerRe.MatchString(trimmed)) {
				endParagraph()
				inList = true
			} else if indent == 0 && afterBlank {
				inList = false
			}
			if paraStart < 0 {
				paraStart = offset
			}
			paraEnd = offset + len(line)
		}
		afterBlank = blank
		offset += len(line)
	}
	endParagraph()
	if fence != "" {
		code = append(code, [2]int{fenceStart, len(text)})
	}
	return code
}

// wikilinkMatches finds every wikilink and embed outside code.
func wikilinkMatches(text string) []wikilinkMatch {
	code := codeRanges(text)
	inCode := func(pos int) bool {
		for _, r := range code {
			if pos >= r[0] && pos < r[1] {
				return true
			}
		}
		return false
	}

	var out []wikilinkMatch
	for _, m := range wikilinkRe.FindAllStringSubmatchIndex(text, -1) {
		// A backslash before [[ makes it text.
		if inCode(m[0]) || (m[0] > 0 && text[m[0]-1] == '\\') {
			continue
		}
		wm := wikilinkMatch{start: m[0], end: m[1], inner: [2]int{m[2], m[3]}}
		if m[0] > 0 && text[m[0]-1] == '!' {
			wm.start, wm.embed = m[0]-1, true
			target, _, _ := strings.Cut(text[m[2]:m[3]], "|")
			target, _, _ = strings.Cut(target, "#")
			if ext := fileExtRe.FindStringSubmatch(strings.TrimSpace(target)); ext != nil && !strings.EqualFold(ext[1], "md") {
				wm.file = true
			}
		}
		out = append(out, wm)
	}
	return out
}

// RewriteWikilinks replaces the target of each [[target]] or
// [[target|display]] in text for which rewrite returns a new target and
// true, keeping the display text. It returns the new text and how many
// links changed.
func RewriteWikilinks(text string, rewrite func(target string) (string, bool)) (string, int) {
	changed := 0
	var b strings.Builder
	last := 0
	for _, m := range linkMatches(text) {
		target, sep, display, _ := splitLink(text[m[2]:m[3]])
		b.WriteString(text[last:m[0]])
		replacement, ok := rewrite(target)
		if ok {
			changed++
		} else {
			replacement = target
		}
		b.WriteString("[[" + replacement + sep + display + "]]")
		last = m[1]
	}
	b.WriteString(text[last:])
	return b.String(), changed
}
