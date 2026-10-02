package fstore

import (
	"os"
	"path/filepath"

	"github.com/jeryldev/kb/internal/model"
	"gopkg.in/yaml.v3"
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

func (s *Store) readTargets() []targetYAML {
	var file struct {
		Targets []targetYAML `yaml:"targets"`
	}
	if data, err := os.ReadFile(s.publishFile()); err == nil {
		yaml.Unmarshal(data, &file)
	}
	return file.Targets
}

// ListPublishTargets lists this machine's publish targets.
func (s *Store) ListPublishTargets() []*model.PublishTarget {
	var out []*model.PublishTarget
	for _, t := range s.readTargets() {
		pt := &model.PublishTarget{ID: t.Name, Name: t.Name, Engine: model.Engine(t.Engine), BasePath: t.Path, PostsDir: t.PostsDir}
		if t.Workspace != "" {
			id := s.workspaceIDForName(t.Workspace)
			pt.WorkspaceID = &id
		}
		out = append(out, pt)
	}
	return out
}
