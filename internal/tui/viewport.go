package tui

// A paused viewport is anchored from the beginning, so appended output cannot
// steal the reader's position. Selection uses this same visible layout.
type outputViewport struct {
	top    int
	paused bool
}

func (m Model) outputRange(total int) (int, int) {
	visible := m.outputCapacity(total)
	top := max(0, total-visible)
	if m.viewport.paused {
		top = min(top, max(0, m.viewport.top))
	}
	return top, min(total, top+visible)
}

func (m *Model) clampOutput() {
	start, _ := m.outputRange(len(m.outputRows()))
	m.viewport.top = start
}

func (m *Model) scrollOutput(delta int) {
	total := len(m.outputRows())
	start, _ := m.outputRange(total)
	bottom := max(0, total-m.outputCapacity(total))
	m.viewport.top = min(bottom, max(0, start+delta))
	m.viewport.paused = m.viewport.top < bottom
	// Screen coordinates no longer identify the selected text after scrolling.
	m.selection = textSelection{}
}

// Reserve the action row only while viewing history above the bottom.
// Rendering, scrolling and hit testing all use the same terminal-cell geometry.
func (m Model) returnButtonVisible(total int) bool {
	return m.taskID != "" && m.width > 0 && m.layout().inputStart >= 3 &&
		m.viewport.paused && total > m.layout().inputStart-1 &&
		m.viewport.top < total-(m.layout().inputStart-2)
}

func (m Model) outputCapacity(total int) int {
	visible := max(0, m.layout().inputStart-1)
	if m.returnButtonVisible(total) {
		visible--
	}
	return visible
}

const returnButtonLabel = " ↓ 回到底部 · esc "

func (m *Model) followOutput() {
	m.viewport.paused = false
	m.selection = textSelection{}
	m.toolPress = toolPress{}
}
