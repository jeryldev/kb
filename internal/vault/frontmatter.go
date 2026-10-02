// Package vault reads and writes notes as Markdown files with YAML
// frontmatter. The files are the source of truth; the store only indexes
// them.
package vault

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Doc is a note file: optional frontmatter plus the Markdown body. The
// frontmatter is kept as a YAML node tree so that keys kb does not know
// (an Obsidian cssclass, a publish date) survive an edit, in their order.
type Doc struct {
	meta *yaml.Node
	Body string
}

func Parse(data []byte) (*Doc, error) {
	text := string(data)
	rest, ok := cutDelimiter(text)
	if !ok {
		return &Doc{Body: text}, nil
	}

	var front string
	for {
		line, after, found := strings.Cut(rest, "\n")
		if strings.TrimRight(line, "\r") == "---" {
			doc := &Doc{Body: after}
			if !found {
				doc.Body = ""
			}
			if err := doc.decode(front); err != nil {
				return nil, err
			}
			return doc, nil
		}
		if !found {
			// No closing delimiter: the leading "---" was a thematic break.
			return &Doc{Body: text}, nil
		}
		front += line + "\n"
		rest = after
	}
}

func cutDelimiter(text string) (string, bool) {
	if rest, ok := strings.CutPrefix(text, "---\n"); ok {
		return rest, true
	}
	return strings.CutPrefix(text, "---\r\n")
}

func (d *Doc) decode(front string) error {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(front), &root); err != nil {
		return fmt.Errorf("parsing frontmatter: %w", err)
	}
	if len(root.Content) == 0 {
		return nil
	}
	if root.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("frontmatter is not a key/value mapping")
	}
	d.meta = root.Content[0]
	return nil
}

func (d *Doc) Render() []byte {
	if d.meta == nil || len(d.meta.Content) == 0 {
		return []byte(d.Body)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	// Encoding a node tree built by Parse or the setters cannot fail.
	_ = enc.Encode(d.meta)
	_ = enc.Close()
	buf.WriteString("---\n")
	buf.WriteString(d.Body)
	return buf.Bytes()
}

// Value is the frontmatter value of key as text, or "" if it is absent or
// not a plain value.
func (d *Doc) Value(key string) string { return d.str(key) }

// Has reports whether the frontmatter has key, whatever its value.
func (d *Doc) Has(key string) bool { return d.get(key) != nil }

func (d *Doc) ID() string        { return d.str("id") }
func (d *Doc) Title() string     { return d.str("title") }
func (d *Doc) Workspace() string { return d.str("workspace") }
func (d *Doc) Tags() []string    { return d.list("tags") }
func (d *Doc) Aliases() []string { return d.list("aliases") }
func (d *Doc) Created() *time.Time {
	return d.time("created")
}
func (d *Doc) Archived() *time.Time {
	return d.time("archived")
}

func (d *Doc) Pinned() bool {
	var b bool
	if v := d.get("pinned"); v != nil {
		_ = v.Decode(&b)
	}
	return b
}

func (d *Doc) SetID(id string)          { d.setString("id", id) }
func (d *Doc) SetTitle(title string)    { d.setString("title", title) }
func (d *Doc) SetWorkspace(name string) { d.setString("workspace", name) }
func (d *Doc) SetTags(tags []string)    { d.setList("tags", tags) }
func (d *Doc) SetAliases(a []string)    { d.setList("aliases", a) }
func (d *Doc) SetCreated(t *time.Time)  { d.setTime("created", t) }
func (d *Doc) SetArchived(t *time.Time) { d.setTime("archived", t) }

func (d *Doc) SetPinned(pinned bool) {
	if !pinned {
		d.set("pinned", nil)
		return
	}
	d.set("pinned", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
}

func (d *Doc) get(key string) *yaml.Node {
	if d.meta == nil {
		return nil
	}
	for i := 0; i+1 < len(d.meta.Content); i += 2 {
		if d.meta.Content[i].Value == key {
			return d.meta.Content[i+1]
		}
	}
	return nil
}

// set replaces key's value in place, appends a new key, or with a nil value
// removes the key.
func (d *Doc) set(key string, value *yaml.Node) {
	if d.meta == nil {
		if value == nil {
			return
		}
		d.meta = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	for i := 0; i+1 < len(d.meta.Content); i += 2 {
		if d.meta.Content[i].Value != key {
			continue
		}
		if value == nil {
			d.meta.Content = append(d.meta.Content[:i], d.meta.Content[i+2:]...)
		} else {
			d.meta.Content[i+1] = value
		}
		return
	}
	if value != nil {
		d.meta.Content = append(d.meta.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
	}
}

func (d *Doc) str(key string) string {
	if v := d.get(key); v != nil && v.Kind == yaml.ScalarNode {
		return strings.TrimSpace(v.Value)
	}
	return ""
}

// list accepts a YAML sequence or, as Obsidian does, a string of items
// separated by commas or spaces. A leading "#" on a tag is dropped.
func (d *Doc) list(key string) []string {
	v := d.get(key)
	if v == nil {
		return nil
	}
	var raw []string
	switch v.Kind {
	case yaml.SequenceNode:
		for _, item := range v.Content {
			if item.Kind == yaml.ScalarNode {
				raw = append(raw, item.Value)
			}
		}
	case yaml.ScalarNode:
		raw = strings.FieldsFunc(v.Value, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t'
		})
	}
	var out []string
	for _, s := range raw {
		s = strings.TrimPrefix(strings.TrimSpace(s), "#")
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

var timeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

func (d *Doc) time(key string) *time.Time {
	s := d.str(key)
	if s == "" {
		return nil
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

func (d *Doc) setString(key, value string) {
	if value == "" {
		d.set(key, nil)
		return
	}
	d.set(key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func (d *Doc) setList(key string, items []string) {
	if len(items) == 0 {
		d.set(key, nil)
		return
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, item := range items {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: item})
	}
	d.set(key, seq)
}

func (d *Doc) setTime(key string, t *time.Time) {
	if t == nil {
		d.set(key, nil)
		return
	}
	d.set(key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: t.UTC().Format(time.RFC3339)})
}
