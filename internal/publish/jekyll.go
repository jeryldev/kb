package publish

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeryldev/kb/internal/model"
)

// NoteResolver finds the note a wikilink names, by any name it could use.
type NoteResolver interface {
	ResolveNoteRef(ref string) (*model.Note, error)
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

	// Front matter is plain text, so links become their words.
	excerpt := extractExcerpt(ResolveWikilinks(note.Body, nil, nil))
	if excerpt != "" {
		fmt.Fprintf(&b, "excerpt: %q\n", excerpt)
	}

	if draft {
		b.WriteString("published: false\n")
	}

	b.WriteString("---\n")
	return b.String()
}

// GeneratePost renders a note as a Jekyll post. permalinks maps the ids of
// notes already published to the same site to their post URLs.
func GeneratePost(note *model.Note, date time.Time, draft bool, permalinks map[string]string, resolver NoteResolver) string {
	frontMatter := GenerateFrontMatter(note, date, draft)
	body := ResolveWikilinks(note.Body, permalinks, resolver)
	return frontMatter + "\n" + body + "\n"
}

// ResolveWikilinks turns wikilinks into Markdown for the site: a link to a
// published note becomes a link to its post, and anything else becomes
// plain text (the display text, the note's title, or the link as written).
func ResolveWikilinks(body string, permalinks map[string]string, resolver NoteResolver) string {
	return model.ReplaceWikilinks(body, func(target, display string, hasDisplay bool) string {
		target = strings.TrimSpace(target)
		display = strings.TrimSpace(display)

		if strings.HasPrefix(target, "card:") || strings.HasPrefix(target, "board:") {
			if hasDisplay && display != "" {
				return display
			}
			return strings.TrimPrefix(strings.TrimPrefix(target, "card:"), "board:")
		}

		var note *model.Note
		if resolver != nil {
			note, _ = resolver.ResolveNoteRef(target)
		}
		text := display
		if text == "" && note != nil {
			text = note.Title
		}
		if text == "" {
			text = target
		}
		if note != nil {
			if permalink, ok := permalinks[note.ID]; ok {
				return fmt.Sprintf("[%s](%s)", text, permalink)
			}
		}
		return text
	})
}

// PermalinkFromPostPath is the URL of the post written at path, read from
// its YYYY-MM-DD-slug.md file name, so links follow where a post really is.
func PermalinkFromPostPath(path string) (string, bool) {
	date, slug, ok := splitPostName(path)
	if !ok {
		return "", false
	}
	return JekyllPermalink(slug, date), true
}

// PostDateFromPath is the date in a post's YYYY-MM-DD-slug.md file name.
func PostDateFromPath(path string) (time.Time, bool) {
	date, _, ok := splitPostName(path)
	return date, ok
}

func splitPostName(path string) (time.Time, string, bool) {
	base := strings.TrimSuffix(filepath.Base(path), ".md")
	if len(base) < 12 || base[10] != '-' {
		return time.Time{}, "", false
	}
	date, err := time.Parse(time.DateOnly, base[:10])
	if err != nil {
		return time.Time{}, "", false
	}
	return date, base[11:], true
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
