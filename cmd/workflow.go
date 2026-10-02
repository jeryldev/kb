package cmd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"text/tabwriter"
	"time"

	"github.com/jeryldev/kb/internal/editor"
	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open <note>",
	Short: "Open a note's file in your editor",
	Long: `Open a note's Markdown file in $VISUAL or $EDITOR.

The note can be named the way a wikilink would name it: its title, file
name, slug or an alias, in any case.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}
		return editNoteFile(note)
	},
}

var dailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "Open today's daily note, creating it if needed",
	Long: `Open the daily note for today (or --date) in your editor.

Daily notes are YYYY-MM-DD.md files in the vault folder named by
KB_DAILY_DIR (default "daily").`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		date, _ := cmd.Flags().GetString("date")
		if date == "" {
			date = time.Now().Format(time.DateOnly)
		}
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			return fmt.Errorf("--date must look like 2026-10-02, got %q", date)
		}
		dir := os.Getenv("KB_DAILY_DIR")
		if dir == "" {
			dir = "daily"
		}
		rel := path.Join(dir, date+".md")

		note, err := db.GetNoteByPath(rel)
		if err != nil {
			if !errors.Is(err, fstore.ErrNotFound) {
				return err
			}
			if note, err = db.CreateNoteAt(rel, date, "", db.DefaultWorkspace().ID); err != nil {
				return err
			}
		}
		return editNoteFile(note)
	},
}

var tagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "List note tags, most used first",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tags := db.ListTags()
		if jsonOutput {
			return jsonList(tags, func(t fstore.TagCount) tagJSON { return tagJSON{Tag: t.Tag, Notes: t.Count} })
		}
		if len(tags) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No tags yet. Add some with: kb note edit <note> --tags a,b")
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TAG\tNOTES")
		for _, t := range tags {
			fmt.Fprintf(w, "%s\t%d\n", t.Tag, t.Count)
		}
		return w.Flush()
	},
}

// editNoteFile runs the editor on the note's file, then reads the vault
// again.
func editNoteFile(note *model.Note) error {
	path, err := db.Vault().Abs(note.Path)
	if err != nil {
		return err
	}
	cmd, err := editor.Command(path)
	if err != nil {
		return err
	}
	runErr := cmd.Run()
	// The editor may have changed anything in the file.
	if err := db.Reload(); err != nil {
		return err
	}
	if runErr != nil {
		return fmt.Errorf("editor: %w", runErr)
	}
	return nil
}

func init() {
	dailyCmd.Flags().String("date", "", "Date of the daily note (YYYY-MM-DD, default today)")
	rootCmd.AddCommand(openCmd, dailyCmd, tagsCmd)
}
