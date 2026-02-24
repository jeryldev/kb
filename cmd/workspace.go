package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/jeryldev/kb/internal/model"
	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage workspaces (PARA method)",
	RunE: func(cmd *cobra.Command, args []string) error {
		kindFilter, _ := cmd.Flags().GetString("kind")

		var workspaces []*model.Workspace
		var err error

		if kindFilter != "" {
			kind, parseErr := model.ParseWorkspaceKind(kindFilter)
			if parseErr != nil {
				return parseErr
			}
			workspaces, err = db.ListWorkspacesByKind(kind)
		} else {
			workspaces, err = db.ListWorkspaces()
		}
		if err != nil {
			return err
		}

		if len(workspaces) == 0 {
			if jsonOutput {
				return printJSON([]workspaceJSON{})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "No workspaces found. Create one with: kb workspace create \"name\" --kind project")
			return nil
		}

		if jsonOutput {
			out := make([]workspaceJSON, len(workspaces))
			for i, ws := range workspaces {
				out[i] = toWorkspaceJSON(ws)
			}
			return printJSON(out)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KIND\tNAME\tDESCRIPTION\tUPDATED")
		for _, ws := range workspaces {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				ws.Kind.Label(), ws.Name, truncateStr(ws.Description, 40),
				ws.UpdatedAt.Format("02 Jan 2006"))
		}
		return w.Flush()
	},
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		kindStr, _ := cmd.Flags().GetString("kind")
		kind, err := model.ParseWorkspaceKind(kindStr)
		if err != nil {
			return err
		}
		description, _ := cmd.Flags().GetString("description")
		path, _ := cmd.Flags().GetString("path")

		ws, err := db.CreateWorkspace(name, kind, description, path)
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
	Use:   "show <name-or-id>",
	Short: "Show workspace details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspace(args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toWorkspaceJSON(ws))
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
		fmt.Fprintf(out, "\nCreated: %s   Updated: %s\n",
			ws.CreatedAt.Format("02 Jan 2006"), ws.UpdatedAt.Format("02 Jan 2006"))
		fmt.Fprintf(out, "ID: %s\n", ws.ID)

		boards, err := db.ListBoardsByWorkspace(ws.ID)
		if err == nil && len(boards) > 0 {
			fmt.Fprintf(out, "\nBoards (%d):\n", len(boards))
			for _, b := range boards {
				fmt.Fprintf(out, "  - %s\n", b.Name)
			}
		}

		notes, err := db.ListNotesByWorkspace(ws.ID)
		if err == nil && len(notes) > 0 {
			fmt.Fprintf(out, "\nNotes (%d):\n", len(notes))
			for _, n := range notes {
				fmt.Fprintf(out, "  - [[%s]] %s\n", n.Slug, n.Title)
			}
		}

		return nil
	},
}

var workspaceEditCmd = &cobra.Command{
	Use:   "edit <name-or-id>",
	Short: "Edit a workspace's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspace(args[0])
		if err != nil {
			return err
		}

		if cmd.Flags().Changed("name") {
			ws.Name, _ = cmd.Flags().GetString("name")
		}
		if cmd.Flags().Changed("description") {
			ws.Description, _ = cmd.Flags().GetString("description")
		}
		if cmd.Flags().Changed("path") {
			ws.Path, _ = cmd.Flags().GetString("path")
		}
		if cmd.Flags().Changed("kind") {
			kindStr, _ := cmd.Flags().GetString("kind")
			kind, parseErr := model.ParseWorkspaceKind(kindStr)
			if parseErr != nil {
				return parseErr
			}
			ws.Kind = kind
		}

		if err := db.UpdateWorkspace(ws); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toWorkspaceJSON(ws))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Updated workspace %q\n", ws.Name)
		return nil
	},
}

