package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/publish"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
		// kb has only the name of a note iCloud has not downloaded; its
		// empty body must not become the post.
		if db.NotDownloaded(note.ID) {
			return fmt.Errorf("%s %w", note.Path, fstore.ErrNotDownloaded)
		}
		targetName, _ := cmd.Flags().GetString("target")
		target, err := resolvePublishTarget(targetName, note)
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
			if relPath, err = freePostPath(target.BasePath, target.PostsDir, note, date, posts); err != nil {
				return err
			}
		}

		// Link only to posts that are out: a draft's URL does not exist yet.
		// A site served below its domain (GitHub project pages) says so in
		// its _config.yml baseurl, which every link starts with.
		baseurl := siteBaseurl(target.BasePath)
		permalinks := make(map[string]string, len(posts))
		for noteID, post := range posts {
			if post.Draft {
				continue
			}
			if permalink, ok := publish.PermalinkFor(target.Permalink, post.Path); ok {
				permalinks[noteID] = baseurl + permalink
			}
		}

		content := publish.GeneratePost(note, date, draft, permalinks, db)
		fullPath := filepath.Join(target.BasePath, relPath)

		if dryRun {
			if jsonOutput {
				return printJSON(struct {
					publicationJSON
					Content string `json:"content"`
				}{publicationJSON{Note: note.Slug, Target: target.Name, FilePath: relPath, FullPath: fullPath, Draft: draft}, content})
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Would write to: %s\n\n", fullPath)
			fmt.Fprint(out, content)
			return nil
		}

		// The note records the post first: a post written but not
		// recorded would look like someone else's file to the next
		// publish, which would add a second post beside it. A record whose
		// post failed to write is mended by publishing again.
		if err := db.RecordPublish(note.ID, target.Name, relPath, draft); err != nil {
			return fmt.Errorf("could not record the post in the note, so nothing was published: %w", err)
		}
		if err := writeFileAtomic(fullPath, []byte(content)); err != nil {
			return fmt.Errorf("writing %s: %w; publish again to write it", fullPath, err)
		}

		if jsonOutput {
			return printJSON(publicationJSON{Note: note.Slug, Target: target.Name, FilePath: relPath, FullPath: fullPath, Draft: draft})
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
			target, err := resolvePublishTarget(targetName, nil)
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
		target, err := resolvePublishTarget(args[0], nil)
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

// resolvePublishTarget finds a target by name; with no name, the only
// one, or the only one set up for the note's workspace.
func resolvePublishTarget(name string, note *model.Note) (*model.PublishTarget, error) {
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
	var mine []*model.PublishTarget
	for _, t := range targets {
		if note != nil && t.WorkspaceID != nil && *t.WorkspaceID == note.WorkspaceID {
			mine = append(mine, t)
		}
	}
	if len(mine) == 1 {
		return mine[0], nil
	}
	return nil, fmt.Errorf("there are %d publish targets; choose one with --target", len(targets))
}

// siteBaseurl is the baseurl in the site's _config.yml, or "".
func siteBaseurl(site string) string {
	data, err := os.ReadFile(filepath.Join(site, "_config.yml"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Baseurl string `yaml:"baseurl"`
	}
	if yaml.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return strings.TrimRight(cfg.Baseurl, "/")
}

// writeFileAtomic writes a file whole or not at all, through a temporary
// file beside it, so a site rebuilding on changes never reads half a post.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".kb-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// freePostPath is the post path for a note's first publish, with -2, -3
// added to the slug when another note's post (one renamed since, say) or a
// file kb did not write already has the name.
func freePostPath(basePath, postsDir string, note *model.Note, date time.Time, posts map[string]fstore.Post) (string, error) {
	taken := map[string]bool{}
	for noteID, post := range posts {
		if noteID != note.ID {
			taken[post.Path] = true
		}
	}
	slug := note.Slug
	for n := 2; ; n++ {
		relPath := publish.PostFilePath(postsDir, slug, date)
		_, err := os.Stat(filepath.Join(basePath, relPath))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			// Not a name in use but a posts folder kb cannot use.
			return "", fmt.Errorf("checking %s: %w", filepath.Join(basePath, relPath), err)
		}
		if !taken[relPath] && err != nil {
			return relPath, nil
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
	publishSetupCmd.Flags().String("permalink", "", "The site's permalink pattern, for links between posts, from :year, :month, :day and :title (default "+publish.DefaultPermalink+")")

	publishListCmd.Flags().StringP("target", "t", "", "List this target's posts")

	publishCmd.AddCommand(publishSetupCmd, publishListCmd, publishDeleteCmd)
	rootCmd.AddCommand(publishCmd)
}
