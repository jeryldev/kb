package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/publish"
	"github.com/jeryldev/kb/internal/store"
	"github.com/spf13/cobra"
)

var publishCmd = &cobra.Command{
	Use:   "publish <note-slug-or-id>",
	Short: "Publish a note to a configured target",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		note, err := resolveNote(args[0])
		if err != nil {
			return err
		}

		targetName, _ := cmd.Flags().GetString("target")
		target, err := resolvePublishTarget(targetName)
		if err != nil {
			return err
		}

		draft, _ := cmd.Flags().GetBool("draft")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		posts, err := db.GetPublishedPosts(target.ID)
		if err != nil {
			return fmt.Errorf("getting published posts: %w", err)
		}

		// A post is dated by the day the note was written, on the writer's
		// calendar, and a note published before keeps its post, so
		// republishing updates it in place.
		date := note.CreatedAt.In(time.Local)
		relPath := ""
		prev, err := db.GetLatestPublishLog(note.ID, target.ID)
		if err != nil {
			return err
		}
		if prev != nil {
			if prevDate, ok := publish.PostDateFromPath(prev.FilePath); ok {
				date, relPath = prevDate, prev.FilePath
			}
		}
		if relPath == "" {
			relPath = freePostPath(target.BasePath, target.PostsDir, note, date, posts)
		}

		// Link only to posts that are out: a draft's URL does not exist yet.
		permalinks := make(map[string]string, len(posts))
		for noteID, post := range posts {
			if post.Draft {
				continue
			}
			if permalink, ok := publish.PermalinkFromPostPath(post.Path); ok {
				permalinks[noteID] = permalink
			}
		}

		content := publish.GeneratePost(note, date, draft, permalinks, db)
		fullPath := filepath.Join(target.BasePath, relPath)

		if dryRun {
			if jsonOutput {
				return printJSON(struct {
					FilePath string `json:"file_path"`
					Content  string `json:"content"`
					Draft    bool   `json:"draft"`
				}{fullPath, content, draft})
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Would write to: %s\n\n", fullPath)
			fmt.Fprint(out, content)
			return nil
		}

		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}

		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing file %s: %w", fullPath, err)
		}

		frontMatter := publish.GenerateFrontMatter(note, date, draft)
		pl, err := db.CreatePublishLog(note.ID, target.ID, relPath, frontMatter)
		if err != nil {
			return fmt.Errorf("recording publish log: %w", err)
		}

		if jsonOutput {
			return printJSON(toPublishLogJSON(pl, note.Slug))
		}

		label := "Published"
		if draft {
			label = "Published (draft)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %q to %s\n", label, note.Title, fullPath)
		return nil
	},
}

var publishSetupCmd = &cobra.Command{
	Use:   "setup <name>",
	Short: "Configure a publish target",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		engineStr, _ := cmd.Flags().GetString("engine")
		engine, err := model.ParseEngine(engineStr)
		if err != nil {
			return err
		}
		basePath, _ := cmd.Flags().GetString("path")
		if basePath == "" {
			return fmt.Errorf("--path is required")
		}
		postsDir, _ := cmd.Flags().GetString("posts-dir")

		var wsID *string
		if wsName, _ := cmd.Flags().GetString("workspace"); wsName != "" {
			ws, wsErr := resolveWorkspace(wsName)
			if wsErr != nil {
				return wsErr
			}
			wsID = &ws.ID
		}

		pt, err := db.CreatePublishTarget(name, engine, basePath, postsDir, wsID)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toPublishTargetJSON(pt))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Created publish target %q (%s) at %s\n", pt.Name, pt.Engine, pt.BasePath)
		return nil
	},
}

