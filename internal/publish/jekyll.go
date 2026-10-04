package publish

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jeryldev/kb/internal/model"
	"gopkg.in/yaml.v3"
)

// NoteResolver finds the note a wikilink names, by any name it could use.
type NoteResolver interface {
	ResolveNoteRef(ref string) (*model.Note, error)
}

func JekyllFileName(slug string, date time.Time) string {
	return fmt.Sprintf("%s-%s.md", date.Format("2006-01-02"), slug)
}

// DefaultPermalink is the permalink pattern of a target that sets none.
const DefaultPermalink = "/blog/:year/:month/:day/:title/"

var permalinkToken = regexp.MustCompile(`:[a-z_]+`)

// ValidatePermalink refuses a pattern kb cannot fill in: kb expands only
// :year, :month, :day and :title, so a Jekyll style name ("pretty") or
// another placeholder (:categories) would give links that go nowhere.
func ValidatePermalink(pattern string) error {
	if pattern == "" {
		return nil
	}
	if !strings.HasPrefix(pattern, "/") {
		return fmt.Errorf("permalink %q must start with / (kb does not know Jekyll's named styles; spell the pattern out, such as /:year/:month/:day/:title/)", pattern)
	}
	for _, tok := range permalinkToken.FindAllString(pattern, -1) {
		switch tok {
		case ":year", ":month", ":day", ":title":
		default:
			return fmt.Errorf("permalink %q uses %s; kb fills in only :year, :month, :day and :title", pattern, tok)
		}
	}
	return nil
}

// ExpandPermalink fills in a Jekyll permalink pattern's :year, :month,
// :day and :title for a post.
func ExpandPermalink(pattern, slug string, date time.Time) string {
	if pattern == "" {
		pattern = DefaultPermalink
	}
	return strings.NewReplacer(
		":year", date.Format("2006"),
		":month", date.Format("01"),
		":day", date.Format("02"),
		":title", slug,
	).Replace(pattern)
}

func GenerateFrontMatter(note *model.Note, date time.Time, draft bool) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("layout: post\n")
	fmt.Fprintf(&b, "title: %s\n", yamlQuote(note.Title))
	fmt.Fprintf(&b, "date: %s\n", date.Format("2006-01-02"))

	if note.Tags != "" {
		var tags []string
		for _, t := range note.TagList() {
			if plainTag.MatchString(t) && readsAsText(t) {
				tags = append(tags, t)
			} else {
				tags = append(tags, yamlQuote(t))
			}
		}
		fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(tags, ", "))
	}

	// Front matter is plain text, so links become their words.
	excerpt := extractExcerpt(ResolveWikilinks(note.Body, nil, nil))
	if excerpt != "" {
		fmt.Fprintf(&b, "excerpt: %s\n", yamlQuote(excerpt))
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
	// Jekyll runs Liquid over a post, so a note holding {{ }} or {% %} (a
	// template, a code sample) would break the site's build or lose text.
	// raw keeps it text on every Jekyll; a note that itself closes a raw
	// block is left as written.
	if (strings.Contains(body, "{{") || strings.Contains(body, "{%")) && !strings.Contains(body, "endraw") {
		body = "{% raw %}" + body + "{% endraw %}"
	}
	return frontMatter + "\n" + body + "\n"
}

// ResolveWikilinks turns wikilinks into Markdown for the site: a link to a
// published note becomes a link to its post, and anything else becomes
// plain text (the display text, the note's title, or the link as written).
func ResolveWikilinks(body string, permalinks map[string]string, resolver NoteResolver) string {
	return model.ReplaceLinks(body, func(l model.Wikilink) string {
		target := strings.TrimSpace(l.Target)
		display := strings.TrimSpace(l.Display)
		hasDisplay := l.HasDisplay
		if l.File {
			// An embedded file (an image, a PDF) is not on the site: its name.
			name, _, _ := strings.Cut(target, "#")
			return filepath.Base(strings.TrimSpace(name))
		}

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
		name, heading, _ := strings.Cut(target, "#")
		name, heading = strings.TrimSpace(name), strings.TrimSpace(heading)
		if name == "" {
			// [[#Heading]] is a place in the same note.
			if hasDisplay && display != "" {
				return display
			}
			return heading
		}
		text := display
		if text == "" || !hasDisplay {
			text = name
			if note != nil {
				text = note.Title
			}
			// Obsidian shows a link to a heading as "Note > Heading".
			if heading != "" {
				text += " > " + heading
			}
		}
		if note != nil {
			if permalink, ok := permalinks[note.ID]; ok {
				if heading != "" {
					permalink += "#" + headingAnchor(heading)
				}
				return fmt.Sprintf("[%s](%s)", text, permalink)
			}
		}
		return text
	})
}

// PermalinkFor is the URL of the post written at path, read from its
// YYYY-MM-DD-slug.md file name, so links follow where a post really is.
func PermalinkFor(pattern, path string) (string, bool) {
	date, slug, ok := splitPostName(path)
	if !ok {
		return "", false
	}
	return ExpandPermalink(pattern, slug, date), true
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
	fence, comment := "", ""
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case fence != "":
			if model.ClosesFence(raw, fence) {
				fence = ""
			}
			continue
		case comment != "":
			// Inside an Obsidian %% comment %% or an HTML <!-- comment -->.
			if strings.Contains(line, comment) {
				comment = ""
			}
			continue
		case model.OpeningFence(raw) != "":
			fence = model.OpeningFence(raw)
			continue
		case strings.HasPrefix(line, "%%"):
			if !strings.Contains(line[2:], "%%") {
				comment = "%%"
			}
			continue
		case strings.HasPrefix(line, "<!--"):
			if !strings.Contains(line, "-->") {
				comment = "-->"
			}
			continue
		}
		indented := strings.HasPrefix(raw, "    ") || strings.HasPrefix(raw, "\t")
		if line == "" || indented || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "|") {
			continue
		}
		if runes := []rune(line); len(runes) > 200 {
			line = string(runes[:200]) + "..."
		}
		return line
	}
	return ""
}

// plainTag is a tag that needs no quotes in a flow list, unless YAML
// would read it as another value (see readsAsText).
var plainTag = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}_/.-]*$`)

// readsAsText reports whether YAML reads s unquoted as the text s, not a
// number, date, null or boolean. Jekyll parses with Ruby's YAML 1.1, where
// yes, no, on and off are booleans too.
func readsAsText(s string) bool {
	switch strings.ToLower(s) {
	case "y", "n", "yes", "no", "on", "off", "true", "false":
		return false
	}
	var v any
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		return false
	}
	str, ok := v.(string)
	return ok && str == s
}

// yamlQuote writes s as a YAML double-quoted scalar. Go's %q is close but
// not the same: YAML has no \x00-style escapes past \xFF and reads \a
// and others differently, so only what YAML defines is escaped.
func yamlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// headingAnchor is the id kramdown, Jekyll's Markdown converter, gives a
// heading: lowercase, with characters other than letters, digits, spaces
// and dashes dropped, and spaces made dashes.
func headingAnchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}
