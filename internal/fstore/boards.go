package fstore

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jeryldev/kb/internal/board"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
)

type boardDoc = board.Board

// boardFile is a board as read from the vault: a Markdown file in the
// Obsidian Kanban plugin's format. Its id is its vault path.
type boardFile struct {
	path string
	b    *board.Board
	rev  string
}

func loadBoard(rel string, data []byte) (*boardFile, error) {
	b, err := board.ParseSalted(data, rel)
	if err != nil {
		return nil, err
	}
	return &boardFile{path: rel, b: b, rev: revOf(data)}, nil
}

// name is the board's name: its file name.
func (bf *boardFile) name() string { return stem(bf.path) }

func (s *Store) boardByPath(rel string) *boardFile {
	for _, b := range s.boards {
		if b.path == rel {
			return b
		}
	}
	return nil
}

// updateBoardFile changes one board file: under the board's lock it reads
// the file as it is now, applies fn and writes the result, so that changes
// from other kb processes in between are never lost. fn must express a
// change that can be applied to whatever the file holds now (add, move,
// archive a card), not a snapshot of the whole board.
func (s *Store) updateBoardFile(rel string, fn func(*boardDoc) error) error {
	err := s.writeLocked(rel, func() error {
		abs, err := s.vault.Abs(rel)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("reading board %s: %w", rel, err)
		}
		b, err := board.ParseSalted(data, rel)
		if err != nil {
			return err
		}
		if err := fn(b); err != nil {
			return err
		}
		_, err = s.vault.Write(rel, &vault.Doc{Body: string(b.Render())})
		return err
	})
	if reloadErr := s.Reload(); reloadErr != nil && err == nil {
		err = reloadErr
	}
	return err
}

// rewriteBoardLinks rewrites links in the cards of one board, for a note
// rename.
func (s *Store) rewriteBoardLinks(rel string, rewrite func(string) (string, bool)) (int, error) {
	changed := 0
	err := s.updateBoardFile(rel, func(b *boardDoc) error {
		for _, lane := range append(append([]*board.Lane{}, b.Lanes...), archiveOf(b)...) {
			for _, it := range lane.Items() {
				title, n1 := model.RewriteWikilinks(it.Title, rewrite)
				desc, n2 := model.RewriteWikilinks(it.Description, rewrite)
				if n1 > 0 {
					it.SetTitle(title)
				}
				if n2 > 0 {
					it.SetDescription(desc)
				}
				if n1+n2 > 0 {
					changed++
				}
			}
		}
		return nil
	})
	return changed, err
}

func archiveOf(b *boardDoc) []*board.Lane {
	if b.Archive != nil {
		return []*board.Lane{b.Archive}
	}
	return nil
}


// boardName is used in error messages.
func boardName(rel string) string { return strings.TrimSuffix(rel, ".md") }

// BoardsDir is where kb creates boards (KB_BOARDS_DIR, default "Boards").
func BoardsDir() string {
	if d := os.Getenv("KB_BOARDS_DIR"); d != "" {
		return strings.Trim(d, "/")
	}
	return "Boards"
}

var defaultLanes = []string{"Backlog", "Todo", "In Progress", "Review", "Done"}

func (s *Store) boardModel(bf *boardFile) *model.Board {
	return &model.Board{
		ID:          bf.path,
		Name:        bf.name(),
		Description: bf.b.Front.Value("description"),
		WorkspaceID: s.workspaceIDForName(bf.b.Front.Workspace()),
	}
}

