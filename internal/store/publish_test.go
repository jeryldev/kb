package store

import (
	"strings"
	"testing"

	"github.com/jeryldev/kb/internal/model"
)

func TestMigrate004CreatesPublishTargetsTable(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='publish_targets'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected publish_targets table to exist, got count=%d", count)
	}
}

func TestMigrate004CreatesPublishLogTable(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='publish_log'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected publish_log table to exist, got count=%d", count)
	}
}

func TestCreatePublishTarget(t *testing.T) {
	db := testDB(t)

	pt, err := db.CreatePublishTarget("portfolio", model.EngineJekyll, "/tmp/site", "_posts", nil)
	if err != nil {
		t.Fatalf("CreatePublishTarget: %v", err)
	}
	if pt.Name != "portfolio" {
		t.Errorf("Name = %q, want %q", pt.Name, "portfolio")
	}
	if pt.Engine != model.EngineJekyll {
		t.Errorf("Engine = %q, want %q", pt.Engine, model.EngineJekyll)
	}
	if pt.BasePath != "/tmp/site" {
		t.Errorf("BasePath = %q, want %q", pt.BasePath, "/tmp/site")
	}
	if pt.PostsDir != "_posts" {
		t.Errorf("PostsDir = %q, want %q", pt.PostsDir, "_posts")
	}
}

func TestCreatePublishTargetDuplicate(t *testing.T) {
	db := testDB(t)

	_, err := db.CreatePublishTarget("dup", model.EngineJekyll, "/tmp", "_posts", nil)
	if err != nil {
		t.Fatalf("CreatePublishTarget: %v", err)
	}
	_, err = db.CreatePublishTarget("dup", model.EngineJekyll, "/tmp2", "_posts", nil)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err.Error())
	}
}

func TestCreatePublishTargetEmptyName(t *testing.T) {
	db := testDB(t)

	_, err := db.CreatePublishTarget("", model.EngineJekyll, "/tmp", "_posts", nil)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreatePublishTargetEmptyPath(t *testing.T) {
	db := testDB(t)

	_, err := db.CreatePublishTarget("test", model.EngineJekyll, "", "_posts", nil)
	if err == nil {
		t.Fatal("expected error for empty base path")
	}
}

func TestCreatePublishTargetDefaultPostsDir(t *testing.T) {
	db := testDB(t)

	pt, err := db.CreatePublishTarget("test", model.EngineJekyll, "/tmp", "", nil)
	if err != nil {
		t.Fatalf("CreatePublishTarget: %v", err)
	}
	if pt.PostsDir != "_posts" {
		t.Errorf("PostsDir = %q, want '_posts'", pt.PostsDir)
	}
}

func TestGetPublishTarget(t *testing.T) {
	db := testDB(t)

	created, err := db.CreatePublishTarget("test", model.EngineJekyll, "/tmp", "_posts", nil)
	if err != nil {
		t.Fatalf("CreatePublishTarget: %v", err)
	}

	got, err := db.GetPublishTarget(created.ID)
	if err != nil {
		t.Fatalf("GetPublishTarget: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("Name = %q, want %q", got.Name, "test")
	}
}

func TestGetPublishTargetNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetPublishTarget("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent target")
	}
}

