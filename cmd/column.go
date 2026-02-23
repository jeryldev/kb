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
	Short:   "Manage columns",
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		columns, err := db.ListColumns(board.ID)
		if err != nil {
			return err
		}

		if jsonOutput {
			out := make([]columnJSON, len(columns))
			for i, col := range columns {
				count, _ := db.CountCardsInColumn(col.ID)
				out[i] = toColumnJSON(col, count)
			}
			return printJSON(out)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "POS\tNAME\tWIP LIMIT\tCARDS")
		for _, col := range columns {
			count, _ := db.CountCardsInColumn(col.ID)
			wip := "—"
			if col.WIPLimit != nil {
				wip = fmt.Sprintf("%d", *col.WIPLimit)
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%d\n", col.Position+1, col.Name, wip, count)
		}
		return w.Flush()
	},
}

var columnAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a column to the current board",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}
		col, err := db.CreateColumn(board.ID, args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toColumnJSON(col, 0))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Added column %q at position %d\n", col.Name, col.Position+1)
		return nil
	},
}

var columnReorderCmd = &cobra.Command{
	Use:   "reorder <id1,id2,...>",
	Short: "Reorder columns by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}
		ids := strings.Split(args[0], ",")
		if err := db.ReorderColumns(board.ID, ids); err != nil {
			return err
		}

		if jsonOutput {
			columns, err := db.ListColumns(board.ID)
			if err != nil {
				return err
			}
			out := make([]columnJSON, len(columns))
			for i, col := range columns {
				count, _ := db.CountCardsInColumn(col.ID)
				out[i] = toColumnJSON(col, count)
			}
			return printJSON(out)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Columns reordered")
		return nil
	},
}

var columnDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a column and all its cards",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		col, err := resolveColumnByName(board.ID, args[0])
		if err != nil {
			return err
		}

		count, _ := db.CountCardsInColumn(col.ID)

		force, _ := cmd.Flags().GetBool("force")
		if !force && !jsonOutput {
			fmt.Fprintf(cmd.OutOrStdout(), "Delete column %q with %d cards? This cannot be undone. [y/N] ", col.Name, count)
			var confirm string
			fmt.Scanln(&confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
				return nil
			}
		}

		out := toColumnJSON(col, count)
		if err := db.DeleteColumn(col.ID); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(out)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Deleted column %q\n", col.Name)
		return nil
	},
}

var columnWIPLimitCmd = &cobra.Command{
	Use:   "wip-limit <name> <limit>",
	Short: "Set or clear WIP limit for a column (0 to clear)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		col, err := resolveColumnByName(board.ID, args[0])
		if err != nil {
			return err
		}

		limit, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid limit %q: must be a number", args[1])
		}
		if limit < 0 {
			return fmt.Errorf("WIP limit must be 0 (to clear) or a positive number")
		}

		var wipLimit *int
		if limit > 0 {
			wipLimit = &limit
		}

		if err := db.UpdateColumnWIPLimit(col.ID, wipLimit); err != nil {
			return err
		}
		col.WIPLimit = wipLimit

		if jsonOutput {
			count, _ := db.CountCardsInColumn(col.ID)
			return printJSON(toColumnJSON(col, count))
		}

		if wipLimit == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Cleared WIP limit for column %q\n", col.Name)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Set WIP limit for column %q to %d\n", col.Name, *wipLimit)
		}
		return nil
	},
}

func init() {
	columnDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")

	columnCmd.AddCommand(columnAddCmd)
	columnCmd.AddCommand(columnReorderCmd)
	columnCmd.AddCommand(columnDeleteCmd)
	columnCmd.AddCommand(columnWIPLimitCmd)
	rootCmd.AddCommand(columnCmd)
}
