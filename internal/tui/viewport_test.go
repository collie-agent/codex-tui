package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestHistoryScrollAndReturn(t *testing.T) {
	m := New(".")
	m.width, m.height, m.taskID = 30, 10, "test"
	m.responses = []string{"zero\none\ntwo\nthree\nfour\nfive\nsix\nseven"}
	if !strings.Contains(ansi.Strip(m.View().Content), "seven") {
		t.Fatal("did not follow output")
	}
	updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = updated.(Model)
	before := m.contentRows()[1]
	m.responses = append(m.responses, "eight\nnine")
	if m.contentRows()[1] != before {
		t.Fatal("append moved historical viewport")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "回到底部") {
		t.Fatal("missing return action")
	}
	updated, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.width / 2, Y: m.layout().inputStart - 1})
	m = updated.(Model)
	if !strings.Contains(ansi.Strip(m.View().Content), "nine") {
		t.Fatal("return did not follow output")
	}
}

func TestHistorySelectionAndResize(t *testing.T) {
	m := New(".")
	m.width, m.height, m.taskID = 30, 10, "test"
	m.responses = []string{"世界🙂 hello\none\ntwo\nthree\nfour\nfive\nsix"}
	m.scrollOutput(-100)
	m.selection = textSelection{anchor: screenPoint{9, 1}, focus: screenPoint{2, 1}}
	if got := m.selectedText(); got != "世界🙂 h" {
		t.Fatalf("reverse Unicode selection: %q", got)
	}
	m.scrollOutput(1)
	if m.selection.active() {
		t.Fatal("scroll kept stale selection")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 1, Height: 1}, {Width: 0, Height: 0}, {Width: 8, Height: 10}} {
		updated, _ := m.Update(size)
		m = updated.(Model)
		for _, row := range strings.Split(m.View().Content, "\n") {
			if ansi.StringWidth(row) > size.Width {
				t.Fatal("row exceeds width")
			}
		}
	}
}

func TestHistoryStreamingKeepsStableSelection(t *testing.T) {
	m := New(".")
	m.width, m.height, m.taskID = 30, 10, "test"
	m.responses = []string{"世界🙂 hello\none\ntwo\nthree\nfour\nfive\nsix"}
	m.scrollOutput(-100)
	m.selection = textSelection{anchor: screenPoint{0, 1}, focus: screenPoint{5, 1}}
	before := m.selectedText()
	// Incoming text below the viewport must not erase the historical selection.
	event := taskEvent{Type: "answer_delta", Delta: `{"text":"new output"}`}
	m.applyEvent(event)
	if got := m.selectedText(); got != before {
		t.Fatalf("selection moved: %q -> %q", before, got)
	}
}

func TestReturnButtonAboveInputOnOverflow(t *testing.T) {
	m := New(".")
	m.width, m.height, m.taskID = 40, 12, "test"
	m.responses = []string{strings.Repeat("line\n", 20)}
	if strings.Contains(ansi.Strip(m.View().Content), "回到底部") {
		t.Fatal("button visible while following bottom")
	}
	m.scrollOutput(-3)
	rows := strings.Split(ansi.Strip(m.View().Content), "\n")
	y := m.layout().inputStart - 1
	label := strings.TrimSpace(rows[y])
	if !strings.Contains(label, "回到底部") {
		t.Fatalf("missing button above input: %q", rows[y])
	}
	left := ansi.StringWidth(rows[y][:strings.Index(rows[y], label)])
	if left != (m.width-ansi.StringWidth(label))/2 {
		t.Fatalf("button not centered: %q", rows[y])
	}
	m.scrollOutput(-3)
	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: m.width / 2, Y: y})
	m = updated.(Model)
	if m.viewport.paused || strings.Contains(ansi.Strip(m.View().Content), "回到底部") {
		t.Fatal("button did not resume following output and hide")
	}
	m.scrollOutput(-3)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if m.viewport.paused || strings.Contains(ansi.Strip(m.View().Content), "回到底部") {
		t.Fatal("escape did not resume following output and hide")
	}
	m.scrollOutput(-3)
	m.scrollOutput(100)
	if m.viewport.paused || strings.Contains(ansi.Strip(m.View().Content), "回到底部") {
		t.Fatal("button visible after scrolling to bottom")
	}
	m.responses = []string{"short"}
	if strings.Contains(ansi.Strip(m.View().Content), "回到底部") {
		t.Fatal("button visible without overflow")
	}
}
