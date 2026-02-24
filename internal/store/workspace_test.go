package store

import (
	"strings"
	"testing"

	"github.com/jeryldev/kb/internal/model"
)

func TestMigrate003CreatesWorkspacesTable(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='workspaces'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected workspaces table to exist, got count=%d", count)
	}
}

func TestMigrate003AddsWorkspaceIDToBoards(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('boards') WHERE name='workspace_id'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying pragma: %v", err)
	}
	if count != 1 {
		t.Errorf("expected workspace_id column on boards, got count=%d", count)
	}
}

func TestMigrate003AddsWorkspaceIDToNotes(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('notes') WHERE name='workspace_id'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying pragma: %v", err)
	}
	if count != 1 {
		t.Errorf("expected workspace_id column on notes, got count=%d", count)
	}
}

func TestCreateWorkspace(t *testing.T) {
	db := testDB(t)

	ws, err := db.CreateWorkspace("My Project", model.KindProject, "A test project", "/tmp/project")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if ws.Name != "My Project" {
		t.Errorf("Name = %q, want %q", ws.Name, "My Project")
	}
	if ws.Kind != model.KindProject {
		t.Errorf("Kind = %q, want %q", ws.Kind, model.KindProject)
	}
	if ws.Description != "A test project" {
		t.Errorf("Description = %q, want %q", ws.Description, "A test project")
	}
	if ws.Path != "/tmp/project" {
		t.Errorf("Path = %q, want %q", ws.Path, "/tmp/project")
	}
	if ws.Position != 0 {
		t.Errorf("Position = %d, want 0", ws.Position)
	}
}

func TestCreateWorkspaceAutoPosition(t *testing.T) {
	db := testDB(t)

	ws1, err := db.CreateWorkspace("First", model.KindProject, "", "")
	if err != nil {
		t.Fatalf("CreateWorkspace 1: %v", err)
	}
	ws2, err := db.CreateWorkspace("Second", model.KindArea, "", "")
	if err != nil {
		t.Fatalf("CreateWorkspace 2: %v", err)
	}
	if ws1.Position != 0 {
		t.Errorf("ws1.Position = %d, want 0", ws1.Position)
	}
	if ws2.Position != 1 {
		t.Errorf("ws2.Position = %d, want 1", ws2.Position)
	}
}

func TestCreateWorkspaceDuplicate(t *testing.T) {
	db := testDB(t)

	_, err := db.CreateWorkspace("Dup", model.KindProject, "", "")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	_, err = db.CreateWorkspace("Dup", model.KindArea, "", "")
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err.Error())
	}
}

func TestCreateWorkspaceEmptyName(t *testing.T) {
	db := testDB(t)

	_, err := db.CreateWorkspace("", model.KindProject, "", "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestGetWorkspace(t *testing.T) {
	db := testDB(t)

	created, err := db.CreateWorkspace("Test", model.KindArea, "desc", "/path")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	got, err := db.GetWorkspace(created.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.Name != "Test" {
		t.Errorf("Name = %q, want %q", got.Name, "Test")
	}
	if got.Kind != model.KindArea {
		t.Errorf("Kind = %q, want %q", got.Kind, model.KindArea)
	}
}

func TestGetWorkspaceNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetWorkspace("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent workspace")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestGetWorkspaceByName(t *testing.T) {
	db := testDB(t)

	created, err := db.CreateWorkspace("My Area", model.KindArea, "", "")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	got, err := db.GetWorkspaceByName("my area")
	if err != nil {
		t.Fatalf("GetWorkspaceByName: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
}

func TestGetWorkspaceByNameNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetWorkspaceByName("nope")
	if err == nil {
		t.Fatal("expected error for nonexistent workspace")
	}
}

func TestListWorkspaces(t *testing.T) {
	db := testDB(t)

	_, _ = db.CreateWorkspace("Alpha", model.KindProject, "", "")
	_, _ = db.CreateWorkspace("Beta", model.KindArea, "", "")
	_, _ = db.CreateWorkspace("Gamma", model.KindResource, "", "")

	list, err := db.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	if list[0].Name != "Alpha" || list[1].Name != "Beta" || list[2].Name != "Gamma" {
		t.Errorf("order = [%s, %s, %s], want [Alpha, Beta, Gamma]", list[0].Name, list[1].Name, list[2].Name)
	}
}

func TestListWorkspacesByKind(t *testing.T) {
	db := testDB(t)

	_, _ = db.CreateWorkspace("P1", model.KindProject, "", "")
	_, _ = db.CreateWorkspace("A1", model.KindArea, "", "")
	_, _ = db.CreateWorkspace("P2", model.KindProject, "", "")

	projects, err := db.ListWorkspacesByKind(model.KindProject)
	if err != nil {
		t.Fatalf("ListWorkspacesByKind: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("len = %d, want 2", len(projects))
	}

	areas, err := db.ListWorkspacesByKind(model.KindArea)
	if err != nil {
		t.Fatalf("ListWorkspacesByKind: %v", err)
	}
	if len(areas) != 1 {
		t.Fatalf("len = %d, want 1", len(areas))
	}
}

func TestUpdateWorkspace(t *testing.T) {
	db := testDB(t)

	ws, err := db.CreateWorkspace("Original", model.KindProject, "", "")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	ws.Name = "Updated"
	ws.Description = "new desc"
	ws.Kind = model.KindArea
	ws.Path = "/new/path"
	if err := db.UpdateWorkspace(ws); err != nil {
		t.Fatalf("UpdateWorkspace: %v", err)
	}

	got, err := db.GetWorkspace(ws.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if got.Name != "Updated" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated")
	}
	if got.Description != "new desc" {
		t.Errorf("Description = %q, want %q", got.Description, "new desc")
	}
	if got.Kind != model.KindArea {
		t.Errorf("Kind = %q, want %q", got.Kind, model.KindArea)
	}
	if got.Path != "/new/path" {
		t.Errorf("Path = %q, want %q", got.Path, "/new/path")
	}
}

func TestArchiveWorkspace(t *testing.T) {
	db := testDB(t)

	ws, err := db.CreateWorkspace("To Archive", model.KindProject, "", "")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	if err := db.ArchiveWorkspace(ws.ID); err != nil {
		t.Fatalf("ArchiveWorkspace: %v", err)
	}

	got, err := db.GetWorkspace(ws.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if got.Kind != model.KindArchive {
		t.Errorf("Kind = %q, want %q", got.Kind, model.KindArchive)
	}
}

func TestDeleteWorkspace(t *testing.T) {
	db := testDB(t)

	ws, err := db.CreateWorkspace("To Delete", model.KindProject, "", "")
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	if err := db.DeleteWorkspace(ws.ID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	_, err = db.GetWorkspace(ws.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteWorkspaceNotFound(t *testing.T) {
	db := testDB(t)

	err := db.DeleteWorkspace("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent workspace")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}
