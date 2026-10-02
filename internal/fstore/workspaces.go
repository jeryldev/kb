package fstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
	"gopkg.in/yaml.v3"
)

// Workspaces are defined in the vault, in .kb/workspaces.yml, so they sync
// with the notes that name them. A workspace a note or board names that
// the file does not list still exists (an "implicit" workspace); kb never
// strips such a name from a file. Default always exists.

const workspacesFile = ".kb/workspaces.yml"

type workspaceYAML struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Kind        string `yaml:"kind"`
	Description string `yaml:"description,omitempty"`
	Position    int    `yaml:"position"`
}

type workspacesYAML struct {
	Workspaces []workspaceYAML `yaml:"workspaces"`
}

// nameID is the id of a workspace known only by name.
func nameID(name string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kb-workspace:"+strings.ToLower(name))).String()
}

func (s *Store) readWorkspacesFile() (workspacesYAML, error) {
	var w workspacesYAML
	abs, err := s.vault.Abs(workspacesFile)
	if err != nil {
		return w, err
	}
	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return w, nil
	}
	if err != nil {
		return w, err
	}
	if err := yaml.Unmarshal(data, &w); err != nil {
		return w, fmt.Errorf("%s: %w", workspacesFile, err)
	}
	return w, nil
}

func (s *Store) loadWorkspaces(cands []*candidate, boards []*boardFile) ([]*model.Workspace, []string, error) {
	var problems []string
	file, err := s.readWorkspacesFile()
	if err != nil {
		problems = append(problems, err.Error())
	}
	paths := s.readWorkspacePaths()
	var out []*model.Workspace
	byName := map[string]*model.Workspace{}
	for _, w := range file.Workspaces {
		if w.Name == "" || byName[strings.ToLower(w.Name)] != nil {
			continue
		}
		kind, err := model.ParseWorkspaceKind(w.Kind)
		if err != nil {
			kind = model.KindArea
		}
		ws := &model.Workspace{ID: w.ID, Name: w.Name, Kind: kind, Description: w.Description, Position: w.Position, Path: paths[w.ID]}
		if ws.ID == "" {
			ws.ID = nameID(w.Name)
		}
		out = append(out, ws)
		byName[strings.ToLower(w.Name)] = ws
	}
	implicit := func(name string) {
		if name == "" || byName[strings.ToLower(name)] != nil {
			return
		}
		ws := &model.Workspace{ID: nameID(name), Name: name, Kind: model.KindArea, Position: len(out), Path: paths[nameID(name)]}
		out = append(out, ws)
		byName[strings.ToLower(name)] = ws
	}
	implicit(model.DefaultWorkspaceName)
	for _, c := range cands {
		implicit(c.doc.Workspace())
	}
	for _, b := range boards {
		implicit(b.b.Front.Workspace())
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, problems, nil
}

func (s *Store) ListWorkspaces() []*model.Workspace { return s.workspaces }

func (s *Store) DefaultWorkspace() *model.Workspace {
	ws, _ := s.GetWorkspaceByName(model.DefaultWorkspaceName)
	return ws
}

func (s *Store) GetWorkspace(id string) (*model.Workspace, error) {
	for _, w := range s.workspaces {
		if w.ID == id {
			return w, nil
		}
	}
	return nil, fmt.Errorf("workspace %q: %w", id, ErrNotFound)
}

// GetWorkspaceByName finds a workspace by name, ignoring case.
func (s *Store) GetWorkspaceByName(name string) (*model.Workspace, error) {
	for _, w := range s.workspaces {
		if strings.EqualFold(w.Name, name) {
			return w, nil
		}
	}
	return nil, fmt.Errorf("workspace %q: %w", name, ErrNotFound)
}

// workspaceIDForName resolves the workspace a file names; no name means
// Default.
func (s *Store) workspaceIDForName(name string) string {
	if name == "" {
		name = model.DefaultWorkspaceName
	}
	if ws, err := s.GetWorkspaceByName(name); err == nil {
		return ws.ID
	}
	return nameID(name)
}

// workspaceNameForFile is the name a file records for a workspace: nothing
// for Default, so most files carry no workspace key.
func (s *Store) workspaceNameForFile(id string) string {
	ws, err := s.GetWorkspace(id)
	if err != nil || ws.Name == model.DefaultWorkspaceName {
		return ""
	}
	return ws.Name
}

// writeWorkspaces writes the workspace list to .kb/workspaces.yml.
func (s *Store) writeWorkspaces(list []*model.Workspace) error {
	var file workspacesYAML
	for i, w := range list {
		file.Workspaces = append(file.Workspaces, workspaceYAML{ID: w.ID, Name: w.Name, Kind: string(w.Kind), Description: w.Description, Position: i})
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	header := "# kb workspaces. Notes and boards name their workspace in their frontmatter.\n"
	return s.writeLocked(workspacesFile, func() error {
		_, err := s.vault.Write(workspacesFile, &vault.Doc{Body: header + string(data)})
		return err
	})
}

// CreateWorkspace adds a workspace to the vault's list.
func (s *Store) CreateWorkspace(name string, kind model.WorkspaceKind, description, path string) (*model.Workspace, error) {
	if err := model.ValidateWorkspaceName(name); err != nil {
		return nil, err
	}
	if _, err := s.GetWorkspaceByName(name); err == nil {
		return nil, fmt.Errorf("workspace %q already exists", name)
	}
	ws := &model.Workspace{ID: uuid.New().String(), Name: name, Kind: kind, Description: description, Path: path, CreatedAt: time.Now()}
	list := append(append([]*model.Workspace{}, s.workspaces...), ws)
	if err := s.writeWorkspaces(list); err != nil {
		return nil, err
	}
	if err := s.setWorkspacePath(ws.ID, path); err != nil {
		return nil, err
	}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s.GetWorkspace(ws.ID)
}

// UpdateWorkspace changes a workspace. A new name is written into every
// note and board that names the workspace first, then into the list, so
// that an interrupted rename leaves notes under a name that still exists.
// Default cannot be renamed.
func (s *Store) UpdateWorkspace(ws *model.Workspace) error {
	if err := model.ValidateWorkspaceName(ws.Name); err != nil {
		return err
	}
	before, err := s.GetWorkspace(ws.ID)
	if err != nil {
		return err
	}
	renamed := !strings.EqualFold(before.Name, ws.Name) || before.Name != ws.Name
	if renamed {
		if before.Name == model.DefaultWorkspaceName {
			return fmt.Errorf("the %s workspace cannot be renamed", model.DefaultWorkspaceName)
		}
		if other, err := s.GetWorkspaceByName(ws.Name); err == nil && other.ID != ws.ID {
			return fmt.Errorf("workspace %q already exists", ws.Name)
		}
	}
	var errs []error
	if renamed {
		errs = s.renameWorkspaceInFiles(before.Name, ws.Name)
	}
	list := append([]*model.Workspace{}, s.workspaces...)
	for i, w := range list {
		if w.ID == ws.ID {
			copyWS := *ws
			list[i] = &copyWS
		}
	}
	if err := s.writeWorkspaces(list); err != nil {
		return err
	}
	if err := s.setWorkspacePath(ws.ID, ws.Path); err != nil {
		return err
	}
	if err := s.Reload(); err != nil {
		return err
	}
	return errors.Join(errs...)
}

func (s *Store) renameWorkspaceInFiles(oldName, newName string) []error {
	var errs []error
	for _, n := range s.notes {
		doc := s.docs[n.ID]
		if doc == nil || !strings.EqualFold(doc.Workspace(), oldName) {
			continue
		}
		if err := s.patchNote(n.ID, true, func(d *vault.Doc) {
			if d.ID() == pathID(n.Path) {
				d.SetID("")
			}
			d.SetWorkspace(newName)
		}); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", n.Path, err))
		}
	}
	for _, b := range s.boards {
		if strings.EqualFold(b.b.Front.Workspace(), oldName) {
			if err := s.updateBoardFile(b.path, func(bd *boardDoc) error {
				bd.Front.SetWorkspace(newName)
				bd.MarkFrontmatterChanged()
				return nil
			}); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", b.path, err))
			}
		}
	}
	return errs
}

