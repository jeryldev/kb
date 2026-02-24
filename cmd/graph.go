package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/jeryldev/kb/internal/graph"
	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Visualize the knowledge graph",
	RunE: func(cmd *cobra.Command, args []string) error {
		workspace, _ := cmd.Flags().GetString("workspace")
		open, _ := cmd.Flags().GetBool("open")

		wsID := ""
		if workspace != "" {
			ws, err := resolveWorkspace(workspace)
			if err != nil {
				return err
			}
			wsID = ws.ID
		}

		data, err := graph.BuildGraph(db, wsID)
		if err != nil {
			return err
		}

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
		title = fmt.Sprintf("kb Graph — %s", workspace)
	}

	html, err := graph.GenerateHTML(data, title)
	if err != nil {
		return fmt.Errorf("generating HTML: %w", err)
	}

	tmpDir := os.TempDir()
	outPath := filepath.Join(tmpDir, "kb-graph.html")
	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		return fmt.Errorf("writing graph file: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Graph written to %s\n", outPath)

	var openCmd string
	switch runtime.GOOS {
	case "darwin":
		openCmd = "open"
	case "linux":
		openCmd = "xdg-open"
	default:
		openCmd = "open"
	}

	return exec.Command(openCmd, outPath).Start()
}

func init() {
	graphCmd.Flags().StringP("workspace", "w", "", "Scope graph to a workspace")
	graphCmd.Flags().BoolP("open", "o", false, "Open interactive graph in browser")
	rootCmd.AddCommand(graphCmd)
}