func TestGetPublishTargetByName(t *testing.T) {
	db := testDB(t)

	created, err := db.CreatePublishTarget("My Site", model.EngineJekyll, "/tmp", "_posts", nil)
	if err != nil {
		t.Fatalf("CreatePublishTarget: %v", err)
	}

	got, err := db.GetPublishTargetByName("my site")
	if err != nil {
		t.Fatalf("GetPublishTargetByName: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
}

func TestListPublishTargets(t *testing.T) {
	db := testDB(t)

	_, _ = db.CreatePublishTarget("alpha", model.EngineJekyll, "/tmp/a", "_posts", nil)
	_, _ = db.CreatePublishTarget("beta", model.EngineJekyll, "/tmp/b", "_posts", nil)

	list, err := db.ListPublishTargets()
	if err != nil {
		t.Fatalf("ListPublishTargets: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
}

func TestDeletePublishTarget(t *testing.T) {
	db := testDB(t)

	pt, _ := db.CreatePublishTarget("to-delete", model.EngineJekyll, "/tmp", "_posts", nil)

	if err := db.DeletePublishTarget(pt.ID); err != nil {
		t.Fatalf("DeletePublishTarget: %v", err)
	}

	_, err := db.GetPublishTarget(pt.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeletePublishTargetNotFound(t *testing.T) {
	db := testDB(t)

	err := db.DeletePublishTarget("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent target")
	}
}

func TestCreatePublishLog(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	note, _ := db.CreateNote("Test Note", "test-note", "body", wsID)
	pt, _ := db.CreatePublishTarget("site", model.EngineJekyll, "/tmp", "_posts", nil)

	pl, err := db.CreatePublishLog(note.ID, pt.ID, "_posts/2026-02-24-test-note.md", "---\ntitle: Test\n---")
	if err != nil {
		t.Fatalf("CreatePublishLog: %v", err)
	}
	if pl.NoteID != note.ID {
		t.Errorf("NoteID = %q, want %q", pl.NoteID, note.ID)
	}
	if pl.FilePath != "_posts/2026-02-24-test-note.md" {
		t.Errorf("FilePath = %q", pl.FilePath)
	}
}

func TestGetLatestPublishLog(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	note, _ := db.CreateNote("Test Note", "test-note", "body", wsID)
	pt, _ := db.CreatePublishTarget("site", model.EngineJekyll, "/tmp", "_posts", nil)

	_, _ = db.CreatePublishLog(note.ID, pt.ID, "old-path.md", "old")
	_, _ = db.CreatePublishLog(note.ID, pt.ID, "new-path.md", "new")

	pl, err := db.GetLatestPublishLog(note.ID, pt.ID)
	if err != nil {
		t.Fatalf("GetLatestPublishLog: %v", err)
	}
	if pl == nil {
		t.Fatal("expected non-nil publish log")
	}
	if pl.FilePath != "new-path.md" {
		t.Errorf("FilePath = %q, want 'new-path.md'", pl.FilePath)
	}
}

func TestGetLatestPublishLogNotFound(t *testing.T) {
	db := testDB(t)

	pl, err := db.GetLatestPublishLog("no-note", "no-target")
	if err != nil {
		t.Fatalf("GetLatestPublishLog: %v", err)
	}
	if pl != nil {
		t.Errorf("expected nil for non-existent log, got %+v", pl)
	}
}

func TestListPublishLogs(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	n1, _ := db.CreateNote("Note 1", "note-1", "body", wsID)
	n2, _ := db.CreateNote("Note 2", "note-2", "body", wsID)
	pt, _ := db.CreatePublishTarget("site", model.EngineJekyll, "/tmp", "_posts", nil)

	_, _ = db.CreatePublishLog(n1.ID, pt.ID, "path-1.md", "")
	_, _ = db.CreatePublishLog(n2.ID, pt.ID, "path-2.md", "")

	logs, err := db.ListPublishLogs(pt.ID)
	if err != nil {
		t.Fatalf("ListPublishLogs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("len = %d, want 2", len(logs))
	}
}

func TestGetPublishedNoteSlugs(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	n1, _ := db.CreateNote("Published Note", "published-note", "body", wsID)
	_, _ = db.CreateNote("Unpublished", "unpublished", "body", wsID)
	pt, _ := db.CreatePublishTarget("site", model.EngineJekyll, "/tmp", "_posts", nil)

	_, _ = db.CreatePublishLog(n1.ID, pt.ID, "_posts/2026-02-24-published-note.md", "")

	slugs, err := db.GetPublishedNoteSlugs(pt.ID)
	if err != nil {
		t.Fatalf("GetPublishedNoteSlugs: %v", err)
	}
	if len(slugs) != 1 {
		t.Fatalf("len = %d, want 1", len(slugs))
	}
	if _, ok := slugs["published-note"]; !ok {
		t.Error("expected 'published-note' slug in map")
	}
}

func TestCreatePublishTargetWithWorkspace(t *testing.T) {
	db := testDB(t)

	ws, _ := db.CreateWorkspace("WS", model.KindProject, "", "")
	pt, err := db.CreatePublishTarget("site", model.EngineJekyll, "/tmp", "_posts", &ws.ID)
	if err != nil {
		t.Fatalf("CreatePublishTarget: %v", err)
	}
	if pt.WorkspaceID == nil || *pt.WorkspaceID != ws.ID {
		t.Errorf("WorkspaceID = %v, want %q", pt.WorkspaceID, ws.ID)
	}

	got, _ := db.GetPublishTarget(pt.ID)
	if got.WorkspaceID == nil || *got.WorkspaceID != ws.ID {
		t.Errorf("persisted WorkspaceID = %v, want %q", got.WorkspaceID, ws.ID)
	}
}
