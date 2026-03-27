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
	Context    string
	CreatedAt  time.Time
}

type ParsedLink struct {
	TargetType string
	TargetRef  string
	Display    string
	Context    string
}

var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

func ParseWikilinks(text string) []ParsedLink {
	matches := wikilinkRe.FindAllStringSubmatchIndex(text, -1)
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
