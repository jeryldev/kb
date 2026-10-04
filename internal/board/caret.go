package board

import (
	"regexp"
	"strings"

	"github.com/jeryldev/kb/internal/model"
)

// The Kanban plugin reads a card's block id with a Markdown extension
// (src/parsers/extensions/blockid.ts): a "^" followed by characters that
// are not spaces, reaching the end of a line, anywhere in the card's
// paragraphs and with or without a space before it. The last one in the
// card wins. A code span, a wikilink or a backslash before the "^" keeps
// it text, as does a fenced code block.

var wikilinkSpan = regexp.MustCompile(`\[\[[^\]\n]*\]\]`)

// caretID finds the block id the plugin reads in one line of card text:
// the index of its "^" and the id, or -1.
func caretID(line string) (int, string) {
	line = strings.TrimRight(line, "\r")
	if !strings.Contains(line, "^") {
		return -1, ""
	}
	spans := model.CodeSpans(line)
	for _, m := range wikilinkSpan.FindAllStringIndex(line, -1) {
		spans = append(spans, [2]int{m[0], m[1]})
	}
	inSpan := func(i int) bool {
		for _, s := range spans {
			if i >= s[0] && i < s[1] {
				return true
			}
		}
		return false
	}
	for i := 0; i < len(line); i++ {
		if line[i] != '^' || inSpan(i) || (i > 0 && line[i-1] == '\\') {
			continue
		}
		rest := line[i+1:]
		if rest != "" && !strings.ContainsAny(rest, " \t") {
			return i, rest
		}
	}
	return -1, ""
}

// escapeCarets escapes every "^" the plugin would read as a block id in a
// line kb writes, so the line stays text: "e = mc^2" is written
// "e = mc\^2", which Markdown shows as "e = mc^2".
func escapeCarets(line string) string {
	for {
		i, _ := caretID(line)
		if i < 0 {
			return line
		}
		line = line[:i] + `\` + line[i:]
	}
}

// unescapeCarets undoes escapeCarets: a "\^" in the last word of a line
// reads as "^".
func unescapeCarets(line string) string {
	start := strings.LastIndexAny(line, " \t") + 1
	word := line[start:]
	if !strings.Contains(word, `\^`) {
		return line
	}
	return line[:start] + strings.ReplaceAll(word, `\^`, "^")
}

// cardLines visits the lines of a card's text that are paragraph text,
// not fenced code: the first line, then the description's.
func cardLines(lines []string, visit func(k int, line string)) {
	fence := ""
	for k, l := range lines {
		if k > 0 {
			if fence != "" {
				if model.ClosesFence(l, fence) {
					fence = ""
				}
				continue
			}
			if f := model.OpeningFence(l); f != "" {
				fence = f
				continue
			}
		}
		visit(k, l)
	}
}

// lastCaretID is the line, index and id of the block id the plugin reads
// from a card's lines: the last one.
func lastCaretID(lines []string) (line, at int, id string) {
	line, at = -1, -1
	cardLines(lines, func(k int, l string) {
		if i, v := caretID(l); i >= 0 {
			line, at, id = k, i, v
		}
	})
	return line, at, id
}

// pluginFirstLineID is the plugin's removeBlockId: it drops a trailing
// " ^id" from the first line of the text it holds.
var pluginFirstLineID = regexp.MustCompile(`\s+\^([a-zA-Z0-9-]+)$`)
