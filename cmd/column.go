package cmd

import (
	"fmt"
	"os"
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

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
		fmt.Printf("Added column %q at position %d\n", col.Name, col.Position+1)
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
		fmt.Println("Columns reordered")
		return nil
	},
}

func init() {
	columnCmd.AddCommand(columnAddCmd)
	columnCmd.AddCommand(columnReorderCmd)
	rootCmd.AddCommand(columnCmd)
}
