// Package legacy moves a kb 0.3 database into the vault: its workspaces,
// boards, publish targets and publish history. Notes were files already in
// 0.3, so they stay where they are.
//
// The database is read through the sqlite3 program, so kb needs no SQLite
// code of its own. Read checks everything and builds every board before
// anything is written; Apply writes; a second run skips what the first one
// did, so an import that stopped part way can simply be run again.
package legacy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/jeryldev/kb/internal/board"
	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
)

// ImportKey is the frontmatter key each imported board keeps, holding its
// id in the database, so that a second run knows it is done.
const ImportKey = "kb-import"

// Suffix is added to the database's name once it has been imported.
const Suffix = ".imported-0.4"

type LabelChange struct {
	Card     string
	From, To string
}

type BoardPlan struct {
	OldID    string
	Name     string
	Path     string
	Cards    int
	Archived int
	Dropped  int // deleted in 0.3
	doc      *board.Board
}

type workspacePlan struct {
	name, description, path string
	kind                    model.WorkspaceKind
}

type targetPlan struct {
	name, engine, basePath, postsDir, workspace string
}

type publication struct {
	notePath, target, filePath string
	draft                      bool
}

type noteWorkspace struct {
	path, workspace string
}

// Plan is what an import will do.
type Plan struct {
	Workspaces     []workspacePlan
	Boards         []BoardPlan
	Labels         []LabelChange
	Titles         []string // cards whose title spanned lines
	Targets        []targetPlan
	Publications   []publication
	NoteWorkspaces []noteWorkspace
	Skipped        []string
}

// query runs SQL against the database, read-only, and decodes the rows.
func query(dbPath, sql string, out any) error {
	// immutable=1 opens the file without the -shm and -wal files, so
	// nothing is written next to it; Read refuses a database with a WAL.
	uri := (&url.URL{Scheme: "file", Path: dbPath, RawQuery: "immutable=1"}).String()
	cmd := exec.Command("sqlite3", "-json", uri, sql)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("reading %s with sqlite3: %v: %s", dbPath, err, strings.TrimSpace(stderr.String()))
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil // sqlite3 prints nothing for no rows
	}
	return json.Unmarshal(data, out)
}

func hasTable(dbPath, table string) (bool, error) {
	var rows []struct{ N int }
	err := query(dbPath, fmt.Sprintf("SELECT count(*) AS N FROM sqlite_master WHERE type = 'table' AND name = '%s'", table), &rows)
	return len(rows) == 1 && rows[0].N == 1, err
}

type wsRow struct {
	ID, Name, Kind, Description, Path string
	Position                          int
}

type boardRow struct {
	ID, Name, Description string
	WorkspaceID           *string `json:"workspace_id"`
}

type columnRow struct {
	ID, Name string
	BoardID  string `json:"board_id"`
	Position int
	WIPLimit *int `json:"wip_limit"`
}

type cardRow struct {
	ID, Title, Description, Priority, Labels string
	ColumnID                                 string  `json:"column_id"`
	ExternalID                               *string `json:"external_id"`
	Position                                 int
	Archived                                 *string `json:"archived_at"`
	Deleted                                  *string `json:"deleted_at"`
}

type noteRow struct {
	ID          string
	Path        *string
	WorkspaceID *string `json:"workspace_id"`
}

type targetRow struct {
	ID, Name, Engine string
	BasePath         string  `json:"base_path"`
	PostsDir         string  `json:"posts_dir"`
	WorkspaceID      *string `json:"workspace_id"`
}

type logRow struct {
	NoteID      string `json:"note_id"`
	TargetID    string `json:"target_id"`
	FilePath    string `json:"file_path"`
	FrontMatter string `json:"front_matter"`
}

