package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Rescan the notes vault",
	Long: `Rescan the notes vault (KB_VAULT, default ~/notes) and update kb's index.

kb rescans on every start, so this is only needed to see what changed or,
with --rebuild, to re-read every file. Notes are the Markdown files in the
vault; the index can always be rebuilt from them.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		rebuild, _ := cmd.Flags().GetBool("rebuild")
		scan := db.Scan
		if rebuild {
			scan = db.Rebuild
		}
		res, err := scan()
		if err != nil {
			return err
		}
		// kb scanned when it started, a moment ago; count those changes too.
		// A rebuild re-reads everything anyway, so its counts stand alone.
		if !rebuild {
			opened := db.OpenScan()
			res.Added += opened.Added
			res.Updated += opened.Updated
			res.Removed += opened.Removed
		}
		notes, err := db.ListNotes()
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Vault: %s\n", db.Vault().Root())
		fmt.Fprintf(out, "%d notes (%d added, %d updated, %d removed)\n",
			len(notes), res.Added, res.Updated, res.Removed)
		// Problems from the scan on open were already printed at startup.
		for _, problem := range res.Problems {
			fmt.Fprintf(out, "warning: %s\n", problem)
		}
		return nil
	},
}

func init() {
	indexCmd.Flags().Bool("rebuild", false, "Re-read every file, not just changed ones")
	rootCmd.AddCommand(indexCmd)
}
