package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jeryldev/kb/internal/legacy"
	"github.com/spf13/cobra"
)

// legacyDBPath is where kb 0.3 kept its SQLite database.
func legacyDBPath() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "kb", "kb.db"), nil
}

// checkImport stops kb while a 0.3 database waits to be imported, so that
// nothing is written to the vault that the import would then collide with.
// A kb.db that is not a database at all (empty, damaged) holds nothing to
// import and never stops kb.
func checkImport() error {
	path, err := legacyDBPath()
	if err != nil {
		return err
	}
	if !legacy.MayHoldData(path) {
		return nil
	}
	return fmt.Errorf("found kb 0.3's database at %s; move its boards into the vault with: kb import (see what it would do with: kb import --dry-run). If it is not kb 0.3's, move it aside: mv %q %q", path, path, path+".not-kb")
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Move kb 0.3's boards and settings into the vault",
	Long: `Move what kb 0.3 kept in its database (~/.local/share/kb/kb.db) into the
vault: workspaces into .kb/workspaces.yml, each board into a Markdown file
in the Obsidian Kanban format, publish targets into ~/.config/kb, and
publish history into the notes' frontmatter. Notes are files already.

Everything is checked before anything is written. Labels become #tags,
and the report lists every label that changes. Running it again skips
what is done. Afterwards kb.db is renamed kb.db.imported-0.4 and kept.

It reads the database with the sqlite3 program.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := legacyDBPath()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			if _, err := os.Stat(path + legacy.Suffix); err == nil {
				fmt.Fprintf(out, "Already imported; kb 0.3's database is kept as %s\n", path+legacy.Suffix)
				return nil
			}
			fmt.Fprintf(out, "Nothing to import: there is no kb 0.3 database at %s\n", path)
			return nil
		}
		store, err := openStore()
		if err != nil {
			return err
		}
		db = store
		plan, err := legacy.Read(path, store)
		if err != nil {
			return err
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		fmt.Fprintf(out, "From %s into %s:\n", path, store.Vault().Root())
		if plan.Empty() {
			fmt.Fprintln(out, "  nothing left to move")
		}
		fmt.Fprint(out, plan.Report())
		if dryRun {
			fmt.Fprintln(out, "Nothing was written (--dry-run).")
			return nil
		}
		if err := plan.Apply(store); err != nil {
			return fmt.Errorf("%w\nWhat was written stays; fix the cause and run kb import again to finish", err)
		}
		if err := legacy.Finish(path); err != nil {
			return err
		}
		fmt.Fprintf(out, "Done. kb 0.3's database is kept as %s\n", path+legacy.Suffix)
		return nil
	},
}

func init() {
	importCmd.Flags().Bool("dry-run", false, "Show what the import would do, and write nothing")
	rootCmd.AddCommand(importCmd)
}