// ArchiveWorkspace changes a workspace's kind to archive.
func (s *Store) ArchiveWorkspace(id string) error {
	ws, err := s.GetWorkspace(id)
	if err != nil {
		return err
	}
	if ws.Name == model.DefaultWorkspaceName {
		return fmt.Errorf("the %s workspace cannot be archived", model.DefaultWorkspaceName)
	}
	updated := *ws
	updated.Kind = model.KindArchive
	return s.UpdateWorkspace(&updated)
}

// DeleteWorkspace removes a workspace from the list. It refuses while any
// note (archived ones too), board or publish target uses it, naming them.
func (s *Store) DeleteWorkspace(id string) error {
	ws, err := s.GetWorkspace(id)
	if err != nil {
		return err
	}
	if ws.Name == model.DefaultWorkspaceName {
		return fmt.Errorf("the %s workspace cannot be deleted", model.DefaultWorkspaceName)
	}
	var users []string
	for _, n := range s.notes {
		if n.WorkspaceID == id {
			users = append(users, n.Path)
		}
	}
	for _, b := range s.boards {
		if s.workspaceIDForName(b.b.Front.Workspace()) == id {
			users = append(users, b.path)
		}
	}
	for _, t := range s.ListPublishTargets() {
		if t.WorkspaceID != nil && *t.WorkspaceID == id {
			users = append(users, "publish target "+t.Name)
		}
	}
	if len(users) > 0 {
		return fmt.Errorf("workspace %q is still used by %s", ws.Name, strings.Join(users, ", "))
	}
	var list []*model.Workspace
	for _, w := range s.workspaces {
		if w.ID != id {
			list = append(list, w)
		}
	}
	if err := s.writeWorkspaces(list); err != nil {
		return err
	}
	return s.Reload()
}

// Workspace paths are paths on this machine, so they live in the config
// directory, not in the synced vault.
func (s *Store) workspacePathsFile() string {
	return filepath.Join(s.opts.ConfigDir, "workspace-paths.yml")
}

func (s *Store) readWorkspacePaths() map[string]string {
	paths := map[string]string{}
	data, err := os.ReadFile(s.workspacePathsFile())
	if err == nil {
		yaml.Unmarshal(data, &paths)
	}
	return paths
}

func (s *Store) setWorkspacePath(id, path string) error {
	paths := s.readWorkspacePaths()
	if paths[id] == path {
		return nil
	}
	if path == "" {
		delete(paths, id)
	} else {
		paths[id] = path
	}
	data, err := yaml.Marshal(paths)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.opts.ConfigDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.workspacePathsFile(), data, 0o644)
}
