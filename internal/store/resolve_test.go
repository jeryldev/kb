package store

import (
	"os"
	"path/filepath"
	"testing"
)

// linksFrom maps each link target of the note at slug to whether it
// resolved to a note in the index.
func backlinkSources(t *testing.T, db *DB, targetSlug string) []string {
	t.Helper()
	target, err := db.GetNoteBySlug(targetSlug)
	if err != nil {
		t.Fatalf("target %s: %v", targetSlug, err)
	}
	links, err := db.GetBacklinks("note", target.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, l := range links {
		src, err := db.GetNote(l.SourceID)
		if err != nil {
			t.Fatalf("link source: %v", err)
		}
		sources = append(sources, src.Slug)
	}
	return sources
}

func assertLinked(t *testing.T, db *DB, targetSlug string, want ...string) {
	t.Helper()
	got := backlinkSources(t, db, targetSlug)
	if len(got) != len(want) {
		t.Fatalf("backlinks of %s = %v, want %v", targetSlug, got, want)
	}
	seen := map[string]bool{}
	for _, s := range got {
		seen[s] = true
	}
	for _, s := range want {
		if !seen[s] {
			t.Errorf("backlinks of %s = %v, missing %s", targetSlug, got, s)
		}
	}
}

func TestLinksResolveByTitleFileNameAndAlias(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "cpp.md", "---\ntitle: C++ Notes\n---\n")
	writeVaultFile(t, db, "3 Horizons of Growth.md", "")
	writeVaultFile(t, db, "dual.md", "---\naliases: [DT, Dual Transformation]\n---\n")
	writeVaultFile(t, db, "by-title.md", "see [[C++ Notes]]")
	writeVaultFile(t, db, "by-stem.md", "see [[3 horizons of growth]]")
	writeVaultFile(t, db, "by-alias.md", "see [[dt]] and [[Dual Transformation]]")
	scan(t, db)

	assertLinked(t, db, "cpp", "by-title")
	assertLinked(t, db, "3-horizons-of-growth", "by-stem")
	assertLinked(t, db, "dual", "by-alias")
}

func TestLinksIgnoreHeadingsBlocksAndDisplayText(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "Target Note.md", "")
	writeVaultFile(t, db, "a.md", "[[Target Note#Phase 2]]")
	writeVaultFile(t, db, "b.md", "[[Target Note^abc123]]")
	writeVaultFile(t, db, "c.md", "[[Target Note|shown text]]")
	writeVaultFile(t, db, "d.md", "[[sub/Deep Note]] [[Target Note.md]]")
	writeVaultFile(t, db, "sub/Deep Note.md", "")
	scan(t, db)

	assertLinked(t, db, "target-note", "a", "b", "c", "d")
	assertLinked(t, db, "deep-note", "d")
}

func TestDanglingLinkResolvesWhenTheTargetAppears(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)
	db.CreateNote("Early", "early", "points at [[Later Idea]]", wsID)

	// Created through kb...
	db.CreateNote("Later Idea", "later-idea-note", "", wsID)
	assertLinked(t, db, "later-idea-note", "early")

	// ...and after the target is deleted and comes back as a new file.
	later, _ := db.GetNoteBySlug("later-idea-note")
	if err := db.DeleteNote(later.ID); err != nil {
		t.Fatal(err)
	}
	writeVaultFile(t, db, "Later Idea.md", "back again")
	scan(t, db)
	assertLinked(t, db, "later-idea", "early")
}

func TestTwoRefsToOneNoteMakeOneLink(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "target.md", "---\naliases: [T]\n---\n")
	writeVaultFile(t, db, "src.md", "[[target]] and [[T]] and [[Missing]] and [[Gone]]")
	scan(t, db)
	assertLinked(t, db, "target", "src")
	// Two dangling refs that come to name the same new note collapse into
	// one link instead of failing the scan.
	writeVaultFile(t, db, "Missing.md", "---\naliases: [Gone]\n---\n")
	scan(t, db)
	assertLinked(t, db, "missing", "src")
}

