package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

type screenPoint struct{ x, y int }

// textSelection stores inclusive terminal cells, independent of drag direction.
// A click without movement clears the selection without selecting a character.
type textSelection struct {
	anchor, focus screenPoint
	dragging      bool
}

func (s textSelection) active() bool { return s.anchor != s.focus }

func (m Model) mousePoint(mouse tea.Mouse) screenPoint {
	return screenPoint{max(0, min(mouse.X, m.width-1)), max(0, min(mouse.Y, m.height-1))}
}

// span is shared by highlighting and copying. Either half of a wide grapheme
// selects the whole grapheme; ANSI cutting must never replace it with padding.
func (s textSelection) span(line string, y int) (int, int) {
	first, last := s.anchor, s.focus
	if first.y > last.y || (first.y == last.y && first.x > last.x) {
		first, last = last, first
	}
	if !s.active() || y < first.y || y > last.y {
		return 0, 0
	}
	start, end := 0, ansi.StringWidth(line)
	if y == first.y {
		start = first.x
	}
	if y == last.y {
		end = last.x + 1
	}
	x := 0
	graphemes := uniseg.NewGraphemes(ansi.Strip(line))
	for graphemes.Next() {
		next := x + ansi.StringWidth(graphemes.Str())
		if x < start && start < next {
			start = x
		}
		if x < end && end < next {
			end = next
		}
		x = next
	}
	return start, end
}

func (m Model) selectedText() string {
	var selected []string
	content := m.contentRows()
	for y, line := range strings.Split(m.render(), "\n") {
		start, end := m.selection.span(line, y)
		if end > start {
			// Cut unpadded content, preserving selected spaces and blank-row newlines.
			selected = append(selected, ansi.Cut(ansi.Strip(content[y]), start, end))
		}
	}
	return strings.Join(selected, "\n")
}

func (m Model) highlight(content string) string {
	if !m.selection.active() {
		return content
	}
	style := lipgloss.NewStyle().Background(lipgloss.Color("#A9D1F7")).Foreground(lipgloss.Color("#101820"))
	rows := strings.Split(content, "\n")
	for y, line := range rows {
		start, end := m.selection.span(line, y)
		if end > start {
			selected := ansi.Cut(ansi.Strip(line), start, end)
			rows[y] = ansi.Cut(line, 0, start) + style.Render(selected) + ansi.Cut(line, end, m.width)
		}
	}
	return strings.Join(rows, "\n")
}
