package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/jeryldev/kb/internal/model"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"workspaces", "ws"},
	Short:   "Manage workspaces (PARA method)",
	Long: `Workspaces group notes and boards. They are listed in the vault's
.kb/workspaces.yml, and a note or board names its workspace in its
frontmatter (workspace: Name); one that names none is in Default.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		workspaces := db.ListWorkspaces()
		if kindFilter, _ := cmd.Flags().GetString("kind"); kindFilter != "" {
			kind, err := model.ParseWorkspaceKind(kindFilter)
			if err != nil {
				return err
			}
			var matching []*model.Workspace
			for _, ws := range workspaces {
				if ws.Kind == kind {
					matching = append(matching, ws)
				}
			}
			workspaces = matching
		}
		if jsonOutput {
			return jsonList(workspaces, toWorkspaceJSON)
		}
		if len(workspaces) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No workspaces of that kind.")
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KIND\tNAME\tDESCRIPTION")
		for _, ws := range workspaces {
			fmt.Fprintf(w, "%s\t%s\t%s\n", ws.Kind.Label(), ws.Name, truncateStr(ws.Description, 40))
		}
		return w.Flush()
	},
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kindStr, _ := cmd.Flags().GetString("kind")
		kind, err := model.ParseWorkspaceKind(kindStr)
		if err != nil {
			return err
		}
		description, _ := cmd.Flags().GetString("description")
		path, _ := cmd.Flags().GetString("path")
		ws, err := db.CreateWorkspace(args[0], kind, description, path)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toWorkspaceJSON(ws))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created workspace %s %q\n", ws.Kind.Label(), ws.Name)
		return nil
	},
}

var workspaceShowCmd = &cobra.Command{
	Use:   "show <workspace>",
	Short: "Show a workspace and what is in it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspace(args[0])
		if err != nil {
			return err
		}
		var boards []*model.Board
		for _, b := range db.ListBoards() {
			if b.WorkspaceID == ws.ID {
				boards = append(boards, b)
			}
		}
		notes := db.ListNotesByWorkspace(ws.ID)
		if jsonOutput {
			show := struct {
				workspaceJSON
				Boards []string `json:"boards"` // board names
				Notes  []string `json:"notes"`  // note slugs
			}{toWorkspaceJSON(ws), []string{}, []string{}}
			for _, b := range boards {
				show.Boards = append(show.Boards, b.Name)
			}
			for _, n := range notes {
				show.Notes = append(show.Notes, n.Slug)
			}
			return printJSON(show)
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Name: %s\n", ws.Name)
		fmt.Fprintf(out, "Kind: %s %s\n", ws.Kind.Label(), ws.Kind)
		if ws.Description != "" {
			fmt.Fprintf(out, "Desc: %s\n", ws.Description)
		}
		if ws.Path != "" {
			fmt.Fprintf(out, "Path: %s\n", ws.Path)
		}
		if len(boards) > 0 {
			fmt.Fprintf(out, "\nBoards (%d):\n", len(boards))
			for _, b := range boards {
				fmt.Fprintf(out, "  - %s\n", b.Name)
			}
		}
		if len(notes) > 0 {
			fmt.Fprintf(out, "\nNotes (%d):\n", len(notes))
			for _, n := range notes {
				fmt.Fprintf(out, "  - %s\n", n.Title)
			}
		}
		fmt.Fprintf(out, "\nID: %s\n", ws.ID)
		return nil
	},
}

var workspaceEditCmd = &cobra.Command{
	Use:   "edit <workspace>",
	Short: "Change a workspace; a new name is written into its notes and boards",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspace(args[0])
		if err != nil {
			return err
		}
		if !changedAny(cmd, "name", "description", "path", "kind") {
			return fmt.Errorf("nothing to change; give at least one of --name, --description, --path or --kind")
		}
		updated := *ws
		if cmd.Flags().Changed("name") {
			updated.Name, _ = cmd.Flags().GetString("name")
		}
		if cmd.Flags().Changed("description") {
			updated.Description, _ = cmd.Flags().GetString("description")
		}
		if cmd.Flags().Changed("path") {
			updated.Path, _ = cmd.Flags().GetString("path")
		}
		if cmd.Flags().Changed("kind") {
			kindStr, _ := cmd.Flags().GetString("kind")
			if updated.Kind, err = model.ParseWorkspaceKind(kindStr); err != nil {
				return err
			}
		}
		if err := db.UpdateWorkspace(&updated); err != nil {
			return err
		}
		fresh, err := db.GetWorkspace(ws.ID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toWorkspaceJSON(fresh))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated workspace %q\n", fresh.Name)
		return nil
	},
}

var workspaceArchiveCmd = &cobra.Command{
	Use:   "archive <workspace>",
	Short: "Archive a workspace (change its kind to archive)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspace(args[0])
		if err != nil {
			return err
		}
		if err := db.ArchiveWorkspace(ws.ID); err != nil {
			return err
		}
		if ws, err = db.GetWorkspace(ws.ID); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toWorkspaceJSON(ws))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Archived workspace %q\n", ws.Name)
		return nil
	},
}

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete <workspace>",
	Short: "Delete a workspace that nothing uses",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspace(args[0])
		if err != nil {
			return err
		}
		if err := db.DeleteWorkspace(ws.ID); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toWorkspaceJSON(ws))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted workspace %q\n", ws.Name)
		return nil
	},
}

func init() {
	workspaceCmd.Flags().StringP("kind", "k", "", "Only workspaces of this kind (project, area, resource, archive)")

	workspaceCreateCmd.Flags().StringP("kind", "k", "project", "Kind (project, area, resource, archive)")
	workspaceCreateCmd.Flags().StringP("description", "d", "", "Description")
	workspaceCreateCmd.Flags().StringP("path", "p", "", "A folder on this machine for the workspace")

	workspaceEditCmd.Flags().StringP("name", "n", "", "New name")
	workspaceEditCmd.Flags().StringP("description", "d", "", "New description")
	workspaceEditCmd.Flags().StringP("path", "p", "", "New folder")
	workspaceEditCmd.Flags().StringP("kind", "k", "", "New kind (project, area, resource, archive)")

	workspaceCmd.AddCommand(workspaceCreateCmd, workspaceShowCmd, workspaceEditCmd, workspaceArchiveCmd, workspaceDeleteCmd)
	rootCmd.AddCommand(workspaceCmd)
}
