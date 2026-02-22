package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeryldev/kb/internal/store"
	"github.com/jeryldev/kb/internal/tui"
	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
)

var db *store.DB

var rootCmd = &cobra.Command{
	Use:   "kb",
	Short: "Terminal Kanban board",
	Long:  "A terminal Kanban board for personal project management.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "help" {
			return nil
		}
		var err error
		db, err = store.Open()
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if db != nil {
			return db.Close()
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		boardName := detectBoard()
		app := tui.NewApp(db, boardName)
		p := tea.NewProgram(app, tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func detectBoard() string {
	if name := os.Getenv("KB_BOARD"); name != "" {
		return name
	}

	if tmuxSession := os.Getenv("TMUX_SESSION_NAME"); tmuxSession != "" {
		name := strings.TrimPrefix(tmuxSession, "dev-")
		return name
	}

	if cwd, err := os.Getwd(); err == nil {
		return filepath.Base(cwd)
	}

	return ""
}