func TestSearchMatchesWordPrefixesAccentsAndAllWords(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "cafe.md", "Meeting at the café about strategic foresight")
	writeVaultFile(t, db, "go.md", "---\ntags: [tools]\n---\nTable driven tests in Go")
	writeVaultFile(t, db, "other.md", "Nothing relevant")
	scan(t, db)

	cases := map[string][]string{
		"cafe":            {"cafe"},
		"strat fore":      {"cafe"},
		"tests go":        {"go"},
		"tools":           {"go"},
		"go nothing":      nil,
		`"unbalanced`:     nil,
		"c++ OR -AND (*)": nil,
	}
	for query, want := range cases {
		notes, err := db.SearchNotes(query)
		if err != nil {
			t.Errorf("SearchNotes(%q): %v", query, err)
			continue
		}
		var got []string
		for _, n := range notes {
			got = append(got, n.Slug)
		}
		if len(got) != len(want) || (len(want) > 0 && got[0] != want[0]) {
			t.Errorf("SearchNotes(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestSearchFollowsEditsAndDeletes(t *testing.T) {
	db := testDB(t)
	note, _ := db.CreateNote("Volatile", "volatile", "alpha", testDefaultWSID(t, db))
	note.Body = "beta"
	if err := db.UpdateNote(note); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.SearchNotes("alpha"); len(got) != 0 {
		t.Errorf("stale match on old body")
	}
	if got, _ := db.SearchNotes("beta"); len(got) != 1 {
		t.Errorf("no match on new body")
	}
	db.DeleteNote(note.ID)
	if got, _ := db.SearchNotes("beta"); len(got) != 0 {
		t.Errorf("deleted note still found")
	}
}

func TestRebuildRereadsEveryFile(t *testing.T) {
	db := testDB(t)
	db.CreateNote("One", "one", "x", testDefaultWSID(t, db))
	writeVaultFile(t, db, "two.md", "y")
	scan(t, db)
	res, err := db.Rebuild()
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 2 || res.Added+res.Removed != 0 {
		t.Errorf("rebuild = %+v", res)
	}
}

func forwardRefs(t *testing.T, db *DB, slug string) map[string]string {
	t.Helper()
	n, err := db.GetNoteBySlug(slug)
	if err != nil {
		t.Fatal(err)
	}
	links, _ := db.GetForwardLinks("note", n.ID)
	out := map[string]string{}
	for _, l := range links {
		out[l.TargetRef] = l.TargetID
	}
	return out
}

func TestSameNoteHeadingLinksAreNotLinks(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "toc.md", "jump to [[#Summary]] or [[^block1]]\n# Summary")
	scan(t, db)
	if refs := forwardRefs(t, db, "toc"); len(refs) != 0 {
		t.Errorf("links = %v", refs)
	}
}

func TestADanglingLinkNeverResolvesToItsOwnNote(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "draft.md", "see [[Big Idea]]")
	scan(t, db)
	// The note itself later takes the name it was pointing at.
	writeVaultFile(t, db, "draft.md", "---\naliases: [Big Idea]\n---\nsee [[Big Idea]]")
	writeVaultFile(t, db, "other.md", "unrelated change")
	scan(t, db)
	draft, _ := db.GetNoteBySlug("draft")
	if back, _ := db.GetBacklinks("note", draft.ID); len(back) != 0 {
		t.Errorf("self link created: %+v", back[0])
	}
}

func TestLinksFollowTheVaultAsItChanges(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "deep/folder/Topic.md", "")
	writeVaultFile(t, db, "src.md", "[[Topic]]")
	scan(t, db)
	assertLinked(t, db, "topic", "src")

	// A closer match appears: shortest path wins, now as at write time.
	writeVaultFile(t, db, "Topic.md", "")
	scan(t, db)
	assertLinked(t, db, "topic-2", "src")

	// The target is renamed away from the name: the link no longer points at it.
	os.Rename(filepath.Join(db.Vault().Root(), "Topic.md"), filepath.Join(db.Vault().Root(), "Subject.md"))
	os.Remove(filepath.Join(db.Vault().Root(), "deep/folder/Topic.md"))
	scan(t, db)
	if got := forwardRefs(t, db, "src")["Topic"]; got != "Topic" {
		t.Errorf("link target = %q, want the raw ref", got)
	}
}
