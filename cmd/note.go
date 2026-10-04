package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/jeryldev/kb/internal/model"
	"github.com/spf13/cobra"
)

var noteCmd = &cobra.Command{
	Use:     "notes",
	Aliases: []string{"note"},
	Short:   "Manage notes",
	// A stray word is a mistyped subcommand, not something to ignore while
	// listing every note.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tag, _ := cmd.Flags().GetString("tag")
		search, _ := cmd.Flags().GetString("search")
		notes := db.ListNotes()
		if search != "" {
			notes = db.SearchNotes(search)
		}
		if tag != "" {
			var tagged []*model.Note
			for _, n := range notes {
				if n.HasTag(tag) {
					tagged = append(tagged, n)
				}
			}
			notes = tagged
		}
		return printNotes(cmd, notes)
	},
}

var noteSearchCmd = &cobra.Command{
	Use:   "search <words...>",
	Short: "Search notes for all the given words",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return printNotes(cmd, db.SearchNotes(strings.Join(args, " ")))
	},
}

func printNotes(cmd *cobra.Command, notes []*model.Note) error {
	if jsonOutput {
		return jsonList(notes, toNoteJSON)
	}
	if len(notes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No notes found. Create one with: kb note create \"title\"")
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tTITLE\tTAGS\tUPDATED")
	for _, n := range notes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", n.Slug, truncateStr(n.Title, 40), strings.Join(n.TagList(), ", "), n.UpdatedAt.Format("02 Jan 2006"))
	}
	return w.Flush()
}

var noteCreateCmd = &cobra.Command{
	Use:   "create <title>",
	Short: "Create a note, in a file named after its title",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, _ := cmd.Flags().GetString("slug")
		body, _ := cmd.Flags().GetString("body")
		wsName, _ := cmd.Flags().GetString("workspace")
		workspaceID, err := workspaceIDFor(wsName)
		if err != nil {
			return err
		}
		tags, _ := cmd.Flags().GetString("tags")
		note, err := db.CreateNote(args[0], slug, body, workspaceID, (&model.Note{Tags: tags}).TagList()...)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toNoteJSON(note))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created note %q (%s)\n", note.Title, note.Path)
		return nil
	},
}

var noteShowCmd = &cobra.Command{
	Use:   "show <note>",
	Short: "Show a note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}
		if db.NotDownloaded(note.ID) {
			fmt.Fprintf(cmd.ErrOrStderr(), "kb: %s is in iCloud and not downloaded; its text is not on this machine yet (kb open downloads it)\n", note.Path)
		}
		if jsonOutput {
			return printJSON(toNoteJSON(note))
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Title: %s\n", note.Title)
		fmt.Fprintf(out, "Slug:  %s\n", note.Slug)
		fmt.Fprintf(out, "File:  %s\n", note.Path)
		if tags := note.TagList(); len(tags) > 0 {
			fmt.Fprintf(out, "Tags:  %s\n", strings.Join(tags, ", "))
		}
		if note.Body != "" {
			fmt.Fprintf(out, "\n%s\n", note.Body)
		}
		fmt.Fprintf(out, "\nCreated: %s   Updated: %s\n", note.CreatedAt.Format("02 Jan 2006"), note.UpdatedAt.Format("02 Jan 2006"))
		fmt.Fprintf(out, "ID: %s\n", note.ID)
		return nil
	},
}

