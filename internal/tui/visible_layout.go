package tui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type visibleOutputRow struct {
	text, accentColor string
	accentEnd         int
	taskCard          bool
}

type visibleLayout struct {
	rows                          []visibleOutputRow
	header, footer, prompt, timer string
	top, bottom                   int
}

// Selection is tied to visible cells, not the version of background output.
// Compare layout data only while a gesture exists; never render whole screens
// merely to decide whether an incoming event invalidated their coordinates.
func (m Model) selectionLayout() *visibleLayout {
	if !m.selection.active() && !m.selection.dragging {
		return nil
	}
	state := &visibleLayout{}
	if m.width == 0 || m.height == 0 {
		return state
	}
	clip := func(text string) string { return ansi.Truncate(text, m.contentWidth(), "") }
	geometry := m.layout()
	if m.taskID != "" && geometry.inputStart > 0 {
		state.header = clip("Task " + strings.ReplaceAll(safeRemote(m.taskID)+" · "+m.taskStatus(), "\n", `\n`))
		rows := m.outputLayout()
		start, end := m.outputRange(len(rows))
		state.top, state.bottom = m.outputBounds()
		for _, row := range rows[start:end] {
			state.rows = append(state.rows, visibleOutputRow{row.text, row.accentColor, row.accentEnd, row.taskCard})
		}
		if geometry.inputStart > 2 && start < len(rows) && rows[start].prompt != "" && !rows[start].runHeading {
			state.prompt = ansi.Truncate("› "+strings.ReplaceAll(safeRemote(rows[start].prompt), "\n", " "), m.contentWidth(), "…")
		}
		if m.returnButtonVisible(len(rows)) {
			state.timer = clip(returnButtonLabel)
		} else if m.timerVisible() {
			state.timer = clip("• Working (" + elapsedLabel(m.timerNow.Sub(m.startedAt)) + " • esc to interrupt)")
		}
	}
	if geometry.statusHeight > 0 {
		var parts []string
		if m.conversationID != "" {
			parts = append(parts, "Conversation "+strings.ReplaceAll(safeRemote(m.conversationID), "\n", `\n`))
		}
		if strings.TrimSpace(m.modelName) != "" {
			parts = append(parts, strings.ReplaceAll(safeRemote(m.modelName), "\n", `\n`))
		}
		if usage := m.usage.label(); usage != "" {
			parts = append(parts, usage)
		}
		state.footer = clip(strings.Join(parts, " · "))
	}
	return state
}

func (m *Model) clearChangedSelection(before *visibleLayout) {
	if before == nil {
		return
	}
	after := m.selectionLayout()
	if after == nil || before.header != after.header || before.footer != after.footer || before.prompt != after.prompt || before.timer != after.timer || before.top != after.top || before.bottom != after.bottom || !slices.Equal(before.rows, after.rows) {
		m.selection = textSelection{}
	}
}
