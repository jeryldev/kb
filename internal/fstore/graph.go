package fstore

import (
	"strings"

	"github.com/jeryldev/kb/internal/graph"
)

// GraphSource is everything the graph shows: notes, boards and the cards
// that link to notes, with the links between them.
func (s *Store) GraphSource() graph.Source {
	src := graph.Source{Notes: s.ListNotes()}
	for _, b := range s.ListBoards() {
		src.Boards = append(src.Boards, graph.Item{ID: b.ID, Label: b.Name, WorkspaceID: b.WorkspaceID})
	}
	seen := map[string]bool{}
	for _, l := range s.links {
		if l.SourceType == "card" {
			boardPath, cardID, _ := cutHash(l.SourceID)
			if seen[l.SourceID] {
				continue
			}
			if bf := s.boardByPath(boardPath); bf != nil {
				if it, _ := bf.b.Find(cardID); it != nil {
					seen[l.SourceID] = true
					src.Cards = append(src.Cards, graph.Item{ID: l.SourceID, Label: it.Title, WorkspaceID: s.workspaceIDForName(bf.b.Front.Workspace()), CardID: cardID, Board: bf.name()})
				}
			}
		}
		if l.TargetID == "" {
			continue
		}
		src.Links = append(src.Links, graph.Link{SourceType: l.SourceType, SourceID: l.SourceID, TargetType: l.TargetType, TargetID: l.TargetID, Context: l.Context})
	}
	return src
}

// cutHash splits a card source id "<board path>#<card id>" at its last #:
// a board's file name can hold one, a card id cannot.
func cutHash(s string) (string, string, bool) {
	if i := strings.LastIndexByte(s, '#'); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return s, "", false
}