var workspaceArchiveCmd = &cobra.Command{
	Use:   "archive <name-or-id>",
	Short: "Archive a workspace (change kind to archive)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := resolveWorkspace(args[0])
		if err != nil {
			return err
		}

		if err := db.ArchiveWorkspace(ws.ID); err != nil {
			return err
		}

		if jsonOutput {
			ws.Kind = model.KindArchive
			return printJSON(toWorkspaceJSON(ws))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Archived workspace %q\n", ws.Name)
		return nil
	},
}

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete <name-or-id>",
	Short: "Delete a workspace",
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

var boardMoveCmd = &cobra.Command{
	Use:   "move <board-name> --workspace <workspace-name>",
	Short: "Move a board into a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := db.GetBoardByName(args[0])
		if err != nil {
			return err
		}
		if board == nil {
			return fmt.Errorf("board %q not found", args[0])
		}

		wsName, _ := cmd.Flags().GetString("workspace")
		if wsName == "" {
			if err := db.SetBoardWorkspace(board.ID, nil); err != nil {
				return err
			}
			if jsonOutput {
				board.WorkspaceID = nil
				return printJSON(toBoardJSON(board))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed board %q from workspace\n", board.Name)
			return nil
		}

		ws, err := resolveWorkspace(wsName)
		if err != nil {
			return err
		}

		if err := db.SetBoardWorkspace(board.ID, &ws.ID); err != nil {
			return err
		}

		if jsonOutput {
			board.WorkspaceID = &ws.ID
			return printJSON(toBoardJSON(board))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Moved board %q to workspace %q\n", board.Name, ws.Name)
		return nil
	},
}

var noteMoveCmd = &cobra.Command{
	Use:   "move <slug-or-id> --workspace <workspace-name>",
	Short: "Move a note into a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}

		wsName, _ := cmd.Flags().GetString("workspace")
		if wsName == "" {
			if err := db.SetNoteWorkspace(note.ID, nil); err != nil {
				return err
			}
			if jsonOutput {
				note.WorkspaceID = nil
				return printJSON(toNoteJSON(note))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed note %q from workspace\n", note.Title)
			return nil
		}

		ws, err := resolveWorkspace(wsName)
		if err != nil {
			return err
		}

		if err := db.SetNoteWorkspace(note.ID, &ws.ID); err != nil {
			return err
		}

		if jsonOutput {
			note.WorkspaceID = &ws.ID
			return printJSON(toNoteJSON(note))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Moved note %q to workspace %q\n", note.Title, ws.Name)
		return nil
	},
}

func resolveWorkspace(ref string) (*model.Workspace, error) {
	ws, err := db.GetWorkspaceByName(ref)
	if err == nil {
		return ws, nil
	}
	ws, err = db.GetWorkspace(ref)
	if err == nil {
		return ws, nil
	}
	return nil, fmt.Errorf("workspace %q not found", ref)
}

func init() {
	workspaceCmd.Flags().StringP("kind", "k", "", "Filter by kind (project, area, resource, archive)")

	workspaceCreateCmd.Flags().StringP("kind", "k", "project", "Workspace kind (project, area, resource, archive)")
	workspaceCreateCmd.Flags().StringP("description", "d", "", "Workspace description")
	workspaceCreateCmd.Flags().StringP("path", "p", "", "Associated file path")

	workspaceEditCmd.Flags().StringP("name", "n", "", "New name")
	workspaceEditCmd.Flags().StringP("description", "d", "", "New description")
	workspaceEditCmd.Flags().StringP("path", "p", "", "New path")
	workspaceEditCmd.Flags().StringP("kind", "k", "", "New kind")

	boardMoveCmd.Flags().StringP("workspace", "w", "", "Target workspace (empty to unassign)")

	noteMoveCmd.Flags().StringP("workspace", "w", "", "Target workspace (empty to unassign)")

	workspaceCmd.AddCommand(workspaceCreateCmd)
	workspaceCmd.AddCommand(workspaceShowCmd)
	workspaceCmd.AddCommand(workspaceEditCmd)
	workspaceCmd.AddCommand(workspaceArchiveCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
	rootCmd.AddCommand(workspaceCmd)

	boardCmd.AddCommand(boardMoveCmd)
	noteCmd.AddCommand(noteMoveCmd)
}
