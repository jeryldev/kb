package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var boardCmd = &cobra.Command{
	Use:     "boards",
	Aliases: []string{"board"},
	Short:   "Manage boards",
	Long: `Boards are Markdown files in the Obsidian Kanban plugin's format, in the
vault folder named by KB_BOARDS_DIR (default "Boards"). Any file with
kanban-plugin: board in its frontmatter is a board, wherever it is.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		boards := db.ListBoards()
		if jsonOutput {
			return jsonList(boards, toBoardJSON)
		}
		if len(boards) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No boards found. Create one with: kb board create <name>")
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tWORKSPACE\tFILE\tDESCRIPTION")
		for _, b := range boards {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", b.Name, workspaceName(b.WorkspaceID), b.ID, truncateStr(b.Description, 40))
		}
		return w.Flush()
	},
}

var boardCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a board with the usual columns",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		desc, _ := cmd.Flags().GetString("description")
		wsName, _ := cmd.Flags().GetString("workspace")
		workspaceID, err := workspaceIDFor(wsName)
		if err != nil {
			return err
		}
		board, err := db.CreateBoard(args[0], desc, workspaceID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toBoardJSON(board))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created board %q (%s)\n", board.Name, board.ID)
		return nil
	},
}

var boardDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Move a board's file to the vault's .trash folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := db.GetBoard(args[0])
		if err != nil {
			return err
		}
		force, _ := cmd.Flags().GetBool("force")
		if err := confirm(cmd, force, fmt.Sprintf("Move board %q (%s) to the trash?", board.Name, board.ID)); err != nil {
			return err
		}
		dest, err := db.TrashBoard(board.ID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toBoardJSON(board))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Moved board %q to %s\n", board.Name, dest)
		return nil
	},
}

var boardMoveCmd = &cobra.Command{
	Use:   "move <board> [--workspace <workspace>]",
	Short: "Move a board into a workspace (without --workspace, into Default)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := db.GetBoard(args[0])
		if err != nil {
			return err
		}
		wsName, _ := cmd.Flags().GetString("workspace")
		wsID, err := workspaceIDFor(wsName)
		if err != nil {
			return err
		}
		if err := db.SetBoardWorkspace(board.ID, wsID); err != nil {
			return err
		}
		if board, err = db.GetBoard(board.ID); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toBoardJSON(board))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Moved board %q to workspace %q\n", board.Name, workspaceName(wsID))
		return nil
	},
}

func init() {
	boardCreateCmd.Flags().StringP("description", "d", "", "Board description")
	boardCreateCmd.Flags().StringP("workspace", "w", "", "Workspace for the board (default: Default)")
	boardDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
	boardMoveCmd.Flags().StringP("workspace", "w", "", "Workspace to move to (default: Default)")

	boardCmd.AddCommand(boardCreateCmd, boardDeleteCmd, boardMoveCmd)
	rootCmd.AddCommand(boardCmd)
}
