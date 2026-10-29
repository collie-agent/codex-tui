// Package tui owns the application's event and rendering boundary.
package tui

import (
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	background      = "#303030"
	inputBackground = "#484848"
	muted           = "#A0A0A0"
)

// Model implements Bubble Tea's model; terminal dimensions arrive as events.
type Model struct {
	directory     string
	width, height int
	input         lineInput
	selection     textSelection
}

func New(directory string) Model {
	// Escape terminal controls while keeping ordinary Unicode paths readable.
	var safe strings.Builder
	for _, r := range directory {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			quoted := strconv.QuoteRune(r)
			safe.WriteString(quoted[1 : len(quoted)-1])
		} else {
			safe.WriteRune(r)
		}
	}
	return Model{directory: safe.String()}
}
func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.selection = textSelection{}
		m.width, m.height = max(0, msg.Width), max(0, msg.Height)
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft && m.width > 0 && m.height > 0 {
			point := m.mousePoint(msg.Mouse())
			m.selection = textSelection{anchor: point, focus: point, dragging: true}
		}
	case tea.MouseMotionMsg:
		if m.selection.dragging && msg.Button == tea.MouseLeft {
			m.selection.focus = m.mousePoint(msg.Mouse())
		}
	case tea.MouseReleaseMsg:
		if m.selection.dragging && msg.Button == tea.MouseLeft {
			m.selection.focus = m.mousePoint(msg.Mouse())
			m.selection.dragging = false
		}
	case tea.KeyPressMsg:
		// Screen coordinates become stale when editing or scrolling the input.
		if msg.String() != "ctrl+c" && msg.String() != "enter" && msg.String() != "ctrl+d" {
			m.selection = textSelection{}
		}
		switch msg.String() {
		case "ctrl+d":
			return m, tea.Quit
		case "ctrl+c":
			if m.selection.active() {
				return m, tea.SetClipboard(m.selectedText())
			}
		case "enter":
			// Enter never submits this preview.
		case "backspace":
			m.input.backspace()
		case "delete":
			m.input.delete()
		case "left":
			m.input.caret = m.input.previous()
		case "right":
			m.input.caret = m.input.next()
		case "home":
			m.input.caret = 0
		case "end":
			m.input.caret = len(m.input.text)
		default:
			m.input.insert(msg.Text)
		}
	case tea.PasteMsg:
		m.selection = textSelection{}
		m.input.insert(msg.Content)
	}
	return m, nil
}

func (m Model) View() tea.View {
	view := tea.NewView(m.highlight(m.render()))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	if m.width > 0 && m.height > 0 {
		_, x := m.input.display(m.width)
		layout := m.layout()
		view.Cursor = tea.NewCursor(x, layout.inputStart+layout.inputHeight/2)
	}
	return view
}

type screenLayout struct{ inputStart, inputHeight, statusHeight int }

func (m Model) layout() screenLayout {
	// Reserve input first, then status; the remaining rows belong to the header.
	inputHeight := min(3, m.height)
	statusHeight := min(1, m.height-inputHeight)
	return screenLayout{m.height - inputHeight - statusHeight, inputHeight, statusHeight}
}

func (m Model) render() string {
	rows := m.contentRows()
	layout := m.layout()
	for i, text := range rows {
		color := background
		if i >= layout.inputStart && i < layout.inputStart+layout.inputHeight {
			color = inputBackground
		}
		padding := strings.Repeat(" ", max(0, m.width-ansi.StringWidth(text)))
		rows[i] = text + lipgloss.NewStyle().Background(lipgloss.Color(color)).Render(padding)
	}
	return strings.Join(rows, "\n")
}

// contentRows excludes layout padding so copying can preserve real trailing spaces.
func (m Model) contentRows() []string {
	if m.width == 0 || m.height == 0 {
		return nil
	}
	base := lipgloss.NewStyle().Background(lipgloss.Color(background)).Foreground(lipgloss.Color(muted))
	paint := func(text, color string) string {
		return base.Foreground(lipgloss.Color(color)).Render(text)
	}
	row := func(text string, input bool) string {
		style := base
		if input {
			style = style.Background(lipgloss.Color(inputBackground))
		}
		// Truncate by terminal cells before padding, so narrow screens never wrap.
		text = ansi.Truncate(text, m.width, "")
		return style.Render(text)
	}
	rows := make([]string, m.height)
	for i := range rows {
		rows[i] = row("", false)
	}
	layout := m.layout()
	inputHeight, statusHeight, inputStart := layout.inputHeight, layout.statusHeight, layout.inputStart
	header := []string{"", paint(">_", "#59A9ED") + " " + base.Foreground(lipgloss.Color("#EEEEEE")).Bold(true).Render("OpenAI Codex") + " (v0.165.0)", "   " + m.directory}
	for i, text := range header {
		if i < inputStart {
			rows[i] = row(text, false)
		}
	}
	for i := 0; i < inputHeight; i++ {
		text := ""
		if i == inputHeight/2 {
			text, _ = m.input.display(m.width)
		}
		rows[inputStart+i] = row(text, true)
	}
	if statusHeight > 0 {
		status := "  " + paint("GPT-6.1 Terra medium", "#E8BD37") + " · " + paint("0 in", "#C18B9E") + " · " + paint("weekly 88% left", "#C18B9E") + " · " + paint("5h 100% left", "#C18B9E")
		rows[m.height-1] = row(status, false)
	}
	return rows
}
