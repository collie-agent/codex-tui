package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCreatePreservesDraftAndRequiresCompletion(t *testing.T) {
	var input string
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		input = body.Input
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task_id":"created"}`))
	}))
	defer server.Close()
	m := NewWithTask("", TaskOptions{ServerURL: server.URL})
	m.input = lineInput{text: "  first  ", caret: 9}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("missing POST command")
	}
	next, duplicate := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if duplicate != nil {
		t.Fatal("duplicate creation")
	}
	next, _ = m.Update(tea.PasteMsg{Content: "next draft"})
	m = next.(Model)
	next, streamCmd := m.Update(cmd())
	m = next.(Model)
	defer m.Close()
	if input != "  first  " || posts != 1 || streamCmd == nil {
		t.Fatalf("POST input=%q posts=%d", input, posts)
	}
	if m.input.text != "next draft" {
		t.Fatal("draft overwritten")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("running task submitted")
	}
	m.applyEvent(taskEvent{Type: "usage", Delta: `{}`})
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("usage unlocked task")
	}
	m.applyEvent(taskEvent{Type: "task_completed", Delta: `{}`})
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	defer m.Close()
	if cmd == nil || !strings.Contains(strings.Join(m.responses, "\n"), "You: next draft") {
		t.Fatal("next independent task missing")
	}
}
func TestCreateFailureRequiresManualRetry(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posts++; w.WriteHeader(500) }))
	defer server.Close()
	m := NewWithTask("", TaskOptions{ServerURL: server.URL})
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("empty request")
	}
	m.input = lineInput{text: "retry", caret: 5}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, auto := m.Update(cmd())
	m = next.(Model)
	if auto != nil || posts != 1 || m.input.text != "retry" || !strings.Contains(m.connection, "Create error") {
		t.Fatalf("failure not retained: %#v", m)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	defer m.Close()
	if cmd == nil {
		t.Fatal("manual retry unavailable")
	}
}