// Read reads a kb 0.3 database and plans its import into the store's
// vault, checking everything first: it returns every problem it finds,
// and writes nothing.
func Read(dbPath string, s *fstore.Store) (*Plan, error) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return nil, fmt.Errorf("the import reads kb 0.3's database with the sqlite3 program, which is not installed")
	}
	if info, err := os.Stat(dbPath + "-wal"); err == nil && info.Size() > 0 {
		return nil, fmt.Errorf("%s-wal has changes not yet in the database; kb 0.3 may be running. Quit it (or open kb 0.3 once and quit) and import again", dbPath)
	}

	var workspaces []wsRow
	var boards []boardRow
	var columns []columnRow
	var cards []cardRow
	if err := query(dbPath, "SELECT id AS ID, name AS Name, kind AS Kind, description AS Description, path AS Path, position AS Position FROM workspaces ORDER BY position", &workspaces); err != nil {
		return nil, err
	}
	if err := query(dbPath, "SELECT id AS ID, name AS Name, description AS Description, workspace_id FROM boards ORDER BY name", &boards); err != nil {
		return nil, err
	}
	if err := query(dbPath, "SELECT id AS ID, board_id, name AS Name, position AS Position, wip_limit FROM columns ORDER BY board_id, position", &columns); err != nil {
		return nil, err
	}
	if err := query(dbPath, "SELECT id AS ID, column_id, title AS Title, description AS Description, priority AS Priority, position AS Position, labels AS Labels, external_id, archived_at, deleted_at FROM cards ORDER BY column_id, position", &cards); err != nil {
		return nil, err
	}

	wsName := map[string]string{}
	for _, w := range workspaces {
		wsName[w.ID] = w.Name
	}
	nameOf := func(id *string) string {
		if id == nil {
			return ""
		}
		return wsName[*id]
	}

	p := &Plan{}
	var problems []string

	// Workspaces Default aside; those the vault has already are skipped.
	for _, w := range workspaces {
		if strings.EqualFold(w.Name, model.DefaultWorkspaceName) {
			continue
		}
		if _, err := s.GetWorkspaceByName(w.Name); err == nil {
			p.Skipped = append(p.Skipped, "workspace "+w.Name+" (already in the vault)")
			continue
		}
		kind, err := model.ParseWorkspaceKind(w.Kind)
		if err != nil {
			kind = model.KindArea
		}
		if err := model.ValidateWorkspaceName(w.Name); err != nil {
			problems = append(problems, fmt.Sprintf("workspace %q: %v", w.Name, err))
		}
		p.Workspaces = append(p.Workspaces, workspacePlan{name: w.Name, kind: kind, description: w.Description, path: w.Path})
	}

	colsOf := map[string][]columnRow{}
	for _, c := range columns {
		colsOf[c.BoardID] = append(colsOf[c.BoardID], c)
	}
	cardsOf := map[string][]cardRow{}
	for _, c := range cards {
		cardsOf[c.ColumnID] = append(cardsOf[c.ColumnID], c)
	}

	done := s.BoardsByValue(ImportKey)
	paths := map[string]string{} // lower-case path → board name
	for _, b := range boards {
		if path, ok := done[b.ID]; ok {
			p.Skipped = append(p.Skipped, fmt.Sprintf("board %s (imported before, %s)", b.Name, path))
			continue
		}
		path, err := s.BoardPath(b.Name)
		if err != nil {
			problems = append(problems, fmt.Sprintf("board %q: %v", b.Name, err))
			continue
		}
		if other, taken := paths[strings.ToLower(path)]; taken {
			problems = append(problems, fmt.Sprintf("boards %q and %q would both be %s; rename one in kb 0.3 first", other, b.Name, path))
			continue
		}
		paths[strings.ToLower(path)] = b.Name
		bp, errs := p.buildBoard(b, nameOf(b.WorkspaceID), colsOf[b.ID], cardsOf)
		problems = append(problems, errs...)
		bp.Path = path
		p.Boards = append(p.Boards, bp)
	}

	if err := p.readPublishing(dbPath, s, nameOf, &problems); err != nil {
		return nil, err
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("the import cannot start; nothing was written:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return p, nil
}

// buildBoard builds a board from its columns and cards, in memory.
func (p *Plan) buildBoard(b boardRow, workspace string, cols []columnRow, cardsOf map[string][]cardRow) (BoardPlan, []string) {
	bp := BoardPlan{OldID: b.ID, Name: b.Name}
	var problems []string
	var lanes []string
	for _, c := range cols {
		if err := board.ValidateLaneName(c.Name); err != nil {
			problems = append(problems, fmt.Sprintf("board %q, column %q: %v; rename it in kb 0.3 first", b.Name, c.Name, err))
		}
		lanes = append(lanes, c.Name)
	}
	doc := board.New(lanes)
	doc.Front.SetValue("description", b.Description)
	// Files name no workspace for Default, as kb writes them.
	if !strings.EqualFold(workspace, model.DefaultWorkspaceName) {
		doc.Front.SetWorkspace(workspace)
	}
	doc.Front.SetValue(ImportKey, b.ID)
	doc.MarkFrontmatterChanged()
	if len(problems) > 0 {
		return bp, problems
	}

	for _, c := range cols {
		lane := doc.Lane(c.Name)
		if c.WIPLimit != nil && *c.WIPLimit > 0 {
			lane.MaxItems = *c.WIPLimit
		}
		var archive []string
		for _, card := range cardsOf[c.ID] {
			if card.Deleted != nil {
				bp.Dropped++
				continue
			}
			// Trailing blank lines would be stray indented lines in the file.
			title, desc := card.Title, strings.TrimRight(card.Description, " \t\r\n")
			if first, rest, ok := strings.Cut(strings.ReplaceAll(title, "\r\n", "\n"), "\n"); ok {
				title = strings.TrimSpace(first)
				desc = strings.TrimSpace(strings.TrimSpace(rest) + "\n\n" + desc)
				p.Titles = append(p.Titles, title)
			}
			it, err := doc.Add(c.Name, title, desc)
			if err != nil {
				problems = append(problems, fmt.Sprintf("board %q, card %q: %v", b.Name, card.Title, err))
				continue
			}
			// Keep the start of the old id, so ids in notes and scripts
			// still find the card.
			if id := shortID(card.ID); id != "" {
				if other, _ := doc.Find(id); other == nil {
					it.ID = id
				}
			}
			var labels []string
			for _, l := range strings.Split(card.Labels, ",") {
				if l = strings.TrimSpace(l); l == "" {
					continue
				}
				labels = append(labels, l)
				if n := board.NormalizeLabel(l); n != l {
					p.Labels = append(p.Labels, LabelChange{Card: title, From: l, To: n})
				}
			}
			if len(labels) > 0 {
				it.SetLabels(labels)
			}
			if pr, err := model.ParsePriority(card.Priority); err == nil {
				it.SetPriority(string(pr))
			}
			if card.ExternalID != nil && *card.ExternalID != "" {
				it.SetField("ext", *card.ExternalID)
			}
			if card.Archived != nil {
				archive = append(archive, it.ID)
				bp.Archived++
			} else {
				bp.Cards++
			}
		}
		for _, id := range archive {
			if err := doc.ArchiveItem(id); err != nil {
				problems = append(problems, fmt.Sprintf("board %q: archiving a card: %v", b.Name, err))
			}
		}
	}
	bp.doc = doc
	return bp, problems
}

// shortID is the first 8 hex digits of a uuid, the length of new card ids.
func shortID(id string) string {
	if len(id) < 8 {
		return ""
	}
	for _, r := range id[:8] {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return ""
		}
	}
	return id[:8]
}

