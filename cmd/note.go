package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/jeryldev/kb/internal/model"
	"github.com/spf13/cobra"
)

var noteCmd = &cobra.Command{
	Use:     "notes",
	Aliases: []string{"note"},
	Short:   "Manage notes",
	RunE: func(cmd *cobra.Command, args []string) error {
		tag, _ := cmd.Flags().GetString("tag")
		search, _ := cmd.Flags().GetString("search")

		var notes []*model.Note
		var err error

		switch {
		case search != "":
			notes, err = db.SearchNotes(search)
		case tag != "":
			notes, err = db.ListNotesByTag(tag)
		default:
			notes, err = db.ListNotes()
		}
		if err != nil {
			return err
		}

		if len(notes) == 0 {
			if jsonOutput {
				return printJSON([]noteJSON{})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "No notes found. Create one with: kb note create \"title\"")
			return nil
		}

		if jsonOutput {
			out := make([]noteJSON, len(notes))
			for i, n := range notes {
				out[i] = toNoteJSON(n)
			}
			return printJSON(out)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SLUG\tTITLE\tTAGS\tUPDATED")
		for _, n := range notes {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				n.Slug, truncateStr(n.Title, 40), n.Tags,
				n.UpdatedAt.Format("02 Jan 2006"))
		}
		return w.Flush()
	},
}

var noteCreateCmd = &cobra.Command{
	Use:   "create <title>",
	Short: "Create a new note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := args[0]
		slug, _ := cmd.Flags().GetString("slug")
		if slug == "" {
			slug = model.Slugify(title)
		}

		body, _ := cmd.Flags().GetString("body")

		wsName, _ := cmd.Flags().GetString("workspace")
		workspaceID, err := resolveWorkspaceIDForCreate(wsName)
		if err != nil {
			return err
		}

		note, err := db.CreateNote(title, slug, body, workspaceID)
		if err != nil {
			return err
		}

		if cmd.Flags().Changed("tags") {
			note.Tags, _ = cmd.Flags().GetString("tags")
			if err := db.UpdateNote(note); err != nil {
				return err
			}
		}

		if err := db.SyncNoteLinks(note); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toNoteJSON(note))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Created note %q (slug: %s)\n", note.Title, note.Slug)
		return nil
	},
}

var noteShowCmd = &cobra.Command{
	Use:   "show <slug-or-id>",
	Short: "Show note details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toNoteJSON(note))
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Title: %s\n", note.Title)
		fmt.Fprintf(out, "Slug:  %s\n", note.Slug)
		if note.Tags != "" {
			fmt.Fprintf(out, "Tags:  %s\n", note.Tags)
		}
		if note.Body != "" {
			fmt.Fprintf(out, "\n%s\n", note.Body)
		}
		fmt.Fprintf(out, "\nCreated: %s   Updated: %s\n",
			note.CreatedAt.Format("02 Jan 2006"), note.UpdatedAt.Format("02 Jan 2006"))
		fmt.Fprintf(out, "ID: %s\n", note.ID)
		return nil
	},
}

var noteEditCmd = &cobra.Command{
	Use:   "edit <slug-or-id>",
	Short: "Edit a note's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
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

		if err := db.SyncNoteLinks(note); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toNoteJSON(note))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Updated note %q (slug: %s)\n", note.Title, note.Slug)
		return nil
	},
}

var noteDeleteCmd = &cobra.Command{
	Use:   "delete <slug-or-id>",
	Short: "Delete a note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}

		if err := db.DeleteNote(note.ID); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toNoteJSON(note))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Deleted note %q\n", note.Title)
		return nil
	},
}

var noteBacklinksCmd = &cobra.Command{
	Use:   "backlinks <slug-or-id>",
	Short: "Show notes and cards that link to this note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}

		links, err := db.GetBacklinks("note", note.ID)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := make([]backlinkJSON, len(links))
			for i, l := range links {
				out[i] = backlinkJSON{
					SourceType: l.SourceType,
					SourceID:   l.SourceID,
					Context:    l.Context,
				}
			}
			return printJSON(out)
		}

		if len(links) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "No backlinks to %q\n", note.Slug)
			return nil
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Backlinks to %q:\n\n", note.Slug)
		for _, l := range links {
			if l.SourceType == "note" {
				source, err := db.GetNote(l.SourceID)
				if err == nil {
					fmt.Fprintf(out, "  [[%s]] %s\n", source.Slug, truncateStr(l.Context, 60))
				}
			}
		}
		return nil
	},
}

func resolveNote(ref string) (*model.Note, error) {
	note, err := db.GetNoteBySlug(ref)
	if err == nil {
		return note, nil
	}
	if len(ref) >= 4 {
		id, err := db.ResolveNoteID(ref)
		if err == nil {
			return db.GetNote(id)
		}
	}
	return nil, fmt.Errorf("note %q not found", ref)
}

func init() {
	noteCmd.Flags().StringP("tag", "t", "", "Filter by tag")
	noteCmd.Flags().StringP("search", "s", "", "Search in title, body, and tags")

	noteCreateCmd.Flags().StringP("body", "b", "", "Note body content")
	noteCreateCmd.Flags().StringP("tags", "t", "", "Comma-separated tags")
	noteCreateCmd.Flags().String("slug", "", "Custom slug (default: auto-generated from title)")
	noteCreateCmd.Flags().StringP("workspace", "w", "", "Workspace to assign the note to (default: Default)")

	noteEditCmd.Flags().StringP("title", "T", "", "New title")
	noteEditCmd.Flags().StringP("body", "b", "", "New body content")
	noteEditCmd.Flags().StringP("tags", "t", "", "New tags (comma-separated)")

	noteCmd.AddCommand(noteCreateCmd)
	noteCmd.AddCommand(noteShowCmd)
	noteCmd.AddCommand(noteEditCmd)
	noteCmd.AddCommand(noteDeleteCmd)
	noteCmd.AddCommand(noteBacklinksCmd)
	rootCmd.AddCommand(noteCmd)
}
