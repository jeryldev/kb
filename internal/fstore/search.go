package fstore

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// tokens splits text into lowercase words of letters and digits with
// accents removed, so that "Café" and "cafe" are the same word.
func tokens(text string) []string {
	var b strings.Builder
	for _, r := range norm.NFD.String(text) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// SearchNotes finds live notes in which every word of the query starts a
// word of the title, body or tags, ignoring case and accents. Notes
// matching in the title come first, then those with more matches.
func (s *Store) SearchNotes(query string) []*Note {
	words := tokens(query)
	if len(words) == 0 {
		return nil
	}
	type hit struct {
		n       *Note
		inTitle int
		total   int
	}
	var hits []hit
	for _, n := range s.ListNotes() {
		title := tokens(n.Title)
		all := append(append(append([]string{}, title...), tokens(n.Body)...), tokens(n.Tags)...)
		h := hit{n: n}
		matched := true
		for _, w := range words {
			found := false
			for i, t := range all {
				if strings.HasPrefix(t, w) {
					found = true
					h.total++
					if i < len(title) {
						h.inTitle++
					}
				}
			}
			if !found {
				matched = false
				break
			}
		}
		if matched {
			hits = append(hits, h)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].inTitle != hits[j].inTitle {
			return hits[i].inTitle > hits[j].inTitle
		}
		return hits[i].total > hits[j].total
	})
	out := make([]*Note, len(hits))
	for i, h := range hits {
		out[i] = h.n
	}
	return out
}
