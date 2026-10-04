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

func (s *Store) loadWorkspaces(cands []*candidate, boards []*boardFile) ([]*model.Workspace, []string) {
	var problems []string
	file, err := s.readWorkspacesFile()
	if err != nil {
		problems = append(problems, err.Error())
	}
	paths, err := s.readWorkspacePaths()
	if err != nil {
		problems = append(problems, err.Error())
	}
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
	// Workspaces the list does not hold come after those it does, except
	// Default, which comes first.
	next := 0
	for _, ws := range out {
		next = max(next, ws.Position+1)
	}
	implicit := func(name string) {
		if name == "" || byName[strings.ToLower(name)] != nil {
			return
		}
		pos := next
		if strings.EqualFold(name, model.DefaultWorkspaceName) {
			pos = -1
		} else {
			next++
		}
		ws := &model.Workspace{ID: nameID(name), Name: name, Kind: model.KindArea, Position: pos, Path: paths[nameID(name)]}
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
	return out, problems
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
	if err != nil || strings.EqualFold(ws.Name, model.DefaultWorkspaceName) {
		return ""
	}
	return ws.Name
}

// editWorkspaces changes .kb/workspaces.yml through fn. Under the
// file's lock it reads the vault again, so fn sees workspaces another kb
// added since this one read it, and edits the file's own YAML: comments,
// keys and kinds kb does not know, and entries it skips stay as they are,
// and a workspace only notes name is not written into the list. A file kb
// cannot read is never written over.
func (s *Store) editWorkspaces(fn func(f *workspacesDoc, current []*model.Workspace) error) error {
	return s.writeLocked(workspacesFile, func() error {
		f, err := s.readWorkspacesDoc()
		if err != nil {
			return fmt.Errorf("%w; fix it first, kb will not write over it", err)
		}
		if err := s.Reload(); err != nil {
			return err
		}
		if err := fn(f, s.workspaces); err != nil {
			return err
		}
		var buf strings.Builder
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(f.root); err != nil {
			return err
		}
		enc.Close()
		_, err = s.vault.Write(workspacesFile, &vault.Doc{Body: buf.String()})
		return err
	})
}

// workspacesDoc is .kb/workspaces.yml as a YAML tree, edited in place.
type workspacesDoc struct {
	root *yaml.Node // the document
	list *yaml.Node // the workspaces: sequence
}

func (s *Store) readWorkspacesDoc() (*workspacesDoc, error) {
	if _, err := s.readWorkspacesFile(); err != nil {
		return nil, err
	}
	abs, err := s.vault.Abs(workspacesFile)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if data, err := os.ReadFile(abs); err == nil {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return nil, fmt.Errorf("%s: %w", workspacesFile, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(root.Content) == 0 {
		root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map",
			HeadComment: "kb workspaces. Notes and boards name their workspace in their frontmatter."}}}
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s is not a key/value mapping", workspacesFile)
	}
	if list := mapValue(top, "workspaces"); list != nil && list.Kind == yaml.SequenceNode {
		return &workspacesDoc{root: &root, list: list}, nil
	}
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	setMapValue(top, "workspaces", list)
	return &workspacesDoc{root: &root, list: list}, nil
}

// entry is the list's entry for a workspace: the one with its id, or with
// no id and its name (whose id kb derives from the name).
func (f *workspacesDoc) entry(id string) *yaml.Node {
	for _, e := range f.list.Content {
		if e.Kind != yaml.MappingNode {
			continue
		}
		eid, name := scalar(mapValue(e, "id")), scalar(mapValue(e, "name"))
		if eid == id || (eid == "" && name != "" && nameID(name) == id) {
			return e
		}
	}
	return nil
}

