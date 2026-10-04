// Package board reads and writes kanban boards stored as Markdown in the
// Obsidian Kanban plugin's format (github.com/mgmeyers/obsidian-kanban,
// src/parsers/formats/list.ts): a "kanban-plugin" frontmatter key, one
// heading per lane with an optional "(N)" item limit, task-list items, an
// optional **Complete** marker, an archive after a thematic break, and the
// plugin's settings block at the end.
//
// Parsing is lossless: Render writes back every line kb did not change
// exactly as it was read, so plugin features kb does not model (dates,
// settings, other inline fields, odd checkboxes) survive kb's edits.
package board

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jeryldev/kb/internal/vault"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// FrontmatterKey marks a Markdown file as a board, as the plugin does.
const FrontmatterKey = "kanban-plugin"

type Board struct {
	Front *vault.Doc
	Lanes []*Lane
	// Archive holds archived cards, or is nil when the board has none.
	Archive *Lane

	frontLines []string
	frontDirty bool
	pre        []string // lines between the frontmatter and the first lane
	trailer    []string // the settings block and anything after it
	newArchive bool     // Archive was created by kb and needs its heading
	// archiveSep is the thematic break before the Archive heading, with
	// the lines between them. It belongs to the archive, not to the lane
	// read before it, so adding or reordering lanes keeps it in place.
	archiveSep []string
	// archiveAt is how many lanes come before the archive. Lanes may
	// follow it; archiveLast keeps it last as lanes are added.
	archiveAt   int
	archiveLast bool
	salt        string
	indent      string // continuation indent the file uses: a tab or 4 spaces
	crlf        bool   // the file's lines end in CRLF
	bom         bool   // the file starts with a byte-order mark
	changed     bool   // a card was added, moved, archived or deleted
}

type Lane struct {
	Title    string
	MaxItems int
	// Complete lanes carry the plugin's **Complete** marker: cards in them
	// are checked.
	Complete bool

	heading      string
	underline    string // a setext heading's "===" line, or ""
	origTitle    string
	origMax      int
	completeLine string // the file's own **Complete** marker, in its language
	elems        []elem
}

// elem is a card or a line kb keeps as it is (blank lines, markers, text).
type elem struct {
	item *Item
	raw  string
}

