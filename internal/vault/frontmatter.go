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

	// What the file held, so that frontmatter kb did not change is written
	// back exactly as it was (comments, quoting, indentation).
	root    *yaml.Node // the document node: it holds a trailing comment
	raw     string     // the frontmatter text between the delimiters
	rawEnc  string     // meta as kb would encode it, when read
	indent  int        // spaces the file indents with
	crlf    bool       // the frontmatter's lines end in CRLF
	bom     bool       // the file starts with a byte-order mark
	hadMeta bool
}

func Parse(data []byte) (*Doc, error) {
	// A byte-order mark (some Windows editors write one) would hide the
	// frontmatter delimiter; it is dropped.
	text, bom := strings.CutPrefix(string(data), "\ufeff")
	rest, ok := cutDelimiter(text)
	if !ok {
		return &Doc{Body: text, bom: bom}, nil
	}

	var front string
	for {
		line, after, found := strings.Cut(rest, "\n")
		if strings.TrimRight(line, "\r") == "---" {
			doc := &Doc{Body: after, bom: bom, raw: front, crlf: strings.HasPrefix(text, "---\r\n")}
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
			return &Doc{Body: text, bom: bom}, nil
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
	d.root, d.meta, d.hadMeta = &root, root.Content[0], true
	d.indent = indentOf(front)
	d.rawEnc = d.encode()
	return nil
}

// indentOf is the indent of the first indented line of YAML that is not a
// list item ("  - a" is indented by the list, not the mapping), or 2.
func indentOf(front string) int {
	for _, line := range strings.Split(front, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if n := len(line) - len(trimmed); n > 0 && trimmed != "" && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") {
			return n
		}
	}
	return 2
}

// encode is the frontmatter as YAML text with LF line endings.
func (d *Doc) encode() string {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(max(2, d.indent))
	node := d.meta
	if d.root != nil {
		node = d.root // keeps a comment after the last key
	}
	// Encoding a node tree built by Parse or the setters cannot fail.
	_ = enc.Encode(node)
	_ = enc.Close()
	return buf.String()
}

// Render writes the note. Frontmatter whose values are as they were read
// is written back as the file had it; changed frontmatter is encoded
// again, in the file's indent, line endings and byte-order mark.
func (d *Doc) Render() []byte {
	var front string
	switch {
	case d.meta != nil && len(d.meta.Content) > 0:
		front = d.encode()
		if d.hadMeta && front == d.rawEnc {
			front = d.raw
		}
		front = "---\n" + front + "---\n"
	case d.hadMeta:
		// No keys left. A body that starts with a delimiter would be read as
		// frontmatter without them, so an empty block stays in front of it.
		if _, ok := cutDelimiter(d.Body); ok {
			front = "---\n---\n"
		}
	}
	if d.crlf {
		front = strings.ReplaceAll(strings.ReplaceAll(front, "\r\n", "\n"), "\n", "\r\n")
	}
	out := front + d.Body
	if d.bom {
		out = "\ufeff" + out
	}
	return []byte(out)
}

// Value is the frontmatter value of key as text, or "" if it is absent or
// not a plain value.
func (d *Doc) Value(key string) string { return d.str(key) }

// SetValue sets a plain text frontmatter key; an empty value removes it.
func (d *Doc) SetValue(key, value string) { d.setString(key, value) }

// Has reports whether the frontmatter has key, whatever its value.
func (d *Doc) Has(key string) bool { return d.get(key) != nil }

func (d *Doc) ID() string        { return d.str("id") }
func (d *Doc) Title() string     { return d.str("title") }
func (d *Doc) Workspace() string { return d.str("workspace") }
func (d *Doc) Tags() []string    { return d.list("tags", false) }

// Aliases are a list, or a string of names separated by commas: unlike
// tags, an alias may contain spaces ("Project Phoenix").
func (d *Doc) Aliases() []string { return d.list("aliases", true) }
func (d *Doc) Created() *time.Time {
	return d.time("created")
}

// Archived is when the note was archived, or nil. `archived: true` counts
// too; with no date it reads as the zero time.
func (d *Doc) Archived() *time.Time {
	if v := d.get("archived"); v != nil && v.Kind == yaml.ScalarNode {
		var b bool
		if v.Tag == "!!bool" && v.Decode(&b) == nil {
			if !b {
				return nil
			}
			return &time.Time{}
		}
	}
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
			old := d.meta.Content[i+1]
			value.HeadComment, value.LineComment, value.FootComment = old.HeadComment, old.LineComment, old.FootComment
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
	if v := d.get(key); v != nil && v.Kind == yaml.ScalarNode && v.Tag != "!!null" {
		return strings.TrimSpace(v.Value)
	}
	return ""
}

// list accepts a YAML sequence or a string of items separated by commas
// and, unless commasOnly, spaces, as Obsidian reads tags. A leading "#" on
// an item is dropped.
func (d *Doc) list(key string, commasOnly bool) []string {
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
			return r == ',' || (!commasOnly && (r == ' ' || r == '\t'))
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
	// A time without a zone, like Obsidian's date property, is a time on
	// the writer's own clock.
	for _, layout := range timeLayouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return &t
		}
	}
	return nil
}

// setString sets a text key. A key that already says value is left as it
// is written (quoted or not), so an unchanged save changes nothing.
func (d *Doc) setString(key, value string) {
	if value == "" {
		if d.str(key) == "" && d.get(key) != nil && d.get(key).Kind == yaml.ScalarNode {
			return // an empty key ("title:") stays as the user left it
		}
		d.set(key, nil)
		return
	}
	if d.str(key) == value {
		return
	}
	d.set(key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

// setList sets a list key; a list that already holds items, in order, is
// left as written (a block list stays a block list).
func (d *Doc) setList(key string, items []string) {
	if d.get(key) != nil && strings.Join(d.list(key, key == "aliases"), "\x00") == strings.Join(items, "\x00") {
		return
	}
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

// Publication is where a note was published on one target.
type Publication struct {
	Path  string
	Draft bool
}

// Published reads the note's publish history: the `published` key maps a
// target's name to the post's path, or to {path, draft: true} for a draft.
func (d *Doc) Published() map[string]Publication {
	v := d.get("published")
	if v == nil || v.Kind != yaml.MappingNode {
		return nil
	}
	out := map[string]Publication{}
	for i := 0; i+1 < len(v.Content); i += 2 {
		name, val := v.Content[i].Value, v.Content[i+1]
		switch val.Kind {
		case yaml.ScalarNode:
			out[name] = Publication{Path: val.Value}
		case yaml.MappingNode:
			var p struct {
				Path  string `yaml:"path"`
				Draft bool   `yaml:"draft"`
			}
			if val.Decode(&p) == nil {
				out[name] = Publication{Path: p.Path, Draft: p.Draft}
			}
		}
	}
	return out
}

// SetPublished records a post for one target in the `published` key,
// keeping the other targets' entries.
func (d *Doc) SetPublished(target string, p Publication) {
	m := d.get("published")
	if m == nil || m.Kind != yaml.MappingNode {
		m = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	value := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: p.Path}
	if p.Draft {
		value = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "path"}, {Kind: yaml.ScalarNode, Tag: "!!str", Value: p.Path},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "draft"}, {Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"},
		}}
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == target {
			m.Content[i+1] = value
			d.set("published", m)
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: target}, value)
	d.set("published", m)
}
