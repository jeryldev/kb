package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/publish"
	"github.com/spf13/cobra"
)

var publishCmd = &cobra.Command{
	Use:   "publish <note>",
	Short: "Publish a note to a site",
	Long: `Publish a note as a post to a site set up with kb publish setup.

Where each note was published is kept in its frontmatter (published:), so
publishing again updates the same post. Sites are machine-local settings,
kept in ~/.config/kb/publish.yml.

A note named like a subcommand (list, setup, delete) goes after --:
  kb publish -- list`,
	Args: cobra.ExactArgs(1),
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

		posts := db.PublishedPosts(target.Name)

		// A post is dated by the day the note was written, on the writer's
		// calendar, and a note published before keeps its post, so
		// republishing updates it in place.
		date := note.CreatedAt.In(time.Local)
		relPath := ""
		if prev, ok := db.LatestPost(note.ID, target.Name); ok {
			if prevDate, ok := publish.PostDateFromPath(prev.Path); ok {
				date, relPath = prevDate, prev.Path
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
			if permalink, ok := publish.PermalinkFor(target.Permalink, post.Path); ok {
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

		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", fullPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", fullPath, err)
		}
		if err := db.RecordPublish(note.ID, target.Name, relPath, draft); err != nil {
			return fmt.Errorf("wrote %s but could not record it in the note: %w", fullPath, err)
		}

		if jsonOutput {
			return printJSON(publicationJSON{Note: note.Slug, Target: target.Name, FilePath: relPath, Draft: draft})
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
	Short: "Set up a site to publish to",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		engineStr, _ := cmd.Flags().GetString("engine")
		engine, err := model.ParseEngine(engineStr)
		if err != nil {
			return err
		}
		basePath, _ := cmd.Flags().GetString("path")
		if basePath == "" {
			return fmt.Errorf("--path is required: the site's folder")
		}
		postsDir, _ := cmd.Flags().GetString("posts-dir")
		wsID := ""
		if wsName, _ := cmd.Flags().GetString("workspace"); wsName != "" {
			ws, err := resolveWorkspace(wsName)
			if err != nil {
				return err
			}
			wsID = ws.ID
		}
		permalink, _ := cmd.Flags().GetString("permalink")
		pt, err := db.CreatePublishTarget(args[0], engine, basePath, postsDir, permalink, wsID)
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
	Short: "List publish targets, or a target's posts with --target",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if targetName, _ := cmd.Flags().GetString("target"); targetName != "" {
			target, err := resolvePublishTarget(targetName)
			if err != nil {
				return err
			}
			pubs := db.Publications(target.Name)
			if jsonOutput {
				return jsonList(pubs, func(p fstore.Publication) publicationJSON {
					return publicationJSON{Note: p.Note.Slug, Target: target.Name, FilePath: p.Path, Draft: p.Draft}
				})
			}
			out := cmd.OutOrStdout()
			if len(pubs) == 0 {
				fmt.Fprintf(out, "No posts on %q yet\n", target.Name)
				return nil
			}
			fmt.Fprintf(out, "Posts on %q:\n\n", target.Name)
			for _, p := range pubs {
				draft := ""
				if p.Draft {
					draft = "  (draft)"
				}
				fmt.Fprintf(out, "  %s  %s%s\n", p.Path, p.Note.Title, draft)
			}
			return nil
		}

		targets := db.ListPublishTargets()
		if jsonOutput {
			return jsonList(targets, toPublishTargetJSON)
		}
		if len(targets) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No publish targets. Set one up with: kb publish setup \"name\" --engine jekyll --path ~/path/to/site")
			return nil
		}
		for _, pt := range targets {
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s\n", pt.Name, pt.Engine, filepath.Join(pt.BasePath, pt.PostsDir))
		}
		return nil
	},
}

var publishDeleteCmd = &cobra.Command{
	Use:   "delete <target>",
	Short: "Forget a publish target (its posts and the notes' history stay)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := resolvePublishTarget(args[0])
		if err != nil {
			return err
		}
		if err := db.DeletePublishTarget(target.Name); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toPublishTargetJSON(target))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted publish target %q\n", target.Name)
		return nil
	},
}

// resolvePublishTarget finds a target by name; with no name, the only one.
func resolvePublishTarget(name string) (*model.PublishTarget, error) {
	if name != "" {
		return db.GetPublishTarget(name)
	}
	targets := db.ListPublishTargets()
	switch len(targets) {
	case 0:
		return nil, fmt.Errorf("no publish targets yet; set one up with: kb publish setup")
	case 1:
		return targets[0], nil
	}
	return nil, fmt.Errorf("there are %d publish targets; choose one with --target", len(targets))
}

// freePostPath is the post path for a note's first publish, with -2, -3
// added to the slug when another note's post (one renamed since, say) or a
// file kb did not write already has the name.
func freePostPath(basePath, postsDir string, note *model.Note, date time.Time, posts map[string]fstore.Post) string {
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
	publishCmd.Flags().StringP("target", "t", "", "Publish target (default: the only one)")
	publishCmd.Flags().Bool("draft", false, "Publish as a draft (published: false)")
	publishCmd.Flags().Bool("dry-run", false, "Show the post without writing it")

	publishSetupCmd.Flags().StringP("engine", "e", "jekyll", "Site engine")
	publishSetupCmd.Flags().StringP("path", "p", "", "The site's folder (absolute, or starting with ~/)")
	publishSetupCmd.Flags().String("posts-dir", "_posts", "Posts folder within the site")
	publishSetupCmd.Flags().StringP("workspace", "w", "", "Workspace the target is for")
	publishSetupCmd.Flags().String("permalink", "", "The site's permalink pattern, for links between posts (default "+publish.DefaultPermalink+")")

	publishListCmd.Flags().StringP("target", "t", "", "List this target's posts")

	publishCmd.AddCommand(publishSetupCmd, publishListCmd, publishDeleteCmd)
	rootCmd.AddCommand(publishCmd)
}