var (
	headingRe   = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*$`)
	maxItemsRe  = regexp.MustCompile(`^(.*?)\s*\((\d+)\)$`)
	settingsMk  = "%% kanban:settings"
	archiveName = "Archive"
)

// IsBoard reports whether a Markdown file is a board.
func IsBoard(data []byte) bool {
	doc, err := vault.Parse(data)
	return err == nil && doc.Has(FrontmatterKey)
}

// Parse reads a board. Cards with no block id get ids derived from their
// lane and title, stable across reads; ParseSalted mixes in a salt (the
// board's path) so such ids also differ between boards.
func Parse(data []byte) (*Board, error) {
	return ParseSalted(data, "")
}

func ParseSalted(data []byte, salt string) (*Board, error) {
	doc, err := vault.Parse(data)
	if err != nil {
		return nil, err
	}
	text, bom := strings.CutPrefix(string(data), "\ufeff")
	lines := strings.Split(text, "\n")
	b := &Board{Front: doc, salt: salt, bom: bom}
	// A CRLF board is read without its CRs, which CommonMark and the plugin
	// ignore, and written back with them.
	if strings.HasSuffix(lines[0], "\r") {
		b.crlf = true
		for i, l := range lines {
			lines[i] = strings.TrimSuffix(l, "\r")
		}
	}

	body := lines
	// The delimiters may still end in CR when the endings are mixed.
	if len(lines) > 0 && strings.TrimRight(lines[0], "\r") == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimRight(lines[i], "\r") == "---" {
				b.frontLines, body = lines[:i+1], lines[i+1:]
				break
			}
		}
	}

	st := scanStructure(body)
	end := len(body)
	if st.trailer >= 0 {
		end = st.trailer
		b.trailer = body[st.trailer:]
	}
	firstLane := end
	if len(st.lanes) > 0 {
		firstLane = st.lanes[0].line
	}
	b.pre = body[:firstLane]

	for i, ls := range st.lanes {
		laneEnd := end
		if i+1 < len(st.lanes) {
			laneEnd = st.lanes[i+1].line
		}
		lane := &Lane{heading: body[ls.line], Complete: ls.complete, completeLine: ls.completeLine}
		start := ls.line + 1
		if ls.setext {
			lane.underline = body[start]
			start++
		}
		lane.Title, lane.MaxItems = parseLaneTitle(ls.title)
		lane.origTitle, lane.origMax = lane.Title, lane.MaxItems
		spans := ls.items
		for l := start; l < laneEnd; {
			if len(spans) > 0 && spans[0].start == l {
				lane.elems = append(lane.elems, elem{item: &Item{raw: body[spans[0].start : spans[0].end+1]}})
				l = spans[0].end + 1
				spans = spans[1:]
				continue
			}
			lane.elems = append(lane.elems, elem{raw: body[l]})
			l++
		}
		if ls.archive {
			b.Archive = lane
			b.archiveAt = len(b.Lanes)
		} else {
			b.Lanes = append(b.Lanes, lane)
		}
	}

	b.archiveLast = b.archiveAt == len(b.Lanes)
	b.takeArchiveSep()
	b.decodeItems()
	b.indent = "    "
	for _, l := range b.allLanes() {
		for _, it := range l.Items() {
			if len(it.raw) > 1 && strings.HasPrefix(it.raw[1], "\t") {
				b.indent = "\t"
			}
		}
	}
	return b, nil
}

var thematicBreak = regexp.MustCompile(`^ {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$`)

// takeArchiveSep moves the thematic break before the Archive heading, and
// what follows it, out of the last lane's lines.
func (b *Board) takeArchiveSep() {
	if b.Archive == nil || b.archiveAt == 0 {
		return
	}
	last := b.Lanes[b.archiveAt-1]
	for i := len(last.elems) - 1; i >= 0; i-- {
		e := last.elems[i]
		if e.item != nil {
			return
		}
		if thematicBreak.MatchString(strings.TrimRight(e.raw, "\r")) {
			for _, rest := range last.elems[i:] {
				b.archiveSep = append(b.archiveSep, rest.raw)
			}
			last.elems = last.elems[:i]
			return
		}
	}
}

type laneSpan struct {
	line         int
	setext       bool // the heading is a line of text over === or ---
	title        string
	archive      bool
	complete     bool
	completeLine string
	items        []itemSpan
}

type itemSpan struct{ start, end int }

type structure struct {
	lanes   []laneSpan
	trailer int // first line of the settings block, or -1
}

// scanStructure finds the board's lanes, cards and settings block with a
// CommonMark parser, as the plugin does with its own (mdast), so a card's
// lines are exactly the lines CommonMark gives its list item, lazy
// continuation lines and indented paragraphs after a blank line included.
// A lane's cards are the items of the first list after its heading; the
// plugin ignores anything else between headings.
func scanStructure(body []string) structure {
	src := []byte(strings.Join(body, "\n"))
	starts := make([]int, len(body))
	off := 0
	for i, l := range body {
		starts[i] = off
		off += len(l) + 1
	}
	lineOf := func(pos int) int {
		return sort.Search(len(starts), func(i int) bool { return starts[i] > pos }) - 1
	}

	st := structure{trailer: -1}
	root := goldmark.DefaultParser().Parse(text.NewReader(src))
	var lane *laneSpan
	listSeen := false
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		switch n.Kind() {
		case ast.KindHeading:
			if n.Lines().Len() == 0 {
				continue
			}
			line := lineOf(n.Lines().At(0).Start)
			var title string
			setext := false
			if m := headingRe.FindStringSubmatch(body[line]); m != nil {
				title = headingText(m[1])
			} else if n.Lines().Len() == 1 && line+1 < len(body) && setextUnderline.MatchString(body[line+1]) {
				title, setext = strings.TrimSpace(body[line]), true
			} else {
				continue
			}
			prev := n.PreviousSibling()
			st.lanes = append(st.lanes, laneSpan{
				line:    line,
				setext:  setext,
				title:   title,
				archive: isArchiveWord(title) && prev != nil && prev.Kind() == ast.KindThematicBreak,
			})
			lane = &st.lanes[len(st.lanes)-1]
			listSeen = false
		case ast.KindParagraph:
			if n.Lines().Len() == 0 {
				continue
			}
			line := lineOf(n.Lines().At(0).Start)
			if strings.HasPrefix(body[line], settingsMk) {
				st.trailer = line
				return st
			}
			if lane != nil && !listSeen && n.Lines().Len() == 1 && isCompleteMarker(body[line]) {
				lane.complete = true
				lane.completeLine = body[line]
			}
		case ast.KindList:
			if lane == nil || listSeen {
				continue
			}
			listSeen = true
			for item := n.FirstChild(); item != nil; item = item.NextSibling() {
				first, last := nodeLines(item, lineOf)
				if first < 0 {
					continue
				}
				// The marker is on the item's first content line, except
				// when the content starts on the line after it.
				for first > lane.line+1 && !itemRe.MatchString(body[first]) {
					first--
				}
				lane.items = append(lane.items, itemSpan{first, closingFence(body, item, last)})
			}
		}
	}
	return st
}

// nodeLines is the first and last source line of any text under n.
func nodeLines(n ast.Node, lineOf func(int) int) (int, int) {
	first, last := -1, -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || c.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		lines := c.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			s, e := lineOf(seg.Start), lineOf(max(seg.Start, seg.Stop-1))
			if first < 0 || s < first {
				first = s
			}
			if e > last {
				last = e
			}
		}
		return ast.WalkContinue, nil
	})
	return first, last
}

// closingFence extends an item ending in a fenced code block to the fence
// that closes it, which goldmark does not count as a line of the block.
func closingFence(body []string, item ast.Node, last int) int {
	n := item.LastChild()
	for n != nil && n.Kind() != ast.KindFencedCodeBlock && n.LastChild() != nil {
		n = n.LastChild()
	}
	if n != nil && n.Kind() == ast.KindFencedCodeBlock && last+1 < len(body) {
		trimmed := strings.TrimSpace(body[last+1])
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			return last + 1
		}
	}
	return last
}

// headingText drops an ATX heading's closing #s, which CommonMark counts
// only after a space: "## C#" is the lane "C#", "## Done ##" is "Done".
func headingText(s string) string {
	if strings.Trim(s, "#") == "" {
		return ""
	}
	return strings.TrimSpace(closingHashes.ReplaceAllString(s, ""))
}

var (
	closingHashes   = regexp.MustCompile(`[ \t]+#+[ \t]*$`)
	setextUnderline = regexp.MustCompile(`^ {0,3}(?:=+|-+)[ \t]*$`)
)

func isArchiveWord(title string) bool {
	for _, m := range localeMarkers {
		if title == m.Archive {
			return true
		}
	}
	return false
}

func isCompleteMarker(line string) bool {
	for _, m := range localeMarkers {
		if line == "**"+m.Complete+"**" {
			return true
		}
	}
	return false
}

func parseLaneTitle(s string) (string, int) {
	if m := maxItemsRe.FindStringSubmatch(s); m != nil {
		var n int
		fmt.Sscanf(m[2], "%d", &n)
		return m[1], n
	}
	return s, 0
}

// decodeItems fills each item's fields from its raw lines and gives every
// card a unique id.
func (b *Board) decodeItems() {
	seen := map[string]bool{}
	occurrences := map[string]int{}
	keys := map[*Item]string{}
	for _, lane := range b.allLanes() {
		for _, it := range lane.Items() {
			it.decode()
			if it.ID == "" || seen[it.ID] {
				key := lane.Title + "\x00" + it.Title
				occurrences[key]++
				it.ID = b.derivedID(key, occurrences[key], seen)
				it.derived = true
				keys[it] = key
			}
			seen[it.ID] = true
		}
	}
	for it, key := range keys {
		it.twin = occurrences[key] > 1
	}
}

// pinTwins writes out the derived ids of cards that share a lane and a
// title: their ids come from their order, so removing or moving one would
// hand its id to another, and an id a script holds would name the wrong
// card.
func (b *Board) pinTwins() {
	for _, l := range b.allLanes() {
		for _, it := range l.Items() {
			if it.derived && it.twin {
				it.firstDirty = true
			}
		}
	}
}

func (b *Board) derivedID(key string, n int, seen map[string]bool) string {
	for attempt := 0; ; attempt++ {
		sum := sha1.Sum([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d", b.salt, key, n, attempt)))
		id := hex.EncodeToString(sum[:4])
		if !seen[id] {
			return id
		}
	}
}

// dedent removes one level of indent from a card's continuation line as
// the plugin does (dedentNewLines): a tab or exactly four spaces, so a line
// indented less keeps its spaces.
func dedent(line string) string {
	if strings.HasPrefix(line, "\t") {
		return line[1:]
	}
	return strings.TrimPrefix(line, "    ")
}

func (b *Board) allLanes() []*Lane {
	if b.Archive != nil {
		return append(append([]*Lane{}, b.Lanes...), b.Archive)
	}
	return b.Lanes
}

// Items lists the lane's cards in order.
func (l *Lane) Items() []*Item {
	var out []*Item
	for _, e := range l.elems {
		if e.item != nil {
			out = append(out, e.item)
		}
	}
	return out
}

// New is an empty board with the given lanes, as the plugin creates one.
func New(lanes []string) *Board {
	doc, _ := vault.Parse([]byte("---\n" + FrontmatterKey + ": board\n---\n"))
	b := &Board{
		Front:      doc,
		frontLines: []string{"---", FrontmatterKey + ": board", "---"},
		pre:        []string{""},
		trailer:    []string{settingsMk, "```", `{"kanban-plugin":"board"}`, "```", "%%"},
		indent:     "    ",
	}
	for _, name := range lanes {
		b.Lanes = append(b.Lanes, &Lane{
			Title: name, origTitle: name, heading: "## " + name,
			elems: []elem{{raw: ""}, {raw: ""}, {raw: ""}},
		})
	}
	return b
}

// MarkFrontmatterChanged makes Render write the frontmatter from Front
// instead of the lines that were read.
func (b *Board) MarkFrontmatterChanged() { b.frontDirty = true }

// Lane finds a lane by title, ignoring case.
func (b *Board) Lane(title string) *Lane {
	for _, l := range b.Lanes {
		if strings.EqualFold(l.Title, title) {
			return l
		}
	}
	return nil
}

// Find returns the card with the id and the lane holding it.
func (b *Board) Find(id string) (*Item, *Lane) {
	for _, l := range b.allLanes() {
		for _, it := range l.Items() {
			if it.ID == id {
				return it, l
			}
		}
	}
	return nil, nil
}

// Add appends a new card to the end of a lane.
func (b *Board) Add(laneTitle, title, description string) (*Item, error) {
	lane := b.Lane(laneTitle)
	if lane == nil {
		return nil, fmt.Errorf("no column %q", laneTitle)
	}
	if err := ValidateCardTitle(title); err != nil {
		return nil, err
	}
	it := &Item{ID: b.newID(), Check: ' ', bullet: lane.marker(), raw: []string{""}}
	if lane.Complete {
		it.Check = 'x'
	}
	it.SetTitle(title)
	if description != "" {
		it.SetDescription(description)
	}
	lane.insert(it, len(lane.Items()))
	b.changed = true
	return it, nil
}

// Move puts a card at position index (0 = top) of another lane, or the same
// one. Cards moved into a **Complete** lane are checked, and out of one,
// unchecked, as the plugin does.
func (b *Board) Move(id, laneTitle string, index int) error {
	it, from := b.Find(id)
	if it == nil {
		return fmt.Errorf("no card %q", id)
	}
	to := b.Lane(laneTitle)
	if to == nil {
		return fmt.Errorf("no column %q", laneTitle)
	}
	from.remove(it)
	switch {
	case to.Complete:
		it.Check = 'x'
	case from.Complete:
		it.Check = ' '
	}
	if from != to {
		it.lead, it.bullet = "", to.marker()
	}
	it.firstDirty = true
	to.insert(it, index)
	b.changed = true
	return nil
}

// ArchiveItem moves a card to the board's archive, creating it if needed.
func (b *Board) ArchiveItem(id string) error {
	it, from := b.Find(id)
	if it == nil {
		return fmt.Errorf("no card %q", id)
	}
	if from == b.Archive {
		return fmt.Errorf("card %q is already archived", id)
	}
	if b.Archive == nil {
		name := b.archiveWord()
		b.Archive = &Lane{Title: name, origTitle: name, heading: "## " + name, elems: []elem{{raw: ""}}}
		b.newArchive = true
		b.archiveAt, b.archiveLast = len(b.Lanes), true
	}
	from.remove(it)
	it.lead, it.bullet = "", b.Archive.marker()
	it.firstDirty = true
	b.Archive.insert(it, len(b.Archive.Items()))
	b.changed = true
	return nil
}

// Delete removes a card from the board.
func (b *Board) Delete(id string) error {
	it, from := b.Find(id)
	if it == nil {
		return fmt.Errorf("no card %q", id)
	}
	from.remove(it)
	b.changed = true
	return nil
}

// archiveWord is the Archive heading in the board's language, read from
// its **Complete** marker when it has one.
func (b *Board) archiveWord() string {
	for _, l := range b.Lanes {
		for _, m := range localeMarkers {
			if l.completeLine != "" && l.completeLine == "**"+m.Complete+"**" {
				return m.Archive
			}
		}
	}
	return archiveName
}

// marker is the list marker the lane's cards use, for a card added or
// moved there to match: the plugin reads only a lane's first list, and a
// different marker (* after -, or 1) after 1.) would start a second one,
// hiding the card. Such a card starts at the margin (no lead): indented
// as far as the text of the card above it, it would become part of it.
func (l *Lane) marker() string {
	for _, it := range l.Items() {
		if it.bullet != "" {
			return it.bullet
		}
	}
	return "-"
}

// ValidateLaneName refuses names the plugin would misread: a name ending in
// a number in parentheses is read as the lane's item limit.
func ValidateLaneName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return fmt.Errorf("column name cannot be empty")
	case strings.ContainsAny(name, "\r\n"):
		return fmt.Errorf("column name cannot span lines")
	case maxItemsRe.MatchString(name):
		return fmt.Errorf("column name %q ends in a number in parentheses, which the Kanban plugin reads as the column's card limit", name)
	case headingText(name) != name:
		return fmt.Errorf("column name %q ends in #s, which a heading reads as closing marks", name)
	}
	return nil
}

// ValidateCardTitle refuses titles a card line cannot hold.
func ValidateCardTitle(title string) error {
	switch {
	case strings.TrimSpace(title) == "":
		return fmt.Errorf("card title cannot be empty")
	case strings.ContainsAny(title, "\r\n"):
		return fmt.Errorf("card title cannot span lines; put the rest in the description")
	}
	return nil
}

func (b *Board) dirty() bool {
	if b.changed || b.newArchive {
		return true
	}
	for _, l := range b.allLanes() {
		for _, it := range l.Items() {
			if it.dirty() {
				return true
			}
		}
	}
	return false
}

func (b *Board) newID() string {
	for {
		buf := make([]byte, 4)
		rand.Read(buf)
		id := hex.EncodeToString(buf)
		if it, _ := b.Find(id); it == nil {
			return id
		}
	}
}

func (l *Lane) remove(it *Item) {
	for i, e := range l.elems {
		if e.item == it {
			l.elems = append(l.elems[:i], l.elems[i+1:]...)
			return
		}
	}
}

// insert places a card before the index-th card, or after the last one.
// Into a lane with no cards it goes after the blank line and the Complete
// marker that follow the heading, keeping the blank lines before the next
// lane.
func (l *Lane) insert(it *Item, index int) {
	pos := -1
	n := 0
	for i, e := range l.elems {
		if e.item == nil {
			continue
		}
		if n == index {
			pos = i
			break
		}
		n++
		pos = i + 1
	}
	if pos < 0 {
		pos = 0
		if pos < len(l.elems) && l.elems[pos].raw == "" && l.elems[pos].item == nil {
			pos++
		}
		if pos < len(l.elems) && l.elems[pos].item == nil && isCompleteMarker(l.elems[pos].raw) {
			pos++
		}
		// Text right after the card would read as part of it (a lazy
		// continuation line): keep a blank line between them.
		if pos < len(l.elems) && l.elems[pos].item == nil && strings.TrimSpace(l.elems[pos].raw) != "" {
			l.elems = append(l.elems[:pos], append([]elem{{raw: ""}}, l.elems[pos:]...)...)
		}
	}
	l.elems = append(l.elems, elem{})
	copy(l.elems[pos+1:], l.elems[pos:])
	l.elems[pos] = elem{item: it}
}

func (l *Lane) render(indent string) []string {
	out := []string{l.heading}
	if l.underline != "" {
		out = append(out, l.underline)
	}
	if l.Title != l.origTitle || l.MaxItems != l.origMax {
		heading := "## " + l.Title
		if l.MaxItems > 0 {
			heading += fmt.Sprintf(" (%d)", l.MaxItems)
		}
		out = []string{heading}
	}
	for _, e := range l.elems {
		if e.item != nil {
			out = append(out, e.item.render(indent)...)
		} else {
			out = append(out, e.raw)
		}
	}
	return out
}

// Render writes the board back as Markdown.
func (b *Board) Render() []byte {
	var out []string
	if b.frontDirty || b.frontLines == nil {
		front := *b.Front
		front.Body = ""
		out = append(out, strings.Split(strings.TrimSuffix(string(front.Render()), "\n"), "\n")...)
	} else {
		out = append(out, b.frontLines...)
	}
	out = append(out, b.pre...)
	// A board kb changed writes its twin cards' ids (see pinTwins).
	if b.changed {
		b.pinTwins()
	}
	at := len(b.Lanes)
	if b.Archive != nil && !b.archiveLast {
		at = min(b.archiveAt, len(b.Lanes))
	}
	for _, l := range b.Lanes[:at] {
		out = append(out, l.render(b.indent)...)
	}
	if b.Archive != nil {
		out = append(out, b.archiveSep...)
		if b.newArchive {
			if n := len(out); n > 0 && out[n-1] != "" {
				out = append(out, "")
			}
			out = append(out, "***", "")
		}
		out = append(out, b.Archive.render(b.indent)...)
	}
	for _, l := range b.Lanes[at:] {
		out = append(out, l.render(b.indent)...)
	}
	// The plugin finds its settings block only at the very end, and reads a
	// line right after a card as part of that card: keep a blank line
	// before the block once kb has changed the board.
	if len(b.trailer) > 0 && b.dirty() {
		if n := len(out); n > 0 && out[n-1] != "" {
			out = append(out, "")
		}
	}
	out = append(out, b.trailer...)
	sep := "\n"
	if b.crlf {
		sep = "\r\n"
	}
	text := strings.Join(out, sep)
	if b.bom {
		text = "\ufeff" + text
	}
	return []byte(text)
}

// MoveBefore puts a card in a lane just above another card, or at the end
// of the lane when beforeID is empty. It is the one move operation: it
// reads the same whatever else changed in the file since, so it can be
// applied to a board re-read under a lock.
func (b *Board) MoveBefore(id, laneTitle, beforeID string) error {
	to := b.Lane(laneTitle)
	if to == nil {
		return fmt.Errorf("no column %q", laneTitle)
	}
	index := len(to.Items())
	if beforeID != "" {
		index = -1
		for i, it := range to.Items() {
			if it.ID == beforeID {
				index = i
			}
		}
		if index < 0 {
			return fmt.Errorf("no card %q in column %q", beforeID, laneTitle)
		}
	}
	it, from := b.Find(id)
	if it == nil {
		return fmt.Errorf("no card %q", id)
	}
	if from == to {
		for i, other := range to.Items() {
			if other == it && i < index {
				index-- // removing the card first shifts the target up
			}
		}
	}
	return b.Move(id, laneTitle, index)
}

// AddLane appends an empty lane after the others (before the archive).
func (b *Board) AddLane(name string) error {
	name = strings.TrimSpace(name)
	if err := ValidateLaneName(name); err != nil {
		return err
	}
	if b.Lane(name) != nil {
		return fmt.Errorf("column %q already exists", name)
	}
	if n := len(b.Lanes); n > 0 {
		last := b.Lanes[n-1]
		if k := len(last.elems); k == 0 || last.elems[k-1].item != nil || last.elems[k-1].raw != "" {
			last.elems = append(last.elems, elem{raw: ""}, elem{raw: ""})
		}
	}
	b.Lanes = append(b.Lanes, &Lane{
		Title: name, origTitle: name, heading: "## " + name,
		elems: []elem{{raw: ""}, {raw: ""}, {raw: ""}},
	})
	b.changed = true
	return nil
}

// RemoveLane deletes a lane that has no cards.
func (b *Board) RemoveLane(name string) error {
	for i, l := range b.Lanes {
		if strings.EqualFold(l.Title, name) {
			if len(l.Items()) > 0 {
				return fmt.Errorf("column %q still has %d cards", l.Title, len(l.Items()))
			}
			for _, e := range l.elems {
				if strings.TrimSpace(e.raw) != "" && !isCompleteMarker(e.raw) {
					return fmt.Errorf("column %q holds text besides cards (%q); move or delete it in the file first", l.Title, strings.TrimSpace(e.raw))
				}
			}
			b.Lanes = append(b.Lanes[:i], b.Lanes[i+1:]...)
			if i < b.archiveAt {
				b.archiveAt--
			}
			b.changed = true
			return nil
		}
	}
	return fmt.Errorf("no column %q", name)
}

// RenameLane changes a lane's title, keeping its card limit.
func (b *Board) RenameLane(from, to string) error {
	l := b.Lane(from)
	if l == nil {
		return fmt.Errorf("no column %q", from)
	}
	to = strings.TrimSpace(to)
	if err := ValidateLaneName(to); err != nil {
		return err
	}
	if other := b.Lane(to); other != nil && other != l {
		return fmt.Errorf("column %q already exists", to)
	}
	l.Title = to
	// Ids kb derived from the old title would change: write them out.
	for _, it := range l.Items() {
		if it.derived {
			it.firstDirty = true
		}
	}
	b.changed = true
	return nil
}

// ReorderLanes puts the lanes in the order of names, which must name every
// lane exactly once.
func (b *Board) ReorderLanes(names []string) error {
	if len(names) != len(b.Lanes) {
		return fmt.Errorf("name all %d columns to reorder them, got %d", len(b.Lanes), len(names))
	}
	var out []*Lane
	seen := map[*Lane]bool{}
	for _, n := range names {
		l := b.Lane(n)
		if l == nil {
			return fmt.Errorf("no column %q", n)
		}
		if seen[l] {
			return fmt.Errorf("column %q named twice", n)
		}
		seen[l] = true
		out = append(out, l)
	}
	// Each lane keeps its own lines; make sure every lane but the last ends
	// with blank lines before the next heading.
	for _, l := range out {
		if k := len(l.elems); k == 0 || l.elems[k-1].item != nil || l.elems[k-1].raw != "" {
			l.elems = append(l.elems, elem{raw: ""}, elem{raw: ""})
		}
	}
	b.Lanes = out
	b.changed = true
	return nil
}
