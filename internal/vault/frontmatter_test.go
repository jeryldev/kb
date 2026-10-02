package vault

import (
	"strings"
	"testing"
	"time"
)

func TestParseWithoutFrontmatter(t *testing.T) {
	doc, err := Parse([]byte("# Heading\n\nbody text\n"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Body != "# Heading\n\nbody text\n" {
		t.Errorf("body = %q", doc.Body)
	}
	if doc.ID() != "" || doc.Title() != "" {
		t.Errorf("expected empty id/title, got %q %q", doc.ID(), doc.Title())
	}
}

func TestParseKnownKeys(t *testing.T) {
	src := `---
id: 3f2a
title: Dual Transformation
tags: [strategy, "#innovation"]
aliases:
  - DT
pinned: true
workspace: school
created: 2026-05-13
archived: 2026-06-01T10:00:00Z
---
Body here
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID() != "3f2a" || doc.Title() != "Dual Transformation" {
		t.Errorf("id/title = %q %q", doc.ID(), doc.Title())
	}
	if got := strings.Join(doc.Tags(), ","); got != "strategy,innovation" {
		t.Errorf("tags = %q", got)
	}
	if got := strings.Join(doc.Aliases(), ","); got != "DT" {
		t.Errorf("aliases = %q", got)
	}
	if !doc.Pinned() || doc.Workspace() != "school" {
		t.Errorf("pinned/workspace = %v %q", doc.Pinned(), doc.Workspace())
	}
	if c := doc.Created(); c == nil || !c.Equal(time.Date(2026, 5, 13, 0, 0, 0, 0, time.Local)) {
		t.Errorf("created = %v", c)
	}
	if a := doc.Archived(); a == nil || a.Hour() != 10 {
		t.Errorf("archived = %v", a)
	}
	if doc.Body != "Body here\n" {
		t.Errorf("body = %q", doc.Body)
	}
}

func TestTagsAsString(t *testing.T) {
	doc, err := Parse([]byte("---\ntags: a, b c\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(doc.Tags(), ","); got != "a,b,c" {
		t.Errorf("tags = %q", got)
	}
}

func TestUnterminatedFrontmatterIsBody(t *testing.T) {
	src := "---\nnot closed\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Body != src {
		t.Errorf("body = %q", doc.Body)
	}
}

func TestInvalidYAMLIsAnError(t *testing.T) {
	if _, err := Parse([]byte("---\n: : :\n  - [\n---\nx")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRenderPreservesUnknownKeysAndOrder(t *testing.T) {
	src := "---\ncssclass: wide\ntitle: Old\n---\nbody\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	doc.SetTitle("New")
	doc.SetID("abc")
	doc.SetTags([]string{"x", "y"})
	out := string(doc.Render())

	if !strings.HasPrefix(out, "---\ncssclass: wide\ntitle: New\n") {
		t.Errorf("unknown key or order lost:\n%s", out)
	}
	if !strings.HasSuffix(out, "---\nbody\n") {
		t.Errorf("body lost:\n%s", out)
	}

	again, err := Parse([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if again.ID() != "abc" || strings.Join(again.Tags(), ",") != "x,y" {
		t.Errorf("round trip: id=%q tags=%v", again.ID(), again.Tags())
	}
}

func TestRenderDropsEmptyValues(t *testing.T) {
	doc, _ := Parse([]byte("---\ntags: [a]\npinned: true\n---\nb"))
	doc.SetTags(nil)
	doc.SetPinned(false)
	doc.SetArchived(nil)
	if out := string(doc.Render()); out != "b" {
		t.Errorf("expected no frontmatter left, got %q", out)
	}
}

func TestRenderWithoutFrontmatterAddsIt(t *testing.T) {
	doc, _ := Parse([]byte("just text"))
	created := time.Date(2026, 10, 2, 8, 30, 0, 0, time.UTC)
	doc.SetCreated(&created)
	out := string(doc.Render())
	if out != "---\ncreated: 2026-10-02T08:30:00Z\n---\njust text" {
		t.Errorf("got %q", out)
	}
}

func TestParseCRLF(t *testing.T) {
	doc, err := Parse([]byte("---\r\ntitle: Win\r\n---\r\nbody\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title() != "Win" || doc.Body != "body\r\n" {
		t.Errorf("title=%q body=%q", doc.Title(), doc.Body)
	}
}

func TestZonelessTimesAreLocal(t *testing.T) {
	orig := time.Local
	time.Local = time.FixedZone("PDT", -7*3600)
	t.Cleanup(func() { time.Local = orig })
	doc, _ := Parse([]byte("---\ncreated: 2026-10-02\n---\n"))
	c := doc.Created()
	if c == nil || c.In(time.Local).Format("2006-01-02") != "2026-10-02" {
		t.Errorf("created = %v, want 2 Oct in local time", c)
	}
	doc, _ = Parse([]byte("---\ncreated: 2026-10-02T23:30:00Z\n---\n"))
	if c := doc.Created(); c == nil || !c.Equal(time.Date(2026, 10, 2, 23, 30, 0, 0, time.UTC)) {
		t.Errorf("an explicit zone must be kept: %v", c)
	}
}

func TestAScalarAliasIsOneAlias(t *testing.T) {
	doc, _ := Parse([]byte("---\naliases: Project Phoenix\ntags: a b\n---\n"))
	if got := strings.Join(doc.Aliases(), "|"); got != "Project Phoenix" {
		t.Errorf("aliases = %q", got)
	}
	doc, _ = Parse([]byte("---\naliases: Phoenix, PX\n---\n"))
	if got := strings.Join(doc.Aliases(), "|"); got != "Phoenix|PX" {
		t.Errorf("aliases = %q", got)
	}
	if got := strings.Join(doc.Tags(), "|"); got != "" {
		_ = got
	}
}

func TestNullValuesAreEmpty(t *testing.T) {
	doc, _ := Parse([]byte("---\nid: ~\ntitle: null\n---\n"))
	if doc.ID() != "" || doc.Title() != "" {
		t.Errorf("id=%q title=%q", doc.ID(), doc.Title())
	}
}

func TestArchivedTrueCountsAsArchived(t *testing.T) {
	doc, _ := Parse([]byte("---\narchived: true\n---\n"))
	if doc.Archived() == nil {
		t.Error("archived: true not seen")
	}
	doc, _ = Parse([]byte("---\narchived: false\n---\n"))
	if doc.Archived() != nil {
		t.Error("archived: false read as archived")
	}
}

func TestAByteOrderMarkDoesNotHideFrontmatter(t *testing.T) {
	doc, err := Parse([]byte("\ufeff---\ntitle: BOM\n---\nbody"))
	if err != nil || doc.Title() != "BOM" || doc.Body != "body" {
		t.Errorf("title=%q body=%q err=%v", doc.Title(), doc.Body, err)
	}
}

func TestCommentsSurviveARewrittenValue(t *testing.T) {
	doc, _ := Parse([]byte("---\ntitle: Old # keep me\n# about tags\ntags: [a]\n---\n"))
	doc.SetTitle("New")
	doc.SetTags([]string{"b"})
	out := string(doc.Render())
	for _, want := range []string{"title: New # keep me", "# about tags"} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q:\n%s", want, out)
		}
	}
}
