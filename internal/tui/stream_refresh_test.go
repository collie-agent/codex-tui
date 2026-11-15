package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestStreamRefreshCoalescesDeltasAndShowsTail(t *testing.T) {
	m := performanceFixture(t, 0, false)
	m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = performanceEvent(m, "answer_delta", `{"text":" first"}`)
	first := ansi.Strip(m.View().Content)
	if !strings.Contains(first, "first") {
		t.Fatal("first delta was not shown immediately")
	}
	m = performanceEvent(m, "answer_delta", `{"text":" second"}`)
	if got := ansi.Strip(m.View().Content); got != first {
		t.Fatal("burst delta redrew before the frame deadline")
	}
	m = performanceUpdate(m, streamRefreshMessage{source: m.stream})
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "first second") || strings.Count(got, "second") != 1 {
		t.Fatalf("tail missing or duplicated: %s", got)
	}
}

func TestStreamRefreshIgnoresWakeFromPreviousTask(t *testing.T) {
	m := performanceFixture(t, 0, false)
	m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = performanceEvent(m, "answer_delta", `{"text":" first"}`)
	first := m.View().Content
	m = performanceEvent(m, "answer_delta", `{"text":" second"}`)
	old := newSubscription(TaskOptions{})
	defer old.cancel()
	m = performanceUpdate(m, streamRefreshMessage{source: old})
	if m.View().Content != first {
		t.Fatal("old task wake released current task frame")
	}
}

func TestStreamRefreshInteractionsAndTerminalEventsShowPendingText(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  tea.Msg
		want string
	}{
		{"input", tea.PasteMsg{Content: "input-ready"}, "input-ready"},
		{"resize", tea.WindowSizeMsg{Width: 70, Height: 30}, "second"},
		{"completion", streamMessage{event: &taskEvent{Type: "task_completed", Delta: `{}`}}, "Completed"},
		{"interruption", streamMessage{event: &taskEvent{Type: "task_interrupted", Delta: `{}`}}, "Interrupted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := performanceFixture(t, 0, false)
			m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
			m = performanceEvent(m, "answer_delta", `{"text":" first"}`)
			_ = m.View()
			m = performanceEvent(m, "answer_delta", `{"text":" second"}`)
			m = performanceUpdate(m, tc.msg)
			got := ansi.Strip(m.View().Content)
			if !strings.Contains(got, "first second") || !strings.Contains(got, tc.want) {
				t.Fatalf("pending text or state missing: %s", got)
			}
		})
	}
}

func TestStreamRefreshCloseCancelsPendingSubscriptionCommand(t *testing.T) {
	m := performanceFixture(t, 0, false)
	updated, cmd := m.Update(streamMessage{source: m.stream, event: &taskEvent{Type: "answer_delta", Delta: `{"text":"tail"}`}})
	m = updated.(Model)
	m.Close()
	if cmd == nil {
		t.Fatal("missing pending subscription command")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("closed subscription returned refresh work: %T", msg)
	}
}

func TestStreamRefreshPreservesPreviouslyRenderedModel(t *testing.T) {
	m := performanceFixture(t, 0, false)
	m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = performanceEvent(m, "answer_delta", `{"text":" first"}`)
	first := m.View().Content
	next := performanceEvent(m, "answer_delta", `{"text":" second"}`)
	next = performanceUpdate(next, streamRefreshMessage{source: next.stream})
	if !strings.Contains(next.View().Content, "second") {
		t.Fatal("latest model did not refresh")
	}
	if m.View().Content != first {
		t.Fatal("new model rendering changed the previous frame")
	}
}
