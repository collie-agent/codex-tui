package tui

import "github.com/charmbracelet/x/ansi"

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

// outputBounds reserves one blank row above and below the text. Keep this
// geometry shared by rendering, scrolling and tool hit testing on tiny screens.
func (m Model) outputBounds() (int, int) {
	limit := m.layout().inputStart
	top := min(2, limit)
	bottomPadding := 1
	// Reserve the timer and its blank rows in the shared viewport geometry.
	if m.timerVisible() {
		bottomPadding = 3
	}
	return top, max(top, limit-bottomPadding)
}

// The return action occupies the bottom padding row while viewing history.
func (m Model) returnButtonVisible(total int) bool {
	return m.taskID != "" && m.width > 0 && m.outputCapacity(total) > 0 &&
		m.viewport.paused && m.viewport.top < max(0, total-m.outputCapacity(total))
}

func (m Model) outputCapacity(total int) int {
	top, bottom := m.outputBounds()
	return bottom - top
}

const returnButtonLabel = " ↓ 回到底部 · esc "

func (m *Model) followOutput() {
	m.viewport.paused = false
	m.selection = textSelection{}
	m.toolPress = toolPress{}
}

// Only pin once the run heading has left the visible output. Unknown task goals
// (for example an attached stream without history) never get a fabricated title.
func (m Model) hasStickyPrompt() bool {
	if m.taskID == "" || m.layout().inputStart <= 2 {
		return false
	}
	rows := m.outputLayout()
	start, _ := m.outputRange(len(rows))
	return start < len(rows) && rows[start].prompt != "" && !rows[start].runHeading
}

// Only the visible prompt text is clickable; dragging keeps the copy gesture.
func (m Model) stickyTarget(x, y int) (int, bool) {
	if y != 1 || !m.hasStickyPrompt() || x < m.margin() {
		return 0, false
	}
	if x >= ansi.StringWidth(m.contentRows()[1]) {
		return 0, false
	}
	rows := m.outputLayout()
	start, _ := m.outputRange(len(rows))
	return rows[start].runBlock, true
}

// Tiny terminals retain the original viewport instead of squeezing in a timer.
func (m Model) timerVisible() bool {
	return !m.startedAt.IsZero() && !m.completed && m.layout().inputStart >= 10
}
