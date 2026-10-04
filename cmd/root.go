package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/tui"
	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
)

// version is set at release time (goreleaser's ldflags); a go install
// build reports its module version instead.
var version = "dev"

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
format, and workspaces are listed in .kb/workspaces.yml. Run kb alone in
a terminal window for the full-screen view.

New to kb? Start with: kb help start

Card and column commands work on the current board: --board, else
$KB_BOARD, else the first board that exists named after the dev tmux
session ($TMUX_SESSION_NAME), the folder you are in, or its git
repository (worktrees included).

Upgrading from kb 0.3? Run kb import once.`,
	// Usage is for mistakes in how a command was typed, not for errors
	// such as a note that does not exist.
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "help" || strings.HasPrefix(cmd.Name(), "__") || cmd.HasParent() && cmd.Parent().Name() == "completion" || cmd.Name() == "import" {
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
		// The full-screen view draws on a terminal; run where there is none
		// (an editor's task runner, an AI assistant's shell), say so plainly.
		if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
			return errNoTerminal
		}
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
	c, err := rootCmd.ExecuteC()
	if err != nil && holdOnError(c, isTerminal(os.Stdin), os.Getenv("TMUX") != "") {
		// dev opens kb in a tmux popup that closes when kb exits, which
		// would take the error with it before it could be read.
		fmt.Fprint(os.Stderr, "Press Enter to close.")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return err
}

// holdOnError says whether to wait before exiting on an error: when the
// TUI could not start in a terminal inside tmux, such as a popup.
var errNoTerminal = errors.New(`kb's full-screen view needs a terminal window, and this one has none.
Open kb in a terminal (Terminal, iTerm, a tmux pane), or use the commands
here: kb notes, kb boards, kb cards -B <board>. See: kb help`)

func holdOnError(c *cobra.Command, terminal, inTmux bool) bool {
	return c == rootCmd && terminal && inTmux
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func versionString() string {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			return strings.TrimPrefix(info.Main.Version, "v")
		}
	}
	return version
}

func init() {
	rootCmd.Version = versionString()
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().StringVarP(&boardFlag, "board", "B", "", "Board to use (default: $KB_BOARD, else the board named after the tmux session, the folder or its git repository)")
}

// boardCandidates are the boards the user may mean, in order: --board,
// then $KB_BOARD (each the only candidate when set), then the dev tmux
// session's name, the folder's name, and the name of the git repository
// the folder is in. dev opens kb in a workspace's folder, which can be a
// worktree (~/.worktrees/<repo>/<branch>) or a folder deep in a repo.
func boardCandidates() []string {
	if boardFlag != "" {
		return []string{boardFlag}
	}
	if name := os.Getenv("KB_BOARD"); name != "" {
		return []string{name}
	}
	var out []string
	if session := os.Getenv("TMUX_SESSION_NAME"); session != "" {
		out = append(out, strings.TrimPrefix(session, "dev-"))
	}
	if cwd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Base(cwd))
		if repo := repoName(cwd); repo != "" {
			out = append(out, repo)
		}
	}
	return out
}

// repoName is the name of the git repository dir is in: the main
// repository's folder, also from inside one of its worktrees.
func repoName(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		dotGit := filepath.Join(d, ".git")
		info, err := os.Stat(dotGit)
		if err == nil && info.IsDir() {
			return filepath.Base(d)
		}
		if err == nil {
			// A worktree's .git file points into the main repository:
			// "gitdir: <repo>/.git/worktrees/<name>".
			data, err := os.ReadFile(dotGit)
			if err != nil {
				return ""
			}
			gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
			if !ok {
				return ""
			}
			gitdir = filepath.Clean(strings.TrimSpace(gitdir))
			if before, _, found := strings.Cut(gitdir, string(filepath.Separator)+".git"+string(filepath.Separator)); found {
				return filepath.Base(before)
			}
			return filepath.Base(d)
		}
		if parent := filepath.Dir(d); parent == d {
			return ""
		}
	}
}

// detectBoard is the first candidate board that exists, else the first
// candidate.
func detectBoard() string {
	candidates := boardCandidates()
	if len(candidates) == 0 {
		return ""
	}
	if db != nil {
		for _, name := range candidates {
			if _, err := db.GetBoard(name); err == nil {
				return name
			}
		}
	}
	return candidates[0]
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
	return nil, fmt.Errorf("no board named %s (taken from the tmux session, the folder and its repository); choose one with --board or $KB_BOARD, or create it with: kb board create %q", strings.Join(quoted(boardCandidates()), " or "), name)
}

func quoted(names []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			out = append(out, fmt.Sprintf("%q", n))
		}
	}
	return out
}
