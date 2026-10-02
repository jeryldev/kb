package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Read the vault and report what kb finds there",
	Long: `Read the vault (KB_VAULT, default ~/notes) and report its notes and boards,
and any files kb could only partly read. kb keeps no index: it reads the
vault every time it starts, so this is only a check.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := db.Reload(); err != nil {
			return err
		}
		notes, boards, problems := len(db.ListNotes()), len(db.ListBoards()), db.Problems()
		if jsonOutput {
			return printJSON(struct {
				Vault    string   `json:"vault"`
				Notes    int      `json:"notes"`
				Boards   int      `json:"boards"`
				Problems []string `json:"problems"`
			}{db.Vault().Root(), notes, boards, orEmpty(problems)})
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Vault: %s\n", db.Vault().Root())
		fmt.Fprintf(out, "%d notes, %d boards\n", notes, boards)
		// The problems were printed as warnings when kb started.
		if len(problems) > 0 {
			fmt.Fprintf(out, "%d warnings (above)\n", len(problems))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(indexCmd)
}