var noteEditCmd = &cobra.Command{
	Use:   "edit <note>",
	Short: "Change a note's title, body or tags",
	Long: `Change a note's title, body or tags. A new title does not rename the file
or update links; kb note rename does both.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}
		if !changedAny(cmd, "title", "body", "tags") {
			return fmt.Errorf("nothing to change; give at least one of --title, --body or --tags")
		}
		if cmd.Flags().Changed("title") {
			note.Title, _ = cmd.Flags().GetString("title")
		}
		if cmd.Flags().Changed("body") {
			note.Body, _ = cmd.Flags().GetString("body")
		}
		if cmd.Flags().Changed("tags") {
			note.Tags, _ = cmd.Flags().GetString("tags")
		}
		if err := db.UpdateNote(note); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toNoteJSON(note))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated note %q (%s)\n", note.Title, note.Path)
		return nil
	},
}

var noteRenameCmd = &cobra.Command{
	Use:   "rename <note> <new title>",
	Short: "Rename a note and its file, updating links to it",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}
		renamed, changed, err := db.RenameNote(note.ID, args[1])
		if renamed == nil {
			return err
		}
		if jsonOutput {
			if err != nil {
				return err
			}
			return printJSON(toNoteJSON(renamed))
		}
		noun := "files"
		if changed == 1 {
			noun = "file"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Renamed %q to %q (%s); updated links in %d %s\n", note.Title, renamed.Title, renamed.Path, changed, noun)
		return err
	},
}

var noteDeleteCmd = &cobra.Command{
	Use:   "delete <note>",
	Short: "Move a note's file to the vault's .trash folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}
		force, _ := cmd.Flags().GetBool("force")
		if err := confirm(cmd, force, fmt.Sprintf("Move note %q (%s) to the trash?", note.Title, note.Path)); err != nil {
			return err
		}
		dest, err := db.TrashNote(note.ID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(struct {
				noteJSON
				Trashed string `json:"trashed"` // where the file is now
			}{toNoteJSON(note), dest})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Moved note %q to %s\n", note.Title, dest)
		return nil
	},
}

var noteMoveCmd = &cobra.Command{
	Use:   "move <note> [--workspace <workspace>]",
	Short: "Move a note into a workspace (without --workspace, into Default)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}
		wsName, _ := cmd.Flags().GetString("workspace")
		wsID, err := workspaceIDFor(wsName)
		if err != nil {
			return err
		}
		if note.WorkspaceID != wsID {
			if err := db.SetNoteWorkspace(note.ID, wsID); err != nil {
				return err
			}
			if note, err = db.GetNote(note.ID); err != nil {
				return err
			}
		}
		if jsonOutput {
			return printJSON(toNoteJSON(note))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Moved note %q to workspace %q\n", note.Title, workspaceName(wsID))
		return nil
	},
}

var noteBacklinksCmd = &cobra.Command{
	Use:   "backlinks <note>",
	Short: "Show the notes and cards that link to a note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}
		links := db.Backlinks(note.ID)
		if jsonOutput {
			return jsonList(links, func(l backlink) backlinkJSON {
				return backlinkJSON{SourceType: l.SourceType, SourceID: l.SourceID, Title: l.Title, Slug: l.Slug, Board: l.Board, Context: l.Context}
			})
		}
		out := cmd.OutOrStdout()
		if len(links) == 0 {
			fmt.Fprintf(out, "No backlinks to %q\n", note.Title)
			return nil
		}
		fmt.Fprintf(out, "Backlinks to %q:\n\n", note.Title)
		for _, l := range links {
			source := "[[" + l.Title + "]]"
			if l.SourceType == "card" {
				source = fmt.Sprintf("card %s on %s", l.SourceID, l.Board)
			}
			fmt.Fprintf(out, "  %s  %s\n", source, truncateStr(strings.TrimSpace(l.Context), 60))
		}
		return nil
	},
}

func init() {
	noteCmd.Flags().StringP("tag", "t", "", "Only notes with this tag")
	noteCmd.Flags().StringP("search", "s", "", "Only notes with all these words")

	noteCreateCmd.Flags().StringP("body", "b", "", "Note body")
	noteCreateCmd.Flags().StringP("tags", "t", "", "Comma-separated tags")
	noteCreateCmd.Flags().String("slug", "", "Name the file by this slug instead of the title")
	noteCreateCmd.Flags().StringP("workspace", "w", "", "Workspace for the note (default: Default)")

	noteEditCmd.Flags().StringP("title", "T", "", "New title")
	noteEditCmd.Flags().StringP("body", "b", "", "New body")
	noteEditCmd.Flags().StringP("tags", "t", "", "New tags, comma-separated")

	noteDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	noteMoveCmd.Flags().StringP("workspace", "w", "", "Workspace to move to (default: Default)")

	noteCmd.AddCommand(noteCreateCmd, noteShowCmd, noteEditCmd, noteDeleteCmd, noteBacklinksCmd, noteSearchCmd, noteRenameCmd, noteMoveCmd)
	rootCmd.AddCommand(noteCmd)
}
