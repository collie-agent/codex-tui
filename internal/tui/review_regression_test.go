package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUsageBetweenDeltaAndFinalText(t *testing.T) {
	m := New(".")
	m.width, m.height, m.taskID = 80, 20, "test"
	for _, event := range []taskEvent{
		{Type: "answer_delta", Delta: `{"text":"hello"}`},
		{Type: "usage", Delta: `{"status":"completed"}`},
		{Type: "text", Delta: `{"text":"hello"}`},
	} {
		m.applyEvent(event)
	}
	if got := strings.Count(ansi.Strip(m.View().Content), "hello"); got != 1 {
		t.Fatalf("response rendered %d times", got)
	}
	// Identical text in a later response must remain visible as a separate response.
	m.applyEvent(taskEvent{Type: "answer_delta", Delta: `{"text":"hello"}`})
	if got := strings.Count(ansi.Strip(m.View().Content), "hello"); got != 2 {
		t.Fatalf("later response lost: %d", got)
	}
}

func TestEmptyTextEventsDoNotCreateResponses(t *testing.T) {
	m := New(".")
	for _, kind := range []string{"answer_delta", "text_delta", "text"} {
		m.applyEvent(taskEvent{Type: kind, Delta: `{"text":""}`})
	}
	if len(m.responses) != 0 || len(m.output) != 0 {
		t.Fatalf("empty events created output: responses=%v output=%v", m.responses, m.output)
	}
	m.applyEvent(taskEvent{Type: "answer_delta", Delta: `{"text":"hello"}`})
	m.applyEvent(taskEvent{Type: "text", Delta: `{"text":""}`})
	m.applyEvent(taskEvent{Type: "answer_delta", Delta: `{"text":"next"}`})
	if len(m.responses) != 2 || m.responses[0] != "hello" || m.responses[1] != "next" {
		t.Fatalf("empty final text did not close the round: %v", m.responses)
	}
}

func TestPausedHistoryKeepsTaskStatus(t *testing.T) {
	for _, status := range []string{"Completed", "Connection error: EOF"} {
		t.Run(status, func(t *testing.T) {
			m := New(".")
			m.width, m.height, m.taskID, m.connection = 80, 10, "test", status
			m.responses = []string{strings.Repeat("history\n", 30)}
			updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
			view := ansi.Strip(updated.View().Content)
			if !strings.Contains(view, status) || !strings.Contains(view, "回到底部") {
				t.Fatalf("missing status or action: %s", view)
			}
		})
	}
}

func TestResizeAnchorsActualContent(t *testing.T) {
	for _, prefix := range []string{"", strings.Repeat("long ", 60) + "\n", "世界🙂" + strings.Repeat("文", 90) + "\n"} {
		m := New(".")
		m.width, m.height, m.taskID = 100, 10, "test"
		var content strings.Builder
		content.WriteString(prefix)
		for i := 0; i < 100; i++ {
			fmt.Fprintf(&content, "line %03d\n", i)
		}
		m.responses = []string{content.String()}
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
		m = updated.(Model)
		before := ansi.Strip(m.contentRows()[1])
		for _, width := range []int{50, 120, 25} {
			updated, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 10})
			m = updated.(Model)
			if got := ansi.Strip(m.contentRows()[1]); got != before {
				t.Fatalf("width %d moved %q to %q", width, before, got)
			}
		}
	}
}