// ListBoards lists the boards in the vault by name.
func (s *Store) ListBoards() []*model.Board {
	var out []*model.Board
	for _, bf := range s.boards {
		out = append(out, s.boardModel(bf))
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// GetBoard finds a board by name (ignoring case) or by its vault path.
func (s *Store) GetBoard(ref string) (*model.Board, error) {
	bf, err := s.findBoard(ref)
	if err != nil {
		return nil, err
	}
	return s.boardModel(bf), nil
}

func (s *Store) findBoard(ref string) (*boardFile, error) {
	var matches []*boardFile
	for _, bf := range s.boards {
		if bf.path == ref {
			return bf, nil
		}
		if strings.EqualFold(bf.name(), ref) {
			matches = append(matches, bf)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("board %q: %w", ref, ErrNotFound)
	}
	var paths []string
	for _, m := range matches {
		paths = append(paths, m.path)
	}
	return nil, fmt.Errorf("more than one board is called %q (%s); name it by path", ref, strings.Join(paths, ", "))
}

// CreateBoard writes a new board, with the usual lanes, in BoardsDir.
func (s *Store) CreateBoard(name, description, workspaceID string) (*model.Board, error) {
	if err := model.ValidateBoardName(name); err != nil {
		return nil, err
	}
	if _, err := s.findBoard(name); err == nil {
		return nil, fmt.Errorf("board %q already exists", name)
	}
	file := model.FileName(name)
	if file == "" {
		return nil, fmt.Errorf("%q has no characters a file name can hold", name)
	}
	rel := BoardsDir() + "/" + file + ".md"
	b := board.New(defaultLanes)
	if description != "" {
		b.Front.SetValue("description", description)
	}
	b.Front.SetWorkspace(s.workspaceNameForFile(workspaceID))
	b.MarkFrontmatterChanged()
	err := s.writeLocked(rel, func() error {
		_, err := s.vault.Create(rel, &vault.Doc{Body: string(b.Render())})
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s.GetBoard(rel)
}

// TrashBoard moves a board's file to the vault's .trash folder.
func (s *Store) TrashBoard(id string) (string, error) {
	bf, err := s.findBoard(id)
	if err != nil {
		return "", err
	}
	var dest string
	err = s.writeLocked(bf.path, func() error {
		var err error
		dest, err = s.moveToTrash(bf.path)
		return err
	})
	if reloadErr := s.Reload(); reloadErr != nil && err == nil {
		err = reloadErr
	}
	return dest, err
}

// SetBoardWorkspace records a board's workspace in its frontmatter.
func (s *Store) SetBoardWorkspace(id, workspaceID string) error {
	bf, err := s.findBoard(id)
	if err != nil {
		return err
	}
	return s.updateBoardFile(bf.path, func(b *boardDoc) error {
		if s.workspaceIDForName(b.Front.Workspace()) != workspaceID {
			b.Front.SetWorkspace(s.workspaceNameForFile(workspaceID))
			b.MarkFrontmatterChanged()
		}
		return nil
	})
}

// Lane is one lane of a board as kb shows it.
type Lane struct {
	Name     string
	WIPLimit *int
	Position int
	Cards    int
}

func (s *Store) Lanes(boardID string) ([]Lane, error) {
	bf, err := s.findBoard(boardID)
	if err != nil {
		return nil, err
	}
	var out []Lane
	for i, l := range bf.b.Lanes {
		lane := Lane{Name: l.Title, Position: i, Cards: len(l.Items())}
		if l.MaxItems > 0 {
			n := l.MaxItems
			lane.WIPLimit = &n
		}
		out = append(out, lane)
	}
	return out, nil
}

func (s *Store) withBoard(boardID string, fn func(*boardDoc) error) error {
	bf, err := s.findBoard(boardID)
	if err != nil {
		return err
	}
	return s.updateBoardFile(bf.path, fn)
}

func (s *Store) AddLane(boardID, name string) error {
	return s.withBoard(boardID, func(b *boardDoc) error { return b.AddLane(name) })
}

func (s *Store) RenameLane(boardID, from, to string) error {
	return s.withBoard(boardID, func(b *boardDoc) error { return b.RenameLane(from, to) })
}

func (s *Store) ReorderLanes(boardID string, names []string) error {
	return s.withBoard(boardID, func(b *boardDoc) error { return b.ReorderLanes(names) })
}

// SetWIPLimit sets a lane's card limit; 0 clears it. The plugin keeps it
// in the lane's title, as "In Progress (3)".
func (s *Store) SetWIPLimit(boardID, lane string, limit int) error {
	if limit < 0 {
		return fmt.Errorf("a card limit cannot be negative")
	}
	return s.withBoard(boardID, func(b *boardDoc) error {
		l := b.Lane(lane)
		if l == nil {
			return fmt.Errorf("no lane %q", lane)
		}
		l.MaxItems = limit
		return nil
	})
}

// DeleteLane removes a lane. A lane with cards needs force, and its cards
// are archived rather than lost.
func (s *Store) DeleteLane(boardID, lane string, force bool) error {
	return s.withBoard(boardID, func(b *boardDoc) error {
		l := b.Lane(lane)
		if l == nil {
			return fmt.Errorf("no lane %q", lane)
		}
		if items := l.Items(); len(items) > 0 {
			if !force {
				return fmt.Errorf("lane %q has %d cards; use --force to archive them and delete the lane", l.Title, len(items))
			}
			for _, it := range items {
				if err := b.ArchiveItem(it.ID); err != nil {
					return err
				}
			}
		}
		return b.RemoveLane(l.Title)
	})
}

// WIPLimitError means a lane is full.
type WIPLimitError struct {
	Lane  string
	Limit int
}

func (e *WIPLimitError) Error() string {
	return fmt.Sprintf("lane %q is at its limit of %d cards (use --force to go over it)", e.Lane, e.Limit)
}

func checkWIP(l *board.Lane, force bool) error {
	if !force && l.MaxItems > 0 && len(l.Items()) >= l.MaxItems {
		return &WIPLimitError{Lane: l.Title, Limit: l.MaxItems}
	}
	return nil
}

// CardFields are a new card's optional fields.
type CardFields struct {
	Description string
	Priority    string
	Labels      []string
	ExternalID  string
}

func cardRev(it *board.Item) string { return revOf([]byte(string(it.Check) + it.RawText())) }

func (s *Store) cardModel(bf *boardFile, lane *board.Lane, index int, it *board.Item) *model.Card {
	c := &model.Card{
		ID:          it.ID,
		BoardID:     bf.path,
		ColumnID:    lane.Title,
		Title:       it.Title,
		Description: it.Description,
		Priority:    model.Priority(it.Priority()),
		Position:    index,
		Labels:      strings.Join(it.Labels(), ","),
		ExternalID:  it.Field("ext"),
		Rev:         cardRev(it),
	}
	if lane == bf.b.Archive {
		c.ArchivedAt = &time.Time{}
	}
	return c
}

// Cards lists a lane's cards in order.
func (s *Store) Cards(boardID, lane string) ([]*model.Card, error) {
	bf, err := s.findBoard(boardID)
	if err != nil {
		return nil, err
	}
	l := bf.b.Lane(lane)
	if l == nil {
		return nil, fmt.Errorf("no lane %q on board %q", lane, bf.name())
	}
	var out []*model.Card
	for i, it := range l.Items() {
		out = append(out, s.cardModel(bf, l, i, it))
	}
	return out, nil
}

// ArchivedCards lists a board's archived cards.
func (s *Store) ArchivedCards(boardID string) ([]*model.Card, error) {
	bf, err := s.findBoard(boardID)
	if err != nil {
		return nil, err
	}
	var out []*model.Card
	if a := bf.b.Archive; a != nil {
		for i, it := range a.Items() {
			out = append(out, s.cardModel(bf, a, i, it))
		}
	}
	return out, nil
}

// GetCard finds a card on a board, archived or not.
func (s *Store) GetCard(boardID, cardID string) (*model.Card, error) {
	bf, err := s.findBoard(boardID)
	if err != nil {
		return nil, err
	}
	it, lane := bf.b.Find(cardID)
	if it == nil {
		return nil, fmt.Errorf("card %q on board %q: %w", cardID, bf.name(), ErrNotFound)
	}
	for i, other := range lane.Items() {
		if other == it {
			return s.cardModel(bf, lane, i, it), nil
		}
	}
	return nil, fmt.Errorf("card %q: %w", cardID, ErrNotFound)
}

// AddCard adds a card at the end of a lane. A full lane refuses it unless
// force is set.
func (s *Store) AddCard(boardID, lane, title string, f CardFields, force bool) (*model.Card, error) {
	var id string
	err := s.withBoard(boardID, func(b *boardDoc) error {
		l := b.Lane(lane)
		if l == nil {
			return fmt.Errorf("no lane %q", lane)
		}
		if err := checkWIP(l, force); err != nil {
			return err
		}
		it, err := b.Add(l.Title, title, f.Description)
		if err != nil {
			return err
		}
		if len(f.Labels) > 0 {
			it.SetLabels(f.Labels)
		}
		if f.Priority != "" {
			it.SetPriority(f.Priority)
		}
		if f.ExternalID != "" {
			it.SetField("ext", f.ExternalID)
		}
		id = it.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetCard(boardID, id)
}

// UpdateCard writes a card's title, description, priority, labels and
// external id. If the card's text changed since it was read (its Rev), the
// edit is refused with ErrConflict rather than overwrite that change.
func (s *Store) UpdateCard(c *model.Card) error {
	if err := board.ValidateCardTitle(c.Title); err != nil {
		return err
	}
	err := s.withBoard(c.BoardID, func(b *boardDoc) error {
		it, _ := b.Find(c.ID)
		if it == nil {
			return fmt.Errorf("card %q: %w", c.ID, ErrNotFound)
		}
		if c.Rev != "" && cardRev(it) != c.Rev {
			return ErrConflict
		}
		if it.Title != c.Title {
			it.SetTitle(c.Title)
		}
		if it.Description != c.Description {
			it.SetDescription(c.Description)
		}
		if string(c.Priority) != it.Priority() {
			it.SetPriority(string(c.Priority))
		}
		labels := c.LabelList()
		if strings.Join(normalizeLabels(labels), ",") != strings.Join(it.Labels(), ",") {
			it.SetLabels(labels)
		}
		if it.Field("ext") != c.ExternalID {
			it.SetField("ext", c.ExternalID)
		}
		return nil
	})
	if err != nil {
		return err
	}
	fresh, err := s.GetCard(c.BoardID, c.ID)
	if err != nil {
		return err
	}
	*c = *fresh
	return nil
}

func normalizeLabels(labels []string) []string {
	var out []string
	for _, l := range labels {
		if n := board.NormalizeLabel(l); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// MoveCard puts a card in a lane above another card (beforeID), or at the
// end when beforeID is empty. Moving into another lane that is full is
// refused unless force is set; reordering within a lane is always allowed.
func (s *Store) MoveCard(boardID, cardID, lane, beforeID string, force bool) error {
	return s.withBoard(boardID, func(b *boardDoc) error {
		it, from := b.Find(cardID)
		if it == nil {
			return fmt.Errorf("card %q: %w", cardID, ErrNotFound)
		}
		to := b.Lane(lane)
		if to == nil {
			return fmt.Errorf("no lane %q", lane)
		}
		if to != from {
			if err := checkWIP(to, force); err != nil {
				return err
			}
		}
		return b.MoveBefore(cardID, to.Title, beforeID)
	})
}

func (s *Store) ArchiveCard(boardID, cardID string) error {
	return s.withBoard(boardID, func(b *boardDoc) error { return b.ArchiveItem(cardID) })
}

func (s *Store) DeleteCard(boardID, cardID string) error {
	return s.withBoard(boardID, func(b *boardDoc) error { return b.Delete(cardID) })
}
