package fstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/publish"
	"github.com/jeryldev/kb/internal/vault"
)

// Publish targets hold paths on this machine (the site's folder), so they
// live in the config directory, not in the synced vault.

type targetYAML struct {
	Name      string `yaml:"name"`
	Engine    string `yaml:"engine"`
	Path      string `yaml:"path"`
	PostsDir  string `yaml:"posts_dir"`
	Permalink string `yaml:"permalink,omitempty"`
	Workspace string `yaml:"workspace,omitempty"`
}

func (s *Store) publishFile() string { return filepath.Join(s.opts.ConfigDir, "publish.yml") }

type targetsYAML struct {
	Targets []targetYAML `yaml:"targets"`
}

func (s *Store) readTargets() ([]targetYAML, error) {
	var file targetsYAML
	err := readConfigFile(s.publishFile(), &file)
	return file.Targets, err
}

// ListPublishTargets lists this machine's publish targets. A publish.yml
// kb cannot read lists none, and is among the store's Problems.
func (s *Store) ListPublishTargets() []*model.PublishTarget {
	var out []*model.PublishTarget
	targets, _ := s.readTargets()
	for _, t := range targets {
		pt := &model.PublishTarget{ID: t.Name, Name: t.Name, Engine: model.Engine(t.Engine), BasePath: t.Path, PostsDir: t.PostsDir, Permalink: t.Permalink}
		if t.Workspace != "" {
			id := s.workspaceIDForName(t.Workspace)
			pt.WorkspaceID = &id
		}
		out = append(out, pt)
	}
	return out
}

// editTargets changes publish.yml from the list in it now, under its lock.
func (s *Store) editTargets(fn func([]targetYAML) ([]targetYAML, error)) error {
	var fnErr error
	err := s.editConfigFile(s.publishFile(), func() (any, bool, error) {
		list, err := s.readTargets()
		if err != nil {
			return nil, false, err
		}
		list, fnErr = fn(list)
		if fnErr != nil {
			return nil, false, nil
		}
		return targetsYAML{Targets: list}, true, nil
	})
	if fnErr != nil {
		return fnErr
	}
	return err
}

// CreatePublishTarget adds a site to publish to. Its path must be absolute
// (or start with ~/), so posts never land somewhere that depends on the
// folder kb was run from.
func (s *Store) CreatePublishTarget(name string, engine model.Engine, sitePath, postsDir, permalink, workspaceID string) (*model.PublishTarget, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("a publish target needs a name")
	}
	if _, err := model.ParseEngine(string(engine)); err != nil {
		return nil, err
	}
	if err := publish.ValidatePermalink(permalink); err != nil {
		return nil, err
	}
	if rest, ok := strings.CutPrefix(sitePath, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		sitePath = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(sitePath) {
		return nil, fmt.Errorf("the site path must be absolute (or start with ~/), got %q", sitePath)
	}
	if postsDir == "" {
		postsDir = "_posts"
	}
	err := s.editTargets(func(list []targetYAML) ([]targetYAML, error) {
		for _, t := range list {
			if strings.EqualFold(t.Name, name) {
				return nil, fmt.Errorf("publish target %q already exists", name)
			}
		}
		return append(list, targetYAML{Name: name, Engine: string(engine), Path: filepath.Clean(sitePath), PostsDir: postsDir, Permalink: permalink, Workspace: s.workspaceNameForFile(workspaceID)}), nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetPublishTarget(name)
}

func (s *Store) GetPublishTarget(name string) (*model.PublishTarget, error) {
	for _, t := range s.ListPublishTargets() {
		if strings.EqualFold(t.Name, name) {
			return t, nil
		}
	}
	return nil, fmt.Errorf("publish target %q: %w", name, ErrNotFound)
}

// DeletePublishTarget forgets a site. The posts already written there and
// the history in notes are left alone.
func (s *Store) DeletePublishTarget(name string) error {
	return s.editTargets(func(list []targetYAML) ([]targetYAML, error) {
		var kept []targetYAML
		for _, t := range list {
			if !strings.EqualFold(t.Name, name) {
				kept = append(kept, t)
			}
		}
		if len(kept) == len(list) {
			return nil, fmt.Errorf("publish target %q: %w", name, ErrNotFound)
		}
		return kept, nil
	})
}

// Post is where a note was published on a target.
type Post = vault.Publication

// RecordPublish writes a publish into the note's frontmatter. This also
// pins the note's id into the file, so an outside rename keeps the history.
func (s *Store) RecordPublish(noteID, target, rel string, draft bool) error {
	return s.patchNote(noteID, true, func(d *vault.Doc) {
		d.SetPublished(target, vault.Publication{Path: rel, Draft: draft})
	})
}

// LatestPost is where a note was last published on a target.
func (s *Store) LatestPost(noteID, target string) (Post, bool) {
	doc := s.docs[noteID]
	if doc == nil {
		return Post{}, false
	}
	p, ok := doc.Published()[target]
	return p, ok
}

// PublishedPosts maps each note published to a target to its post.
func (s *Store) PublishedPosts(target string) map[string]Post {
	out := map[string]Post{}
	for id, doc := range s.docs {
		if p, ok := doc.Published()[target]; ok {
			out[id] = p
		}
	}
	return out
}

// Publication is one note's post on a target.
type Publication struct {
	Note  *Note
	Path  string
	Draft bool
}

// Publications lists a target's posts, by path (so by date).
func (s *Store) Publications(target string) []Publication {
	var out []Publication
	for id, p := range s.PublishedPosts(target) {
		out = append(out, Publication{Note: s.byID[id], Path: p.Path, Draft: p.Draft})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
