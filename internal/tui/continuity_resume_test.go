package tui_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func conversationFrame(id, conversation string) string {
	delta, _ := json.Marshal(map[string]string{"conv_id": conversation})
	payload, _ := json.Marshal(map[string]string{"ID": id, "Type": "task_info", "Delta": string(delta)})
	return "data: " + string(payload) + "\n\n"
}

func TestConversationConflictRemainsVisibleAcrossReconnect(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"task_id":"conflicted"}`)
			return
		}
		if connections.Add(1) == 1 {
			fmt.Fprint(w, conversationFrame("original", "original"), conversationFrame("conflict", "other"))
			return
		}
		fmt.Fprint(w, conversationFrame("replayed", "original"), event("task_completed", ""))
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ServerURL: server.URL}), 200, 14)
	defer func() { model.(tui.Model).Close() }()
	model, _ = model.Update(tea.PasteMsg{Content: "first"})
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, cmd = model.Update(receive(t, cmd))
	for step := 0; step < 7; step++ {
		model, cmd = model.Update(receive(t, cmd))
		if step < 2 {
			continue
		}
		view := ansi.Strip(model.View().Content)
		if !strings.Contains(view, "Conversation original") || !strings.Contains(view, "Protocol error: conversation conflict") {
			t.Fatalf("conflict hidden at reconnect step %d: %s", step, view)
		}
		model, _ = model.Update(tea.PasteMsg{Content: "draft"})
		if _, submit := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); submit != nil {
			t.Fatalf("conflict allowed submission at step %d", step)
		}
	}
}

func TestResumedConversationDeduplicatesAndRequiresNewTaskConfirmation(t *testing.T) {
	var tasks, connections atomic.Int32
	requests := make(chan map[string]string, 3)
	headers := make(chan string, 3)
	info := conversationFrame("ownership", "original")
	answer := event("answer_delta", "one answer")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			requests <- body
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"task_id":"task-%d"}`, tasks.Add(1))
			return
		}
		if r.URL.Path == "/tasks/task-2/events" {
			fmt.Fprint(w, event("task_completed", ""))
			return
		}
		headers <- r.Header.Get("Last-Event-ID")
		if connections.Add(1) == 1 {
			fmt.Fprint(w, info, answer)
			return
		}
		fmt.Fprint(w, info, answer, conversationFrame("same-owner", "original"), event("task_completed", ""))
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ServerURL: server.URL}), 200, 18)
	defer func() { model.(tui.Model).Close() }()
	model, _ = model.Update(tea.PasteMsg{Content: "first"})
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, cmd = model.Update(receive(t, cmd))
	for step := 0; step < 7; step++ {
		model, cmd = model.Update(receive(t, cmd))
	}
	view := ansi.Strip(model.View().Content)
	if strings.Count(view, "one answer") != 1 || !strings.Contains(view, "Completed") || strings.Contains(view, "error") {
		t.Fatalf("resumed task lost ownership or duplicated answer: %s", view)
	}
	if first, second := <-headers, <-headers; first != "" || second == "" || !strings.Contains(answer, `"ID":"`+second+`"`) {
		t.Fatalf("incorrect resume cursors: %q, %q", first, second)
	}
	if body := <-requests; len(body) != 1 || body["input"] != "first" {
		t.Fatalf("first request: %#v", body)
	}
	model, _ = model.Update(tea.PasteMsg{Content: "next"})
	model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, cmd = model.Update(receive(t, cmd))
	if body := <-requests; body["conv_id"] != "original" || body["input"] != "next" {
		t.Fatalf("resumed conversation not reused: %#v", body)
	}
	for step := 0; step < 2; step++ {
		model, cmd = model.Update(receive(t, cmd))
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "missing valid task_info") {
		t.Fatalf("new task inherited previous confirmation: %s", view)
	}
	model, _ = model.Update(tea.PasteMsg{Content: "blocked"})
	if _, submit := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); submit != nil {
		t.Fatal("unconfirmed new task allowed another submission")
	}
}

