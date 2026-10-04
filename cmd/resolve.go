package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
	"github.com/spf13/cobra"
)

// resolveNote finds a note by slug, title, alias, file name or id.
func resolveNote(ref string) (*model.Note, error) {
	return db.ResolveNote(ref)
}

// resolveWorkspace finds a workspace by name (ignoring case) or id.
func resolveWorkspace(ref string) (*model.Workspace, error) {
	if ws, err := db.GetWorkspaceByName(ref); err == nil {
		return ws, nil
	}
	if ws, err := db.GetWorkspace(ref); err == nil {
		return ws, nil
	}
	return nil, fmt.Errorf("workspace %q not found; see kb workspace", ref)
}

// workspaceIDFor turns a --workspace value into an id; empty means Default.
func workspaceIDFor(ref string) (string, error) {
	if ref == "" {
		return db.DefaultWorkspace().ID, nil
	}
	ws, err := resolveWorkspace(ref)
	if err != nil {
		return "", err
	}
	return ws.ID, nil
}

// resolveCard finds a card by id or id prefix: on the current board when
// there is one, else on any board (an id found on several boards is an
// error naming them).
func resolveCard(ref string) (*model.Card, error) {
	if b, err := currentBoard(); err == nil {
		if c, err := db.FindCard(b.ID, ref); err == nil {
			return c, nil
		}
	}
	if boardFlag != "" {
		b, err := currentBoard()
		if err != nil {
			return nil, err
		}
		return db.FindCard(b.ID, ref)
	}
	return db.FindCard("", ref)
}

// resolveLane finds a lane of a board by name, ignoring case.
func resolveLane(boardID, name string) (fstore.Lane, error) {
	lanes, err := db.Lanes(boardID)
	if err != nil {
		return fstore.Lane{}, err
	}
	for _, l := range lanes {
		if strings.EqualFold(l.Name, name) {
			return l, nil
		}
	}
	var names []string
	for _, l := range lanes {
		names = append(names, l.Name)
	}
	return fstore.Lane{}, fmt.Errorf("no column %q; the columns are %s", name, strings.Join(names, ", "))
}

var errCancelled = errors.New("cancelled")

// confirm asks before something destructive, on stderr, and reads the
// answer from stdin, so a script can pipe one in. With --force it does
// not ask; with no answer at all (stdin closed) it refuses.
func confirm(cmd *cobra.Command, force bool, question string) error {
	if force {
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", question)
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	case "":
		if line == "" {
			fmt.Fprintln(cmd.ErrOrStderr())
			return fmt.Errorf("%w (no answer; use --force to skip the question)", errCancelled)
		}
	}
	return errCancelled
}

type backlink = fstore.Backlink
