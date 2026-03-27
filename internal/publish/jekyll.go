package publish

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jeryldev/kb/internal/model"
)

var wikilinkPattern = regexp.MustCompile(`\[\[([^\]|]+?)(?:\|([^\]]+?))?\]\]`)

type NoteResolver interface {
	GetNoteBySlug(slug string) (*model.Note, error)
}

func JekyllFileName(slug string, date time.Time) string {
	return fmt.Sprintf("%s-%s.md", date.Format("2006-01-02"), slug)
}

func JekyllPermalink(slug string, date time.Time) string {
	return fmt.Sprintf("/blog/%s/%s/", date.Format("2006/01/02"), slug)
}

func GenerateFrontMatter(note *model.Note, date time.Time, draft bool) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("layout: post\n")
	fmt.Fprintf(&b, "title: %q\n", note.Title)
	fmt.Fprintf(&b, "date: %s\n", date.Format("2006-01-02"))

	if note.Tags != "" {
		tags := note.TagList()
		fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(tags, ", "))
	}

	excerpt := extractExcerpt(note.Body)
	if excerpt != "" {
		fmt.Fprintf(&b, "excerpt: %q\n", excerpt)
	}

	if draft {
		b.WriteString("published: false\n")
	}

	b.WriteString("---\n")
	return b.String()
}

func GeneratePost(note *model.Note, date time.Time, draft bool, publishedSlugs map[string]string, resolver NoteResolver) string {
	frontMatter := GenerateFrontMatter(note, date, draft)
	body := ResolveWikilinks(note.Body, publishedSlugs, resolver)
	return frontMatter + "\n" + body + "\n"
}

func ResolveWikilinks(body string, publishedSlugs map[string]string, resolver NoteResolver) string {
	return wikilinkPattern.ReplaceAllStringFunc(body, func(match string) string {
		groups := wikilinkPattern.FindStringSubmatch(match)
		target := strings.TrimSpace(groups[1])
		displayText := ""
		if len(groups) > 2 {
			displayText = strings.TrimSpace(groups[2])
		}

		if strings.HasPrefix(target, "card:") || strings.HasPrefix(target, "board:") {
			if displayText != "" {
				return displayText
			}
			return strings.TrimPrefix(strings.TrimPrefix(target, "card:"), "board:")
		}

		title := displayText
		if title == "" && resolver != nil {
			note, err := resolver.GetNoteBySlug(target)
			if err == nil {
				title = note.Title
			}
		}
		if title == "" {
			title = target
		}

		if permalink, ok := publishedSlugs[target]; ok {
			return fmt.Sprintf("[%s](%s)", title, permalink)
		}

		return title
	})
}

func PostFilePath(postsDir, slug string, date time.Time) string {
	return filepath.Join(postsDir, JekyllFileName(slug, date))
}

func extractExcerpt(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	lines := strings.SplitN(body, "\n", 10)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "---") {
			continue
		}
		if len(line) > 200 {
			line = line[:200] + "..."
		}
		return line
	}
	return ""
}
