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

func TestSpecifiedConversationStartsEmptyAndContinues(t *testing.T) {
	requests := make(chan map[string]string, 4)
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
			t.Errorf("unexpected history/subscription request: %s", r.URL.Path)
		}
		payload, _ := json.Marshal(map[string]string{"ID": "info", "Type": "task_info", "Delta": `{"conv_id":"existing"}`})
		fmt.Fprintf(w, "data: %s\n\n%s", payload, event("task_completed", ""))
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("/work", tui.TaskOptions{ConversationID: "existing", ServerURL: server.URL}), 160, 14)
	defer func() { model.(tui.Model).Close() }()
	if cmd := model.Init(); cmd != nil {
		t.Fatal("specified conversation started network work before submission")
	}
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Ask Codex to do anything") || !strings.Contains(view, "Conversation existing") || strings.Contains(view, "You:") {
		t.Fatalf("expected blank input and specified conversation: %s", view)
	}
	select {
	case request := <-requests:
		t.Fatalf("startup request: %#v", request)
	default:
	}
	for _, input := range []string{"first", "continue"} {
		model, _ = model.Update(tea.PasteMsg{Content: input})
		var cmd tea.Cmd
		model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		model, cmd = model.Update(receive(t, cmd))
		body := <-requests
		if len(body) != 2 || body["ConvId"] != "existing" || body["input"] != input {
			t.Fatalf("specified conversation request: %#v", body)
		}
		for range 3 {
			model, cmd = model.Update(receive(t, cmd))
		}
		view = ansi.Strip(model.View().Content)
		if !strings.Contains(view, "Completed") || !strings.Contains(view, "Conversation existing") || strings.Contains(view, "error") {
			t.Fatalf("specified conversation did not complete: %s", view)
		}
	}
}

func TestSpecifiedConversationRequiresServerConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, delta, reason string
	}{
		{"missing", "", "missing valid task_info"},
		{"invalid JSON", `{`, "missing valid task_info"},
		{"empty identity", `{"conv_id":""}`, "missing valid task_info"},
		{"conflict", `{"conv_id":"other"}`, "Protocol error: conversation conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusCreated)
					fmt.Fprint(w, `{"task_id":"created"}`)
					return
				}
				if tc.delta != "" {
					payload, _ := json.Marshal(map[string]string{"ID": "info", "Type": "task_info", "Delta": tc.delta})
					fmt.Fprintf(w, "data: %s\n\n", payload)
				}
				fmt.Fprint(w, event("task_completed", ""))
			}))
			defer server.Close()
			var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ConversationID: "existing", ServerURL: server.URL}), 180, 14)
			defer func() { model.(tui.Model).Close() }()
			model, _ = model.Update(tea.PasteMsg{Content: "first"})
			model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			model, cmd = model.Update(receive(t, cmd))
			messages := 2
			if tc.delta != "" {
				messages++
			}
			for range messages {
				model, cmd = model.Update(receive(t, cmd))
			}
			view := ansi.Strip(model.View().Content)
			if !strings.Contains(view, tc.reason) || !strings.Contains(view, "Conversation existing") {
				t.Fatalf("expected original identity and failure reason: %s", view)
			}
			model, _ = model.Update(tea.PasteMsg{Content: "retry draft"})
			model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd != nil || !strings.Contains(ansi.Strip(model.View().Content), "retry draft") {
				t.Fatal("unconfirmed task allowed another submission or lost draft")
			}
		})
	}
}

func TestSpecifiedConversationQueuedWithoutEventsKeepsWaiting(t *testing.T) {
	requests := make(chan map[string]string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			requests <- body
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"task_id":"queued"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ConversationID: "unknown", ServerURL: server.URL}), 180, 14)
	defer func() { model.(tui.Model).Close() }()
	model, _ = model.Update(tea.PasteMsg{Content: "wait"})
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, cmd = model.Update(receive(t, cmd))
	if body := <-requests; body["ConvId"] != "unknown" {
		t.Fatalf("queued request changed identity: %#v", body)
	}
	model, _ = model.Update(receive(t, cmd))
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "completion unconfirmed") || !strings.Contains(view, "Conversation unknown") || strings.Contains(view, "Completed") || strings.Contains(view, "verified") {
		t.Fatalf("queued task presented as confirmed: %s", view)
	}
	model, _ = model.Update(tea.PasteMsg{Content: "draft"})
	model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !strings.Contains(ansi.Strip(model.View().Content), "draft") {
		t.Fatal("waiting task allowed another creation or discarded draft")
	}
	select {
	case body := <-requests:
		t.Fatalf("replacement conversation created automatically: %#v", body)
	default:
	}
}

func TestEmptySpecifiedConversationStartsNewConversation(t *testing.T) {
	requests := make(chan map[string]string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- body
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"task_id":"created"}`)
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ConversationID: "", ServerURL: server.URL}), 160, 14)
	defer func() { model.(tui.Model).Close() }()
	if model.Init() != nil || strings.Contains(ansi.Strip(model.View().Content), "Conversation ") {
		t.Fatal("empty conversation selected an existing identity")
	}
	model, _ = model.Update(tea.PasteMsg{Content: "new"})
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = model.Update(receive(t, cmd))
	if body := <-requests; len(body) != 1 || body["input"] != "new" {
		t.Fatalf("empty conversation did not omit ConvId: %#v", body)
	}
}