func TestLateSubscriptionMessagesCannotChangeCurrentConversation(t *testing.T) {
	var tasks atomic.Int32
	var previousConnections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"task_id":"task-%d"}`, tasks.Add(1))
			return
		}
		switch r.URL.Path {
		case "/tasks/previous/events":
			if previousConnections.Add(1) == 1 {
				fmt.Fprint(w, conversationFrame("different-owner", "other"))
				return
			}
			fmt.Fprint(w, conversationFrame("different-owner", "other"), event("task_completed", ""))
		case "/tasks/task-1/events":
			fmt.Fprint(w, conversationFrame("owner", "original"), event("task_completed", ""))
		default:
			fmt.Fprint(w, event("task_completed", ""))
		}
	}))
	defer server.Close()
	// Capture opaque messages produced by real subscriptions, including an old
	// owner that would conflict and an old owner that would falsely confirm.
	var stale []tea.Msg
	var previous tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{TaskID: "previous", ServerURL: server.URL}), 200, 18)
	defer func() { previous.(tui.Model).Close() }()
	previousCmd := previous.Init()
	for step := 0; step < 5; step++ {
		msg := receive(t, previousCmd)
		stale = append(stale, msg)
		previous, previousCmd = previous.Update(msg)
	}
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ServerURL: server.URL}), 200, 18)
	defer func() { model.(tui.Model).Close() }()
	model, _ = model.Update(tea.PasteMsg{Content: "first"})
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, cmd = model.Update(receive(t, cmd))
	for step := 0; step < 3; step++ {
		msg := receive(t, cmd)
		stale = append(stale, msg)
		model, cmd = model.Update(msg)
	}
	model, _ = model.Update(tea.PasteMsg{Content: "second"})
	model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	// A late completion must not interfere with a pending creation either.
	for _, msg := range stale {
		before := model.View().Content
		model, _ = model.Update(msg)
		if model.View().Content != before {
			t.Fatal("late subscription message changed pending creation")
		}
	}
	model, cmd = model.Update(receive(t, cmd))
	model, cmd = model.Update(receive(t, cmd)) // current subscription connected
	for _, msg := range stale {
		before := model.View().Content
		model, _ = model.Update(msg)
		if model.View().Content != before {
			t.Fatal("late subscription message changed current task")
		}
	}
	model, cmd = model.Update(receive(t, cmd)) // current task has no task_info
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Conversation original") || !strings.Contains(view, "missing valid task_info") || strings.Contains(view, "conversation conflict") {
		t.Fatalf("late ownership contaminated current task: %s", view)
	}
	for _, msg := range stale {
		model, _ = model.Update(msg)
	}
	model, _ = model.Update(tea.PasteMsg{Content: "draft"})
	if _, submit := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); submit != nil {
		t.Fatal("late completion unlocked unconfirmed task")
	}
}

func TestConversationRetryPreservesIdentityDraftAndSerialSubmission(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(fmt.Sprintf("bound=%t", bound), func(t *testing.T) {
			var posts atomic.Int32
			requests := make(chan map[string]string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					var body map[string]string
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					requests <- body
					number := posts.Add(1)
					failure := int32(1)
					if bound {
						failure = 2
					}
					if number == failure {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					w.WriteHeader(http.StatusCreated)
					fmt.Fprintf(w, `{"task_id":"task-%d"}`, number)
					return
				}
				if bound && r.URL.Path == "/tasks/task-1/events" {
					fmt.Fprint(w, conversationFrame("original", "original"), event("task_completed", ""))
					return
				}
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ServerURL: server.URL}), 200, 18)
			defer func() { model.(tui.Model).Close() }()
			if bound {
				model, _ = model.Update(tea.PasteMsg{Content: "initial"})
				var cmd tea.Cmd
				model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				for step := 0; step < 4; step++ {
					model, cmd = model.Update(receive(t, cmd))
				}
				<-requests
			}
			model, _ = model.Update(tea.PasteMsg{Content: "retry"})
			model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			model, _ = model.Update(tea.PasteMsg{Content: " tail"})
			if _, duplicate := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); duplicate != nil {
				t.Fatal("duplicate creation while request pending")
			}
			model, automatic := model.Update(receive(t, cmd))
			view := ansi.Strip(model.View().Content)
			if automatic != nil || !strings.Contains(view, "retry manually") || !strings.Contains(view, "retry tail") {
				t.Fatalf("failure lost manual retry or edited draft: %s", view)
			}
			failed := <-requests
			model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			model, _ = model.Update(tea.PasteMsg{Content: "next draft"})
			model, cmd = model.Update(receive(t, cmd))
			retried := <-requests
			if failed["input"] != "retry" || retried["input"] != "retry tail" {
				t.Fatalf("retry input changed: failed=%#v retried=%#v", failed, retried)
			}
			for _, request := range []map[string]string{failed, retried} {
				id, present := request["conv_id"]
				if bound && id != "original" || !bound && present {
					t.Fatalf("retry changed conversation identity: %#v", request)
				}
			}
			model, cmd = model.Update(receive(t, cmd)) // no ownership or completion follows
			view = ansi.Strip(model.View().Content)
			if !strings.Contains(view, "next draft") || !strings.Contains(view, "completion unconfirmed") || strings.Contains(view, "Completed") {
				t.Fatalf("enqueue overwrote draft or claimed completion: %s", view)
			}
			for repeat := 0; repeat < 3; repeat++ {
				if _, duplicate := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); duplicate != nil {
					t.Fatal("running task allowed duplicate submission")
				}
			}
			if len(requests) != 0 {
				t.Fatal("unexpected automatic request")
			}
		})
	}
}
