package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/tui"
	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	db         *fstore.Store
	boardFlag  string
	jsonOutput bool
)

var rootCmd = &cobra.Command{
	Use:   "kb",
	Short: "Terminal notes and Kanban boards",
	Long: `Terminal notes and Kanban boards for personal projects.

Everything is a file in your vault folder ($KB_VAULT, default ~/notes):
notes are Markdown, boards are Markdown in the Obsidian Kanban plugin's
format, and workspaces are listed in .kb/workspaces.yml. Run kb alone for
the TUI.`,
	// Usage is for mistakes in how a command was typed, not for errors
	// such as a note that does not exist.
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "help" || cmd.Name() == "version" || strings.HasPrefix(cmd.Name(), "__") || cmd.HasParent() && cmd.Parent().Name() == "completion" || cmd.Name() == "import" {
			return nil
		}
		if db != nil {
			return nil
		}
		if err := checkImport(); err != nil {
			return err
		}
		store, err := openStore()
		if err != nil {
			return err
		}
		db = store
		for _, problem := range db.Problems() {
			fmt.Fprintf(os.Stderr, "kb: warning: %s\n", problem)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		app := tui.NewApp(db, detectBoard())
		_, err := tea.NewProgram(app, tea.WithAltScreen()).Run()
		return err
	},
}

func openStore() (*fstore.Store, error) {
	vaultDir, err := fstore.VaultDir()
	if err != nil {
		return nil, err
	}
	opts, err := fstore.DefaultOptions()
	if err != nil {
		return nil, err
	}
	return fstore.Open(vaultDir, opts)
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().StringVarP(&boardFlag, "board", "B", "", "Board to use (default: $KB_BOARD, the dev tmux session, or the folder name)")
}

// detectBoard is the board the user means: --board, then $KB_BOARD, then
// the dev-session-manager tmux session name, then the folder's name.
func detectBoard() string {
	if boardFlag != "" {
		return boardFlag
	}
	if name := os.Getenv("KB_BOARD"); name != "" {
		return name
	}
	if session := os.Getenv("TMUX_SESSION_NAME"); session != "" {
		return strings.TrimPrefix(session, "dev-")
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Base(cwd)
	}
	return ""
}

// currentBoard resolves the board card and column commands work on.
func currentBoard() (*model.Board, error) {
	name := detectBoard()
	b, err := db.GetBoard(name)
	if err == nil {
		return b, nil
	}
	if boardFlag != "" || os.Getenv("KB_BOARD") != "" {
		return nil, err
	}
	return nil, fmt.Errorf("no board named %q (taken from the folder name); choose one with --board or $KB_BOARD, or create it with: kb board create %q", name, name)
}
