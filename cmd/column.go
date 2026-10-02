package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var columnCmd = &cobra.Command{
	Use:     "columns",
	Aliases: []string{"column"},
	Short:   "Manage the current board's columns",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := currentBoard()
		if err != nil {
			return err
		}
		lanes, err := db.Lanes(board.ID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return jsonList(lanes, toColumnJSON)
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "POS\tNAME\tWIP LIMIT\tCARDS")
		for _, l := range lanes {
			wip := "—"
			if l.WIPLimit != nil {
				wip = strconv.Itoa(*l.WIPLimit)
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%d\n", l.Position+1, l.Name, wip, l.Cards)
		}
		return w.Flush()
	},
}

// printColumn prints one column as it is now, after a change.
func printColumn(cmd *cobra.Command, boardID, name, message string) error {
	lane, err := resolveLane(boardID, name)
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(toColumnJSON(lane))
	}
	fmt.Fprintln(cmd.OutOrStdout(), message)
	return nil
}

var columnAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a column to the current board",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := currentBoard()
		if err != nil {
			return err
		}
		if err := db.AddLane(board.ID, args[0]); err != nil {
			return err
		}
		return printColumn(cmd, board.ID, args[0], fmt.Sprintf("Added column %q", args[0]))
	},
}

var columnRenameCmd = &cobra.Command{
	Use:   "rename <name> <new name>",
	Short: "Rename a column",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := currentBoard()
		if err != nil {
			return err
		}
		lane, err := resolveLane(board.ID, args[0])
		if err != nil {
			return err
		}
		if err := db.RenameLane(board.ID, lane.Name, args[1]); err != nil {
			return err
		}
		return printColumn(cmd, board.ID, args[1], fmt.Sprintf("Renamed column %q to %q", lane.Name, args[1]))
	},
}

var columnReorderCmd = &cobra.Command{
	Use:   "reorder <name,name,...>",
	Short: "Put the columns in a new order, naming every column once",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := currentBoard()
		if err != nil {
			return err
		}
		var names []string
		for _, n := range strings.Split(args[0], ",") {
			names = append(names, strings.TrimSpace(n))
		}
		if err := db.ReorderLanes(board.ID, names); err != nil {
			return err
		}
		lanes, err := db.Lanes(board.ID)
		if err != nil {
			return err
		}
		if jsonOutput {
			return jsonList(lanes, toColumnJSON)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Columns reordered")
		return nil
	},
}

var columnDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a column; its cards are archived",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := currentBoard()
		if err != nil {
			return err
		}
		lane, err := resolveLane(board.ID, args[0])
		if err != nil {
			return err
		}
		force, _ := cmd.Flags().GetBool("force")
		question := fmt.Sprintf("Delete column %q?", lane.Name)
		if lane.Cards > 0 {
			question = fmt.Sprintf("Delete column %q and archive its %d cards?", lane.Name, lane.Cards)
		}
		if err := confirm(cmd, force, question); err != nil {
			return err
		}
		if err := db.DeleteLane(board.ID, lane.Name, true); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toColumnJSON(lane))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted column %q\n", lane.Name)
		return nil
	},
}

var columnWIPLimitCmd = &cobra.Command{
	Use:   "wip-limit <name> <limit>",
	Short: "Set a column's card limit (0 clears it)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := currentBoard()
		if err != nil {
			return err
		}
		lane, err := resolveLane(board.ID, args[0])
		if err != nil {
			return err
		}
		limit, err := strconv.Atoi(args[1])
		if err != nil || limit < 0 {
			return fmt.Errorf("the limit must be 0 (to clear it) or a positive number, got %q", args[1])
		}
		if err := db.SetWIPLimit(board.ID, lane.Name, limit); err != nil {
			return err
		}
		message := fmt.Sprintf("Set WIP limit for column %q to %d", lane.Name, limit)
		if limit == 0 {
			message = fmt.Sprintf("Cleared WIP limit for column %q", lane.Name)
		}
		return printColumn(cmd, board.ID, lane.Name, message)
	},
}

func init() {
	columnDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")

	columnCmd.AddCommand(columnAddCmd, columnRenameCmd, columnReorderCmd, columnDeleteCmd, columnWIPLimitCmd)
	rootCmd.AddCommand(columnCmd)
}