var publishListCmd = &cobra.Command{
	Use:   "list",
	Short: "List publish targets and recent publications",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetName, _ := cmd.Flags().GetString("target")

		if targetName != "" {
			target, err := resolvePublishTarget(targetName)
			if err != nil {
				return err
			}
			logs, err := db.ListPublishLogs(target.ID)
			if err != nil {
				return err
			}

			if jsonOutput {
				out := make([]publishLogJSON, len(logs))
				for i, l := range logs {
					slug := resolveNoteSlug(l.NoteID)
					out[i] = toPublishLogJSON(l, slug)
				}
				return printJSON(out)
			}

			if len(logs) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No publications for target %q\n", target.Name)
				return nil
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Publications for %q:\n\n", target.Name)
			for _, l := range logs {
				slug := resolveNoteSlug(l.NoteID)
				fmt.Fprintf(out, "  %s  %s  %s\n",
					l.PublishedAt.Format("02 Jan 2006"),
					slug,
					l.FilePath)
			}
			return nil
		}

		targets, err := db.ListPublishTargets()
		if err != nil {
			return err
		}

		if len(targets) == 0 {
			if jsonOutput {
				return printJSON([]publishTargetJSON{})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "No publish targets. Set one up with: kb publish setup \"name\" --engine jekyll --path /path/to/site")
			return nil
		}

		if jsonOutput {
			out := make([]publishTargetJSON, len(targets))
			for i, pt := range targets {
				out[i] = toPublishTargetJSON(pt)
			}
			return printJSON(out)
		}

		out := cmd.OutOrStdout()
		for _, pt := range targets {
			fmt.Fprintf(out, "%s  %s  %s/%s\n", pt.Name, pt.Engine, pt.BasePath, pt.PostsDir)
		}
		return nil
	},
}

var publishDeleteCmd = &cobra.Command{
	Use:   "delete <target-name>",
	Short: "Delete a publish target",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := resolvePublishTarget(args[0])
		if err != nil {
			return err
		}

		if err := db.DeletePublishTarget(target.ID); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toPublishTargetJSON(target))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Deleted publish target %q\n", target.Name)
		return nil
	},
}

func resolvePublishTarget(nameOrID string) (*model.PublishTarget, error) {
	if nameOrID == "" {
		targets, err := db.ListPublishTargets()
		if err != nil {
			return nil, err
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("no publish targets configured; run: kb publish setup")
		}
		if len(targets) == 1 {
			return targets[0], nil
		}
		return nil, fmt.Errorf("multiple publish targets exist; specify one with --target")
	}
	pt, err := db.GetPublishTargetByName(nameOrID)
	if err == nil {
		return pt, nil
	}
	pt, err = db.GetPublishTarget(nameOrID)
	if err == nil {
		return pt, nil
	}
	return nil, fmt.Errorf("publish target %q not found", nameOrID)
}

func resolveNoteSlug(noteID string) string {
	note, err := db.GetNote(noteID)
	if err != nil {
		return noteID[:8]
	}
	return note.Slug
}

// freePostPath is the post path for a note's first publish, with -2, -3
// added to the slug when another note's post (one renamed since, say) or a
// file kb did not write already has the name.
func freePostPath(basePath, postsDir string, note *model.Note, date time.Time, posts map[string]store.PublishedPost) string {
	taken := map[string]bool{}
	for noteID, post := range posts {
		if noteID != note.ID {
			taken[post.Path] = true
		}
	}
	slug := note.Slug
	for n := 2; ; n++ {
		relPath := publish.PostFilePath(postsDir, slug, date)
		if _, err := os.Stat(filepath.Join(basePath, relPath)); !taken[relPath] && os.IsNotExist(err) {
			return relPath
		}
		slug = fmt.Sprintf("%s-%d", note.Slug, n)
	}
}

func init() {
	publishCmd.Flags().StringP("target", "t", "", "Publish target name (auto-selects if only one exists)")
	publishCmd.Flags().Bool("draft", false, "Publish as draft (published: false)")
	publishCmd.Flags().Bool("dry-run", false, "Preview output without writing file")

	publishSetupCmd.Flags().StringP("engine", "e", "jekyll", "Publishing engine")
	publishSetupCmd.Flags().StringP("path", "p", "", "Base path to the site directory")
	publishSetupCmd.Flags().String("posts-dir", "_posts", "Posts directory within the site")
	publishSetupCmd.Flags().StringP("workspace", "w", "", "Associated workspace")

	publishListCmd.Flags().StringP("target", "t", "", "Show publications for specific target")

	publishCmd.AddCommand(publishSetupCmd)
	publishCmd.AddCommand(publishListCmd)
	publishCmd.AddCommand(publishDeleteCmd)
	rootCmd.AddCommand(publishCmd)
}
