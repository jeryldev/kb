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

		ref := inner
		display := inner
		if idx := strings.Index(inner, "|"); idx != -1 {
			ref = inner[:idx]
			display = inner[idx+1:]
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

var markdownLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

func ExtractMarkdownLinks(text string) []ParsedLink {
	matches := markdownLinkRe.FindAllStringSubmatch(text, -1)
	if matches == nil {
		return nil
	}

	var links []ParsedLink
	for _, match := range matches {
		links = append(links, ParsedLink{
			TargetType: "url",
			TargetRef:  match[2],
			Display:    match[1],
		})
	}

	return links
}

// ReplaceWikilinks replaces each [[target]] or [[target|display]] in text
// with what replace returns for it. Links never span lines, and links in
// code or image embeds are left alone (see linkMatches).
func ReplaceWikilinks(text string, replace func(target, display string, hasDisplay bool) string) string {
	var b strings.Builder
	last := 0
	for _, m := range linkMatches(text) {
		target, display, hasDisplay := strings.Cut(text[m[2]:m[3]], "|")
		b.WriteString(text[last:m[0]])
		b.WriteString(replace(target, display, hasDisplay))
		last = m[1]
	}
	b.WriteString(text[last:])
	return b.String()
}

var (
	inlineCodeRe = regexp.MustCompile("`[^`\n]*`")
	fenceRe      = regexp.MustCompile("^ {0,3}(```|~~~)")
	fileExtRe    = regexp.MustCompile(`\.([A-Za-z0-9]+)$`)
)

// linkMatches finds the wikilinks in text that are links, as Obsidian
// treats them: not inside inline code or a fenced code block, and not an
// embed of a file other than a note (![[diagram.png]]).
func linkMatches(text string) [][]int {
	var code [][2]int
	offset := 0
	inFence := false
	fenceStart := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if fenceRe.MatchString(line) {
			if inFence {
				code = append(code, [2]int{fenceStart, offset + len(line)})
			} else {
				fenceStart = offset
			}
			inFence = !inFence
		} else if !inFence {
			for _, m := range inlineCodeRe.FindAllStringIndex(line, -1) {
				code = append(code, [2]int{offset + m[0], offset + m[1]})
			}
		}
		offset += len(line)
	}
	if inFence {
		code = append(code, [2]int{fenceStart, len(text)})
	}
	inCode := func(pos int) bool {
		for _, r := range code {
			if pos >= r[0] && pos < r[1] {
				return true
			}
		}
		return false
	}

	var out [][]int
	for _, m := range wikilinkRe.FindAllStringSubmatchIndex(text, -1) {
		if inCode(m[0]) {
			continue
		}
		if m[0] > 0 && text[m[0]-1] == '!' {
			target, _, _ := strings.Cut(text[m[2]:m[3]], "|")
			target, _, _ = strings.Cut(target, "#")
			if ext := fileExtRe.FindStringSubmatch(strings.TrimSpace(target)); ext != nil && !strings.EqualFold(ext[1], "md") {
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

// RewriteWikilinks replaces the target of each [[target]] or
// [[target|display]] in text for which rewrite returns a new target and
// true, keeping the display text. It returns the new text and how many
// links changed.
func RewriteWikilinks(text string, rewrite func(target string) (string, bool)) (string, int) {
	changed := 0
	out := ReplaceWikilinks(text, func(target, display string, hasDisplay bool) string {
		replacement, ok := rewrite(target)
		if !ok {
			replacement = target
		} else {
			changed++
		}
		if hasDisplay {
			return "[[" + replacement + "|" + display + "]]"
		}
		return "[[" + replacement + "]]"
	})
	return out, changed
}
