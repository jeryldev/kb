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
	RunE: func(cmd *cobra.Command, args []string) error {
		boards, err := db.ListBoards()
		if err != nil {
			return err
		}
		if len(boards) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No boards found. Create one with: kb board create <name>")
			return nil
		}

		if jsonOutput {
			out := make([]boardJSON, len(boards))
			for i, b := range boards {
				out[i] = toBoardJSON(b)
			}
			return printJSON(out)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tDESCRIPTION\tCREATED")
		for _, b := range boards {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", b.ID[:8], b.Name, b.Description, b.CreatedAt.Format("02 Jan 2006"))
		}
		return w.Flush()
	},
}

var boardCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new board with default columns",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		desc, _ := cmd.Flags().GetString("description")
		board, err := db.CreateBoard(args[0], desc)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toBoardJSON(board))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Created board %q (id: %s)\n", board.Name, board.ID[:8])
		return nil
	},
}

var boardDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a board and all its data",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := db.GetBoardByName(args[0])
		if err != nil {
			return err
		}
		if board == nil {
			return fmt.Errorf("board %q not found", args[0])
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force && !jsonOutput {
			fmt.Fprintf(cmd.OutOrStdout(), "Delete board %q and all its cards? This cannot be undone. [y/N] ", board.Name)
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
				return nil
			}
		}

		out := toBoardJSON(board)
		if err := db.DeleteBoard(board.ID); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(out)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Deleted board %q\n", board.Name)
		return nil
	},
}

func init() {
	boardCreateCmd.Flags().StringP("description", "d", "", "Board description")
	boardDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")

	boardCmd.AddCommand(boardCreateCmd)
	boardCmd.AddCommand(boardDeleteCmd)
	rootCmd.AddCommand(boardCmd)
}