func (p *Plan) readPublishing(dbPath string, s *fstore.Store, nameOf func(*string) string, problems *[]string) error {
	var notes []noteRow
	if ok, err := hasTable(dbPath, "notes"); err != nil {
		return err
	} else if ok {
		if err := query(dbPath, "SELECT id AS ID, path AS Path, workspace_id FROM notes", &notes); err != nil {
			return err
		}
	}
	notePath := map[string]string{}
	for _, n := range notes {
		if n.Path == nil {
			continue
		}
		notePath[n.ID] = *n.Path
		// A workspace the database knew but the file does not name.
		ws := nameOf(n.WorkspaceID)
		if ws == "" || strings.EqualFold(ws, model.DefaultWorkspaceName) {
			continue
		}
		note, err := s.GetNoteByPath(*n.Path)
		if err != nil || note.WorkspaceID != s.DefaultWorkspace().ID {
			continue
		}
		p.NoteWorkspaces = append(p.NoteWorkspaces, noteWorkspace{path: *n.Path, workspace: ws})
	}

	if ok, err := hasTable(dbPath, "publish_targets"); err != nil || !ok {
		return err
	}
	var targets []targetRow
	var logs []logRow
	if err := query(dbPath, "SELECT id AS ID, name AS Name, engine AS Engine, base_path, posts_dir, workspace_id FROM publish_targets ORDER BY name", &targets); err != nil {
		return err
	}
	if err := query(dbPath, "SELECT note_id, target_id, file_path, front_matter FROM publish_log ORDER BY published_at", &logs); err != nil {
		return err
	}
	targetName := map[string]string{}
	for _, t := range targets {
		targetName[t.ID] = t.Name
		if _, err := s.GetPublishTarget(t.Name); err == nil {
			p.Skipped = append(p.Skipped, "publish target "+t.Name+" (already set up)")
			continue
		}
		if !strings.HasPrefix(t.BasePath, "/") && !strings.HasPrefix(t.BasePath, "~/") {
			*problems = append(*problems, fmt.Sprintf("publish target %q has a relative path, %q; fix it in kb 0.3 first, or delete it there and set it up again after the import", t.Name, t.BasePath))
			continue
		}
		p.Targets = append(p.Targets, targetPlan{name: t.Name, engine: t.Engine, basePath: t.BasePath, postsDir: t.PostsDir, workspace: nameOf(t.WorkspaceID)})
	}
	// The latest publish of each note to each target is the one to keep.
	latest := map[[2]string]publication{}
	var order [][2]string
	for _, l := range logs {
		path, ok := notePath[l.NoteID]
		if !ok || targetName[l.TargetID] == "" {
			continue
		}
		key := [2]string{path, targetName[l.TargetID]}
		if _, seen := latest[key]; !seen {
			order = append(order, key)
		}
		latest[key] = publication{notePath: path, target: key[1], filePath: l.FilePath, draft: strings.Contains(l.FrontMatter, "published: false")}
	}
	for _, key := range order {
		p.Publications = append(p.Publications, latest[key])
	}
	return nil
}

