package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestScrolledRunPromptSticksAndSwitches(t *testing.T) {
	m := New(".")
	m.width, m.height, m.taskID = 40, 14, "test"
	m.appendRun("You: 第一轮需求", "第一轮需求")
	m.appendResponse(strings.Repeat("first answer\n", 12))
	m.appendRun("You: 第二轮需求", "第二轮需求")
	m.appendResponse(strings.Repeat("second answer\n", 12))
	if got := ansi.Strip(m.contentRows()[1]); !strings.Contains(got, "› 第二轮需求") {
		t.Fatalf("latest prompt missing: %q", got)
	}
	m.scrollOutput(-1000)
	if got := strings.TrimSpace(ansi.Strip(m.contentRows()[1])); got != "" {
		t.Fatalf("duplicated visible run heading: %q", got)
	}
	m.scrollOutput(4)
	if got := ansi.Strip(m.contentRows()[1]); !strings.Contains(got, "› 第一轮需求") {
		t.Fatalf("historical prompt missing: %q", got)
	}
	m.selection = textSelection{anchor: screenPoint{2, 1}, focus: screenPoint{15, 1}}
	if got := m.selectedText(); got != "› 第一轮需求" {
		t.Fatalf("sticky selection = %q", got)
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = updated.(Model)
	if m.viewport.top <= 4 {
		t.Fatal("page down did not scroll")
	}
	m.followOutput()
	if !strings.Contains(ansi.Strip(m.contentRows()[1]), "› 第二轮需求") {
		t.Fatal("return to bottom kept old prompt")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 5, Height: 5}, {Width: 1, Height: 1}, {Width: 0, Height: 0}, {Width: 40, Height: 14}} {
		updated, _ := m.Update(size)
		m = updated.(Model)
		for _, row := range strings.Split(m.View().Content, "\n") {
			if ansi.StringWidth(row) > size.Width {
				t.Fatalf("row exceeds width: %q", row)
			}
		}
	}
}

func TestStickyPromptClickJumpsToOwningRunWithoutBreakingDrag(t *testing.T) {
	m := New(".")
	m.width, m.height, m.taskID = 50, 14, "test"
	// Identical goals must still resolve to the run being read.
	for range 2 {
		m.appendRun("You: repeated goal", "repeated goal")
		m.appendResponse(strings.Repeat("answer\n", 20))
	}
	update := func(msg tea.Msg) { next, _ := m.Update(msg); m = next.(Model) }
	click := func(x int) {
		update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: 1})
		update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x, Y: 1})
	}
	// The mouse press alone must not navigate, so a drag can select text.
	start, _ := m.outputRange(len(m.outputRows()))
	update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 2, Y: 1})
	if got, _ := m.outputRange(len(m.outputRows())); got != start {
		t.Fatal("mouse press jumped before release")
	}
	update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 9, Y: 1})
	update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: 9, Y: 1})
	if !m.selection.active() {
		t.Fatal("drag lost selection")
	}
	if got, _ := m.outputRange(len(m.outputRows())); got != start {
		t.Fatal("drag unexpectedly navigated")
	}
	click(49)
	if got, _ := m.outputRange(len(m.outputRows())); got != start {
		t.Fatal("padding click navigated")
	}
	click(3)
	top, _ := m.outputBounds()
	if got := ansi.Strip(m.contentRows()[top+1]); !strings.Contains(got, "› repeated goal") {
		t.Fatalf("heading not visible after jump: %q", got)
	}
	secondStart, _ := m.outputRange(len(m.outputRows()))
	if secondStart == 0 || !m.viewport.paused {
		t.Fatal("did not navigate to second run in paused mode")
	}
	m.appendResponse("new live output")
	if got, _ := m.outputRange(len(m.outputRows())); got != secondStart {
		t.Fatal("new output stole reading position")
	}
	m.scrollOutput(-1000)
	m.scrollOutput(4)
	click(3)
	if got, _ := m.outputRange(len(m.outputRows())); got != 0 {
		t.Fatalf("historical prompt jumped to wrong run: %d", got)
	}
}
