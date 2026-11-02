package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestRunTimerLifecycle(t *testing.T) {
	m := NewWithTask("", TaskOptions{TaskID: "task"})
	defer m.Close()
	m.width, m.height = 100, 30
	start := time.Now().Add(-183 * time.Second)
	m.startedAt, m.timerNow = start, start.Add(7*time.Second)
	if got := strings.Join(m.contentRows(), "\n"); !strings.Contains(got, "Working (7s • esc to interrupt)") {
		t.Fatalf("missing running timer: %s", got)
	}
	updated, cmd := m.Update(streamMessage{source: m.stream, tick: true})
	m = updated.(Model)
	if cmd == nil || !strings.Contains(strings.Join(m.contentRows(), "\n"), "Working (3m 3s") {
		t.Fatal("idle tick did not refresh timer")
	}
	m.applyEvent(taskEvent{Type: "task_info", Delta: `{"conv_id":"conv"}`})
	m.applyEvent(taskEvent{Type: "task_completed", Delta: `{}`})
	got := strings.Join(m.contentRows(), "\n")
	if strings.Contains(got, "Working (") || !strings.Contains(got, "Worked for 3m 3s • "+time.Now().Format("3:04 PM")) {
		t.Fatalf("incorrect completed timer: %s", got)
	}
	m.input = lineInput{text: "next"}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	defer m.Close()
	if !strings.Contains(strings.Join(m.contentRows(), "\n"), "Worked for 3m 3s") {
		t.Fatal("next task removed previous elapsed time")
	}
	m.startTimer()
	if !strings.Contains(strings.Join(m.contentRows(), "\n"), "Working (0s") {
		t.Fatal("next task did not reset timer")
	}
}

func TestInterruptionTimerDoesNotClaimSuccess(t *testing.T) {
	m := NewWithTask("", TaskOptions{TaskID: "task"})
	defer m.Close()
	m.width, m.height = 100, 30
	m.applyEvent(taskEvent{Type: "task_interrupted", Delta: `{}`})
	got := strings.Join(m.contentRows(), "\n")
	if !strings.Contains(got, "Interrupted after 0s") || strings.Contains(got, "Worked for") || strings.Contains(got, "Working (") {
		t.Fatalf("incorrect interruption timer: %s", got)
	}
}
