package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/jeryldev/kb/internal/graph"
	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Show the links between notes, boards and cards",
	Long: `Show the knowledge graph: notes, boards and the cards that link to
something, joined by their wikilinks. With --workspace, the workspace's
notes and boards, plus whatever they link to or from elsewhere.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		workspace, _ := cmd.Flags().GetString("workspace")
		open, _ := cmd.Flags().GetBool("open")
		wsID := ""
		if workspace != "" {
			ws, err := resolveWorkspace(workspace)
			if err != nil {
				return err
			}
			wsID, workspace = ws.ID, ws.Name
		}
		data := graph.Build(db.GraphSource(), wsID)
		if jsonOutput {
			return printJSON(data)
		}
		if open {
			return openGraphHTML(cmd, data, workspace)
		}
		nodes, edges, orphans := data.Stats()
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Knowledge Graph\n")
		if workspace != "" {
			fmt.Fprintf(out, "Workspace: %s\n", workspace)
		}
		fmt.Fprintf(out, "\n  Nodes:   %d\n  Edges:   %d\n  Orphans: %d\n", nodes, edges, orphans)
		if nodes > 0 {
			fmt.Fprintf(out, "\nOpen interactive visualization: kb graph --open\n")
		}
		return nil
	},
}

func openGraphHTML(cmd *cobra.Command, data *graph.GraphData, workspace string) error {
	title := "kb Knowledge Graph"
	if workspace != "" {
		title = "kb Graph — " + workspace
	}
	page, err := graph.GenerateHTML(data, title)
	if err != nil {
		return fmt.Errorf("generating HTML: %w", err)
	}
	f, err := os.CreateTemp("", "kb-graph-*.html")
	if err != nil {
		return err
	}
	if _, err := f.WriteString(page); err != nil {
		f.Close()
		return fmt.Errorf("writing %s: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Graph written to %s\n", f.Name())
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, f.Name()).Start()
}

func init() {
	graphCmd.Flags().StringP("workspace", "w", "", "Only this workspace, with its outside links")
	graphCmd.Flags().BoolP("open", "o", false, "Open the interactive graph in a browser")
	rootCmd.AddCommand(graphCmd)
}