// Apply writes the plan into the vault. Every step skips what is already
// there, so Apply can be run again after a failure part way.
func (p *Plan) Apply(s *fstore.Store) error {
	for _, w := range p.Workspaces {
		if _, err := s.GetWorkspaceByName(w.name); err == nil {
			continue
		}
		if _, err := s.CreateWorkspace(w.name, w.kind, w.description, w.path); err != nil {
			return fmt.Errorf("workspace %s: %w", w.name, err)
		}
	}
	for _, b := range p.Boards {
		if _, done := s.BoardsByValue(ImportKey)[b.OldID]; done {
			continue
		}
		if _, err := s.CreateBoardFrom(b.Name, b.doc); err != nil {
			return fmt.Errorf("board %s: %w", b.Name, err)
		}
	}
	wsID := func(name string) string {
		if ws, err := s.GetWorkspaceByName(name); err == nil {
			return ws.ID
		}
		return ""
	}
	for _, t := range p.Targets {
		if _, err := s.GetPublishTarget(t.name); err == nil {
			continue
		}
		engine, err := model.ParseEngine(t.engine)
		if err != nil {
			return fmt.Errorf("publish target %s: %w", t.name, err)
		}
		if _, err := s.CreatePublishTarget(t.name, engine, t.basePath, t.postsDir, "", wsID(t.workspace)); err != nil {
			return fmt.Errorf("publish target %s: %w", t.name, err)
		}
	}
	for _, pub := range p.Publications {
		note, err := s.GetNoteByPath(pub.notePath)
		if errors.Is(err, fstore.ErrNotFound) {
			continue // the note's file is gone; so is its history
		}
		if err != nil {
			return err
		}
		if post, ok := s.LatestPost(note.ID, pub.target); ok && post.Path == pub.filePath && post.Draft == pub.draft {
			continue
		}
		if err := s.RecordPublish(note.ID, pub.target, pub.filePath, pub.draft); err != nil {
			return fmt.Errorf("%s: %w", pub.notePath, err)
		}
	}
	for _, nw := range p.NoteWorkspaces {
		note, err := s.GetNoteByPath(nw.path)
		if err != nil {
			continue
		}
		if id := wsID(nw.workspace); id != "" && note.WorkspaceID != id {
			if err := s.SetNoteWorkspace(note.ID, id); err != nil {
				return fmt.Errorf("%s: %w", nw.path, err)
			}
		}
	}
	return nil
}

// Empty reports whether the plan has nothing left to do.
func (p *Plan) Empty() bool {
	return len(p.Workspaces)+len(p.Boards)+len(p.Targets)+len(p.Publications)+len(p.NoteWorkspaces) == 0
}

// Report says what the import does, for people.
func (p *Plan) Report() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	for _, w := range p.Workspaces {
		line("  workspace %s (%s)", w.name, w.kind)
	}
	for _, bp := range p.Boards {
		parts := []string{plural(bp.Cards, "card")}
		if bp.Archived > 0 {
			parts = append(parts, fmt.Sprintf("%d archived", bp.Archived))
		}
		if bp.Dropped > 0 {
			parts = append(parts, plural(bp.Dropped, "deleted card")+" dropped")
		}
		line("  board %s (%s): %s", bp.Name, bp.Path, strings.Join(parts, ", "))
	}
	for _, t := range p.Targets {
		line("  publish target %s (%s)", t.name, t.basePath)
	}
	if n := len(p.Publications); n > 0 {
		line("  publish history of %s, into their frontmatter", plural(n, "note"))
	}
	for _, nw := range p.NoteWorkspaces {
		line("  %s goes in workspace %s", nw.path, nw.workspace)
	}
	if len(p.Labels) > 0 {
		line("Labels become #tags, which cannot hold spaces:")
		sort.SliceStable(p.Labels, func(i, j int) bool { return p.Labels[i].Card < p.Labels[j].Card })
		for _, l := range p.Labels {
			line("  %q becomes #%s (card %q)", l.From, l.To, l.Card)
		}
	}
	if len(p.Titles) > 0 {
		line("Card titles cannot span lines; the rest went into the description:")
		for _, t := range p.Titles {
			line("  %q", t)
		}
	}
	for _, s := range p.Skipped {
		line("  skipped %s", s)
	}
	return b.String()
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// Finish renames the database, and its -wal and -shm files, out of the
// way once the import is done, keeping them as a backup.
func Finish(dbPath string) error {
	for _, f := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Rename(f, f+Suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("renaming %s: %w", f, err)
		}
	}
	return nil
}