func (f *workspacesDoc) add(ws *model.Workspace) {
	pos := 0
	for _, e := range f.list.Content {
		var p int
		if v := mapValue(e, "position"); v != nil && v.Decode(&p) == nil && p >= pos {
			pos = p + 1
		}
	}
	e := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setMapValue(e, "id", str(ws.ID))
	setMapValue(e, "name", str(ws.Name))
	setMapValue(e, "kind", str(string(ws.Kind)))
	if ws.Description != "" {
		setMapValue(e, "description", str(ws.Description))
	}
	setMapValue(e, "position", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(pos)})
	f.list.Content = append(f.list.Content, e)
}

// update writes what changed from before to after into the workspace's
// entry, adding one when the list has none (a workspace only notes named).
func (f *workspacesDoc) update(before, after *model.Workspace) {
	e := f.entry(before.ID)
	if e == nil {
		f.add(after)
		return
	}
	if before.Name != after.Name {
		setMapValue(e, "name", str(after.Name))
	}
	if before.Kind != after.Kind {
		setMapValue(e, "kind", str(string(after.Kind)))
	}
	if before.Description != after.Description {
		if after.Description == "" {
			setMapValue(e, "description", nil)
		} else {
			setMapValue(e, "description", str(after.Description))
		}
	}
}

func (f *workspacesDoc) remove(id string) {
	e := f.entry(id)
	for i, c := range f.list.Content {
		if c == e {
			f.list.Content = append(f.list.Content[:i], f.list.Content[i+1:]...)
			return
		}
	}
}

func str(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }

func scalar(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMapValue sets key in m, keeping the old value's comments; a nil
// value removes the key.
func setMapValue(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			continue
		}
		if value == nil {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
		old := m.Content[i+1]
		value.HeadComment, value.LineComment, value.FootComment = old.HeadComment, old.LineComment, old.FootComment
		m.Content[i+1] = value
		return
	}
	if value != nil {
		m.Content = append(m.Content, str(key), value)
	}
}

