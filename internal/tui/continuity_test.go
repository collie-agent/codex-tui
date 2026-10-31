package tui_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func TestContinuousTasksShareConversation(t *testing.T) {
	requests := make(chan map[string]string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			requests <- body
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"task_id":"created"}`)
			return
		}
		if r.URL.Path != "/tasks/created/events" {
			t.Errorf("subscription path: %s", r.URL.Path)
		}
		payload, _ := json.Marshal(map[string]string{"ID": "info", "Type": "task_info", "Delta": `{"run_id":"run-1","conv_id":"conversation-1"}`})
		fmt.Fprintf(w, "data: %s\n\n%s", payload, event("task_completed", ""))
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ServerURL: server.URL}), 160, 14)
	defer func() { model.(tui.Model).Close() }()
	model, _ = model.Update(tea.PasteMsg{Content: "  first  "})
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, cmd = model.Update(receive(t, cmd))
	body := <-requests
	if len(body) != 1 || body["input"] != "  first  " {
		t.Fatalf("first request: %#v", body)
	}
	for i := 0; i < 3; i++ {
		model, cmd = model.Update(receive(t, cmd))
	}
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Conversation conversation-1") {
		t.Fatalf("conversation missing: %s", view)
	}
	if strings.Contains(view, "independent") {
		t.Fatalf("misleading task text: %s", view)
	}
	model, _ = model.Update(tea.PasteMsg{Content: "continue"})
	model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, cmd = model.Update(receive(t, cmd))
	body = <-requests
	if len(body) != 2 || body["input"] != "continue" || body["ConvId"] != "conversation-1" {
		t.Fatalf("continuation request: %#v", body)
	}
	for i := 0; i < 3; i++ {
		model, cmd = model.Update(receive(t, cmd))
	}
	view = ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Completed") || !strings.Contains(view, "Conversation conversation-1") || strings.Contains(view, "error") {
		t.Fatalf("second task did not complete in same conversation: %s", view)
	}
	fresh := resize(t, tui.NewWithTask("", tui.TaskOptions{ServerURL: server.URL}), 160, 14)
	if strings.Contains(ansi.Strip(fresh.View().Content), "conversation-1") {
		t.Fatal("new instance retained conversation")
	}
}

func TestMissingConversationPreventsNextSubmission(t *testing.T) {
	testConversationGuard(t, []string{}, "missing valid task_info", false)
}

// Drive the real HTTP/SSE boundary for each task and observe only the UI and
// whether Enter produces another creation command.
func testConversationGuard(t *testing.T, infos []string, reason string, subsequent bool) {
	t.Helper()
	task := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			task++
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"task_id":"task-%d"}`, task)
			return
		}
		deltas := infos
		if subsequent && task == 1 {
			deltas = []string{`{"conv_id":"original"}`}
		}
		for i, delta := range deltas {
			payload, _ := json.Marshal(map[string]string{"ID": fmt.Sprintf("info-%d", i), "Type": "task_info", "Delta": delta})
			fmt.Fprintf(w, "data: %s\n\n", payload)
		}
		fmt.Fprint(w, event("task_completed", ""))
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ServerURL: server.URL}), 180, 14)
	defer func() { model.(tui.Model).Close() }()
	rounds := 1
	if subsequent {
		rounds = 2
	}
	for round := 0; round < rounds; round++ {
		model, _ = model.Update(tea.PasteMsg{Content: "task"})
		var cmd tea.Cmd
		model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		model, cmd = model.Update(receive(t, cmd))
		count := len(infos) + 2
		if subsequent && round == 0 {
			count = 3
		}
		for i := 0; i < count; i++ {
			model, cmd = model.Update(receive(t, cmd))
		}
	}
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, reason) {
		t.Fatalf("missing reason %q: %s", reason, view)
	}
	if subsequent && !strings.Contains(view, "Conversation original") {
		t.Fatalf("original identity lost: %s", view)
	}
	model, _ = model.Update(tea.PasteMsg{Content: "draft"})
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("unverified conversation allowed another submission")
	}
	if !strings.Contains(ansi.Strip(model.View().Content), "draft") {
		t.Fatal("blocked submission erased draft")
	}
}

func TestEachTaskRequiresValidConversation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		infos      []string
		subsequent bool
	}{
		{"invalid JSON", []string{`{`}, false},
		{"empty conversation", []string{`{"run_id":"run","conv_id":""}`}, false},
		{"blank conversation", []string{`{"conv_id":"  "}`}, false},
		{"next task missing", nil, true},
		{"next task invalid", []string{`null`}, true},
	} {
		t.Run(tc.name, func(t *testing.T) { testConversationGuard(t, tc.infos, "missing valid task_info", tc.subsequent) })
	}
}

func TestConflictingConversationRemainsBlockedAfterCompletion(t *testing.T) {
	testConversationGuard(t, []string{`{"conv_id":"other"}`, `{"conv_id":"original"}`}, "Protocol error: conversation conflict", true)
}
