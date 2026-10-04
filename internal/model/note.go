package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type Note struct {
	ID          string
	Title       string
	Slug        string
	Body        string
	Tags        string
	Pinned      bool
	WorkspaceID string
	// Path is the note's file, relative to the vault.
	Path string
	// Rev identifies the file content the note was read from; a save that
	// carries a stale Rev is refused rather than overwrite an outside edit.
	Rev        string
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (n *Note) TagList() []string {
	if n.Tags == "" {
		return nil
	}
	parts := strings.Split(n.Tags, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func (n *Note) HasTag(tag string) bool {
	for _, t := range n.TagList() {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

var nonAlphanumeric = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// Slugify makes a lowercase slug of letters and digits joined by dashes.
// Accents are folded ("Café" is "cafe"); letters of other scripts stay
// ("中文", "мир"), so a title in any language has a slug.
func Slugify(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.TrimSpace(s)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	s = norm.NFC.String(b.String())
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func ValidateNoteTitle(title string) error {
	if title == "" {
		return fmt.Errorf("note title cannot be empty")
	}
	if len(title) > 200 {
		return fmt.Errorf("note title cannot exceed 200 characters")
	}
	return nil
}

var validSlug = regexp.MustCompile(`^[\p{Ll}\p{Lo}\p{Lm}\p{N}]+(-[\p{Ll}\p{Lo}\p{Lm}\p{N}]+)*$`)

func ValidateNoteSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("note slug cannot be empty")
	}
	if !validSlug.MatchString(slug) {
		return fmt.Errorf("note slug must be lowercase letters and digits joined by hyphens")
	}
	return nil
}

// FileName is the file name (without .md) for a note titled title, the way
// Obsidian names notes: the title itself, minus characters that file
// systems reject (/ \ : * ? " < > |), that break wikilinks ([ ] # ^ |), or
// that hide the file (a leading dot). It is "" when nothing is left.
func FileName(title string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|[]#^`, r) || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)
	name := strings.Join(strings.Fields(cleaned), " ")
	return strings.Trim(name, ". ")
}