// CreateWorkspace adds a workspace to the vault's list.
func (s *Store) CreateWorkspace(name string, kind model.WorkspaceKind, description, path string) (*model.Workspace, error) {
	if err := model.ValidateWorkspaceName(name); err != nil {
		return nil, err
	}
	ws := &model.Workspace{ID: uuid.New().String(), Name: name, Kind: kind, Description: description, Path: path, CreatedAt: time.Now()}
	// The path file is checked first, so a workspace is not listed with a
	// path that could not be saved.
	if _, err := s.readWorkspacePaths(); err != nil && path != "" {
		return nil, fmt.Errorf("%w; fix it first, kb will not write over it", err)
	}
	err := s.editWorkspaces(func(f *workspacesDoc, current []*model.Workspace) error {
		for _, w := range current {
			if strings.EqualFold(w.Name, name) {
				return fmt.Errorf("workspace %q already exists", name)
			}
		}
		f.add(ws)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if path != "" {
		if err := s.setWorkspacePath(ws.ID, path); err != nil {
			return nil, err
		}
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
	renamed := before.Name != ws.Name
	if renamed {
		if strings.EqualFold(before.Name, model.DefaultWorkspaceName) {
			return fmt.Errorf("the %s workspace cannot be renamed", model.DefaultWorkspaceName)
		}
		if other, err := s.GetWorkspaceByName(ws.Name); err == nil && other.ID != ws.ID {
			return fmt.Errorf("workspace %q already exists", ws.Name)
		}
	}
	// Both settings files are checked before any note is renamed, so a
	// file kb cannot write stops the edit before it half-applies.
	if _, err := s.readWorkspacesFile(); err != nil {
		return fmt.Errorf("%w; fix it first, kb will not write over it", err)
	}
	if renamed {
		if err := s.noPlaceholders("renaming"); err != nil {
			return err
		}
	}
	pathChanged := before.Path != ws.Path
	if _, err := s.readWorkspacePaths(); err != nil && pathChanged {
		return fmt.Errorf("%w; fix it first, kb will not write over it", err)
	}
	var errs []error
	if renamed {
		errs = s.renameWorkspaceInFiles(before.Name, ws.Name)
	}
	err = s.editWorkspaces(func(f *workspacesDoc, _ []*model.Workspace) error {
		f.update(before, ws)
		return nil
	})
	if err != nil {
		return err
	}
	if pathChanged {
		if err := s.setWorkspacePath(ws.ID, ws.Path); err != nil {
			return err
		}
	}
	if err := s.Reload(); err != nil {
		return err
	}
	return errors.Join(errs...)
}

func (s *Store) renameWorkspaceInFiles(oldName, newName string) []error {
	var errs []error
	s.batch(func() error {
		errs = s.renameWorkspaceInEachFile(oldName, newName)
		return nil
	})
	return errs
}

func (s *Store) renameWorkspaceInEachFile(oldName, newName string) []error {
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

// noPlaceholders refuses a workspace change that must see every file
// while some are iCloud placeholders kb knows only by name: one of them may
// name the workspace.
func (s *Store) noPlaceholders(doing string) error {
	var paths []string
	for _, n := range s.notes {
		if s.dataless[n.ID] {
			paths = append(paths, n.Path)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	verb := "are"
	if len(paths) == 1 {
		verb = "is"
	}
	return fmt.Errorf("%s a workspace needs every file, and %s %s in iCloud and not downloaded; open them (kb open) first", doing, strings.Join(paths, ", "), verb)
}

// ArchiveWorkspace changes a workspace's kind to archive.
func (s *Store) ArchiveWorkspace(id string) error {
	ws, err := s.GetWorkspace(id)
	if err != nil {
		return err
	}
	if strings.EqualFold(ws.Name, model.DefaultWorkspaceName) {
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
	if strings.EqualFold(ws.Name, model.DefaultWorkspaceName) {
		return fmt.Errorf("the %s workspace cannot be deleted", model.DefaultWorkspaceName)
	}
	if err := s.noPlaceholders("deleting"); err != nil {
		return err
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
	err = s.editWorkspaces(func(f *workspacesDoc, _ []*model.Workspace) error {
		f.remove(id)
		return nil
	})
	if err != nil {
		return err
	}
	return s.Reload()
}

// Workspace paths are paths on this machine, so they live in the config
// directory, not in the synced vault.
func (s *Store) workspacePathsFile() string {
	return filepath.Join(s.opts.ConfigDir, "workspace-paths.yml")
}

func (s *Store) readWorkspacePaths() (map[string]string, error) {
	paths := map[string]string{}
	err := readConfigFile(s.workspacePathsFile(), &paths)
	return paths, err
}

func (s *Store) setWorkspacePath(id, path string) error {
	return s.editConfigFile(s.workspacePathsFile(), func() (any, bool, error) {
		paths, err := s.readWorkspacePaths()
		if err != nil {
			return nil, false, err
		}
		if paths[id] == path {
			return nil, false, nil
		}
		if path == "" {
			delete(paths, id)
		} else {
			paths[id] = path
		}
		return paths, true, nil
	})
}

// readConfigFile reads a YAML settings file in the config directory into
// out. A missing file is empty; one that does not parse is an error
// naming it.
func readConfigFile(abs string, out any) error {
	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: %w", abs, err)
	}
	return nil
}

// editConfigFile changes a settings file in the config directory, holding
// its lock so two kb processes never write it from the same old copy. fn
// reads the file and returns what to write, or false to leave it; a read
// error stops the edit, so a file kb cannot parse is never written over.
// The file is replaced whole, through a temporary file, never half
// written.
func (s *Store) editConfigFile(abs string, fn func() (any, bool, error)) error {
	unlock, err := lockFile(s.opts.LockDir, abs)
	if err != nil {
		return err
	}
	defer unlock()
	value, write, err := fn()
	if err != nil {
		return fmt.Errorf("%w; fix it first, kb will not write over it", err)
	}
	if !write {
		return nil
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), filepath.Base(abs)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), abs)
}
