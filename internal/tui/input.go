package tui

import tea "github.com/charmbracelet/bubbletea"

// typedText is the text a key event types into a field. Characters typed
// faster than kb reads them, or pasted, arrive as one event with several
// runes, so the runes are used rather than msg.String(), which is a single
// character only for one key and wraps a paste in brackets.
func typedText(msg tea.KeyMsg) (string, bool) {
	switch msg.Type {
	case tea.KeyRunes:
		return string(msg.Runes), true
	case tea.KeySpace:
		return " ", true
	}
	return "", false
}
