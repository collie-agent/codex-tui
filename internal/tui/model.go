// Package tui owns the application's event and rendering boundary.
package tui

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	background       = "#303030"
	inputBackground  = "#484848"
	muted            = "#A0A0A0"
	foreground       = "#EEEEEE"
	promptBackground = "#505050"
	actionBlue       = "#75BFFF"
	commandGreen     = "#8FCA73"
	errorRed         = "#F16D66"
)

// Model implements Bubble Tea's model; terminal dimensions arrive as events.
type Model struct {
	agents                map[string]Model
	agentOrder            []string
	activeAgent           string
	readOnly              bool
	modelName             string
	usage                 tokenUsage
	directory             string
	width, height         int
	input                 lineInput
	selection             textSelection
	options               TaskOptions
	history               *historyLoad
	create                *creation
	stream                *subscription
	taskID, connection    string
	runID                 string
	interrupt             *interruption
	interruptState        interruptionState
	interruptStatus       string
	completed             bool
	startedAt             time.Time
	timerNow              time.Time
	conversationID        string
	conversationConfirmed bool
	conversationError     string
	responses             []string
	streaming             bool
	textInterrupted       bool
	viewport              outputViewport
	output                []outputItem
	tools                 map[string]*toolOutput
	toolPress             toolPress
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
func (m Model) Init() tea.Cmd {
	if m.history != nil {
		return m.loadHistory
	}
	if m.stream == nil {
		return nil
	}
	return func() tea.Msg { go m.stream.run(); return m.stream.next() }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The root owns transport and task commands; each agent owns its reading state.
	if key, ok := msg.(tea.KeyPressMsg); ok && len(m.agentOrder) > 0 && (key.String() == "tab" || key.String() == "shift+tab") {
		m.switchAgent(key.String() == "shift+tab")
		return m, nil
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		for name, agent := range m.agents {
			updated, _ := agent.Update(size)
			m.agents[name] = updated.(Model)
		}
	}
	if m.activeAgent != "" {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg, tea.MouseWheelMsg, tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
			if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+d" {
				m.Close()
				return m, tea.Quit
			}
			agent := m.agents[m.activeAgent]
			updated, cmd := agent.Update(msg)
			m.agents[m.activeAgent] = updated.(Model)
			return m, cmd
		}
	}
	if m.readOnly {
		switch input := msg.(type) {
		case tea.PasteMsg:
			return m, nil
		case tea.KeyPressMsg:
			// Reuse all reading gestures, but never create or interrupt a task here.
			switch input.String() {
			case "pgup", "pgdown", "ctrl+c", "ctrl+d":
			case "esc":
				m.followOutput()
				return m, nil
			default:
				return m, nil
			}
		}
	}
	switch msg := msg.(type) {
	case historyMessage:
		return m.finishHistory(msg)
	case interruptMessage:
		return m.finishInterruption(msg)
	case createMessage:
		return m.finishCreation(msg)
	case streamMessage:
		if (msg.source != nil && msg.source != m.stream) || m.completed {
			return m, nil
		}
		m.timerNow = time.Now()
		if msg.tick {
			return m, m.stream.next
		}
		before := strings.Join(m.contentRows(), "\n")
		if msg.err != nil {
			// Transport recovery must never hide a conversation ownership failure.
			if m.conversationError == "" {
				m.connection = "Connection error: " + safeRemote(msg.err.Error())
			}
			return m, m.stream.next
		}
		if msg.connected && m.conversationError == "" {
			m.connection = "Connected · completion unconfirmed"
		}
		if msg.event != nil {
			m.applyEvent(*msg.event)
			m.stream.acknowledge(*msg.event)
		}
		if before != strings.Join(m.contentRows(), "\n") {
			m.selection = textSelection{}
		}
		if m.completed {
			m.Close()
			return m, nil
		}
		if m.interruptState == interruptDeferred && m.runID != "" {
			updated, interruptCmd := m.requestInterruption()
			return updated, tea.Batch(interruptCmd, m.stream.next)
		}
		return m, m.stream.next
	case tea.WindowSizeMsg:
		m.selection = textSelection{}
		oldRows := m.outputLayout()
		start, _ := m.outputRange(len(oldRows))
		m.width, m.height = max(0, msg.Width), max(0, msg.Height)
		if m.viewport.paused && start < len(oldRows) {
			anchor := oldRows[start]
			// Locate the same logical line and the wrapped segment containing its offset.
			for i, row := range m.outputLayout() {
				if row.block == anchor.block && row.offset <= anchor.offset {
					m.viewport.top = i
				}
			}
		}
		m.clampOutput()
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			m.scrollOutput(-3)
		} else if msg.Button == tea.MouseWheelDown {
			m.scrollOutput(3)
		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft && m.width > 0 && m.height > 0 {
			buttonWidth := min(m.contentWidth(), ansi.StringWidth(returnButtonLabel))
			buttonLeft := (m.width - buttonWidth) / 2
			if m.returnButtonVisible(len(m.outputRows())) && msg.Y == m.layout().inputStart-1 && msg.X >= buttonLeft && msg.X < buttonLeft+buttonWidth {
				m.followOutput()
				return m, nil
			}
			target, sticky := m.stickyTarget(msg.X, msg.Y)
			m.toolPress = toolPress{tool: m.toolAt(msg.X, msg.Y), x: msg.X, y: msg.Y, sticky: sticky, targetBlock: target}
			point := m.mousePoint(msg.Mouse())
			m.selection = textSelection{anchor: point, focus: point, dragging: true}
		}
	case tea.MouseMotionMsg:
		if m.selection.dragging && (msg.X != m.toolPress.x || msg.Y != m.toolPress.y) {
			m.toolPress.moved = true
		}
		if m.selection.dragging && msg.Button == tea.MouseLeft {
			m.selection.focus = m.mousePoint(msg.Mouse())
		}
	case tea.MouseReleaseMsg:
		if m.selection.dragging && msg.Button == tea.MouseLeft {
			m.selection.focus = m.mousePoint(msg.Mouse())
			m.selection.dragging = false
			if p := m.toolPress; p.tool != nil && !p.moved && msg.X == p.x && msg.Y == p.y && m.toolAt(msg.X, msg.Y) == p.tool {
				start, _ := m.outputRange(len(m.outputRows()))
				p.tool.expanded = !p.tool.expanded
				m.viewport = outputViewport{top: start, paused: true}
				m.clampOutput()
				m.selection = textSelection{}
			}
			if p := m.toolPress; p.sticky && !p.moved && msg.X == p.x && msg.Y == p.y {
				if target, ok := m.stickyTarget(msg.X, msg.Y); ok && target == p.targetBlock {
					// Resolve the logical heading again: streaming or wrapping may
					// have changed its physical row since the mouse press.
					for i, row := range m.outputLayout() {
						if row.block == target {
							m.viewport = outputViewport{top: i, paused: true}
							m.clampOutput()
							m.selection = textSelection{}
							break
						}
					}
				}
			}
			m.toolPress = toolPress{}
		}
	case tea.KeyPressMsg:
		// Screen coordinates become stale when editing or scrolling the input.
		if msg.String() != "ctrl+c" && msg.String() != "enter" && msg.String() != "ctrl+d" {
			m.selection = textSelection{}
		}
		switch msg.String() {
		case "esc":
			if !m.completed && (m.create != nil || m.stream != nil) {
				if m.runID == "" {
					m.interruptState = interruptDeferred
					m.interruptStatus = "Waiting for run to start before interruption"
					return m, nil
				}
				return m.requestInterruption()
			}
			if m.returnButtonVisible(len(m.outputRows())) {
				m.followOutput()
			}
		case "pgup":
			m.scrollOutput(-max(1, m.outputCapacity(len(m.outputRows()))-1))
		case "pgdown":
			m.scrollOutput(max(1, m.outputCapacity(len(m.outputRows()))-1))
		case "ctrl+d":
			m.Close()
			return m, tea.Quit
		case "ctrl+c":
			if m.selection.active() {
				return m, copyToClipboard(m.selectedText())
			}
		case "enter":
			return m.submitTask()
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
	if m.activeAgent != "" {
		return m.agents[m.activeAgent].View()
	}
	view := tea.NewView(m.highlight(m.render()))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	if !m.readOnly && m.width > 0 && m.height > 0 {
		_, x := m.input.display(m.contentWidth())
		x += m.margin()
		layout := m.layout()
		view.Cursor = tea.NewCursor(x, layout.inputStart+layout.inputHeight/2)
	}
	return view
}

// Preserve a usable cell on tiny terminals while keeping symmetric margins.
func (m Model) margin() int       { return min(2, max(0, (m.width-1)/2)) }
func (m Model) contentWidth() int { return max(0, m.width-2*m.margin()) }

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
	output := m.outputLayout()
	start, end := m.outputRange(len(output))
	top, _ := m.outputBounds()
	for i, text := range rows {
		color := background
		if i >= layout.inputStart && i < layout.inputStart+layout.inputHeight {
			color = inputBackground
		}
		if m.taskID != "" && i >= top && i-top < end-start && output[start+i-top].taskCard {
			color = promptBackground
		}
		if i == 1 && m.hasStickyPrompt() {
			color = promptBackground
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
		text = ansi.Truncate(text, m.contentWidth(), "")
		return style.Render(strings.Repeat(" ", m.margin()) + text)
	}
	rows := make([]string, m.height)
	for i := range rows {
		rows[i] = row("", false)
	}
	layout := m.layout()
	inputHeight, statusHeight, inputStart := layout.inputHeight, layout.statusHeight, layout.inputStart
	if m.taskID != "" && inputStart > 0 {
		output := m.outputRows()
		start, end := m.outputRange(len(output))
		output = output[start:end]
		for i := 0; i < inputStart; i++ {
			rows[i] = row("", false)
		}
		rows[0] = row("Task "+strings.ReplaceAll(safeRemote(m.taskID)+" · "+m.taskStatus(), "\n", `\n`), false)
		// Use the reserved top padding row, so pinning never shifts the viewport
		// or changes tool hit testing and copy coordinates.
		visibleRows := m.outputLayout()
		if inputStart > 2 && start < len(visibleRows) {
			first := visibleRows[start]
			if first.prompt != "" && !first.runHeading {
				prompt := "› " + strings.ReplaceAll(safeRemote(first.prompt), "\n", " ")
				prompt = ansi.Truncate(prompt, m.contentWidth(), "…")
				rows[1] = base.Background(lipgloss.Color(promptBackground)).Foreground(lipgloss.Color(foreground)).Render(
					strings.Repeat(" ", m.margin()) + prompt)
			}
		}
		top, _ := m.outputBounds()
		for i, text := range output {
			if visibleRows[start+i].taskCard {
				rows[top+i] = base.Background(lipgloss.Color(promptBackground)).Foreground(lipgloss.Color(foreground)).Render(
					strings.Repeat(" ", m.margin()) + text)
			} else {
				rows[top+i] = row(styleOutput(visibleRows[start+i], text), false)
			}
		}
		if m.timerVisible() && !m.returnButtonVisible(len(m.outputRows())) {
			rows[inputStart-2] = row("• Working ("+elapsedLabel(m.timerNow.Sub(m.startedAt))+" • esc to interrupt)", false)
		}
		if m.returnButtonVisible(len(m.outputRows())) {
			label := ansi.Truncate(returnButtonLabel, m.contentWidth(), "")
			left := (m.contentWidth() - ansi.StringWidth(label)) / 2
			button := base.Background(lipgloss.Color(inputBackground)).Foreground(lipgloss.Color("#59A9ED")).Render(label)
			rows[inputStart-1] = row(strings.Repeat(" ", left)+button, false)
		}
	}
	for i := 0; i < inputHeight; i++ {
		text := ""
		if i == inputHeight/2 {
			text, _ = m.input.display(m.contentWidth())
			if m.readOnly {
				text = "Read only · Tab / Shift+Tab to switch agents"
			}
			if m.input.text != "" {
				text = base.Background(lipgloss.Color(inputBackground)).Foreground(lipgloss.Color(foreground)).Render(text)
			}
		}
		rows[inputStart+i] = row(text, true)
	}
	if statusHeight > 0 {
		var parts []string
		if m.conversationID != "" {
			parts = append(parts, "Conversation "+strings.ReplaceAll(safeRemote(m.conversationID), "\n", `\n`))
		}
		if strings.TrimSpace(m.modelName) != "" {
			parts = append(parts, paint(strings.ReplaceAll(safeRemote(m.modelName), "\n", `\n`), "#E8BD37"))
		}
		if usage := m.usage.label(); usage != "" {
			parts = append(parts, paint(usage, "#C18B9E"))
		}
		status := strings.Join(parts, " · ")
		rows[m.height-1] = row(status, false)
	}
	return rows
}
