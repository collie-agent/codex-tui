package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Apply presentation after wrapping. Plain row text remains the source of
// truth for selection, scrolling and tool hit testing.
func styleOutput(row outputRow, text string) string {
	base := lipgloss.NewStyle().Background(lipgloss.Color(background)).Foreground(lipgloss.Color(foreground))
	if row.toggle != nil {
		return base.Foreground(lipgloss.Color(muted)).Render(text)
	}
	if strings.HasPrefix(text, "Error:") {
		return base.Foreground(lipgloss.Color(errorRed)).Render(text)
	}
	prefix, rest := "", text
	if row.accentEnd > 0 {
		prefix = base.Foreground(lipgloss.Color(row.accentColor)).Render(text[:row.accentEnd])
		rest = text[row.accentEnd:]
	}
	rest = base.Render(rest)
	for _, status := range []struct{ label, color string }{{"[failed]", errorRed}, {"[running]", muted}, {"[completed]", commandGreen}} {
		rest = strings.ReplaceAll(rest, status.label, base.Foreground(lipgloss.Color(status.color)).Render(status.label))
	}
	return prefix + rest
}
