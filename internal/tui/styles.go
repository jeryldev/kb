package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleBarStyle = lipgloss.NewStyle().
			Bold(true).
			Reverse(true).
			Padding(0, 1)

	columnHeaderStyle = lipgloss.NewStyle().
				Bold(true)

	columnHeaderActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Underline(true)

	cardNormalBorder = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				Faint(true).
				Padding(0, 1)

	cardSelectedBorder = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				Bold(true).
				Padding(0, 1)

	priorityStyles = map[string]lipgloss.Style{
		"urgent": lipgloss.NewStyle().Bold(true).Italic(true).
			Foreground(lipgloss.AdaptiveColor{Light: "1", Dark: "9"}),
		"high": lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "3", Dark: "11"}),
		"medium": lipgloss.NewStyle(),
		"low":    lipgloss.NewStyle().Faint(true),
	}

	labelStyle = lipgloss.NewStyle().
			Faint(true).
			Italic(true)

	helpStyle = lipgloss.NewStyle().
			Faint(true)

	statusBarStyle = lipgloss.NewStyle().
			Faint(true).
			Reverse(true).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Bold(true)

	dialogBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			Padding(1, 2)

	formLabelStyle = lipgloss.NewStyle().
			Faint(true)

	formLabelActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Underline(true)

	formValueStyle = lipgloss.NewStyle()

	emptyColumnStyle = lipgloss.NewStyle().
				Faint(true).
				Italic(true)

	filterBarStyle = lipgloss.NewStyle().
			Bold(true)
)

func priorityStyle(priority string) lipgloss.Style {
	if s, ok := priorityStyles[priority]; ok {
		return s
	}
	return lipgloss.NewStyle()
}

// renderCenteredConfirm asks a yes/no question in a box, the prompt
// wrapped to fit; a terminal too small for the box gets the lines alone.
func renderCenteredConfirm(width, height int, prompt string) string {
	const hint = "y: confirm   n: cancel"
	// The border and padding take 6 cells across and 4 lines down.
	inner := min(max(lipgloss.Width(prompt), len(hint)), width-6)
	content := lipgloss.JoinVertical(lipgloss.Center,
		errorStyle.Width(max(1, inner)).Align(lipgloss.Center).Render(prompt),
		"",
		helpStyle.Render(hint),
	)
	dialog := dialogBoxStyle.Render(content)
	if lipgloss.Width(dialog) > width || lipgloss.Height(dialog) > height {
		content = lipgloss.JoinVertical(lipgloss.Left,
			errorStyle.Width(max(1, width)).Render(prompt), helpStyle.Render(hint))
		return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Center, content)
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, dialog)
}
