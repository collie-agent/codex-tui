package tui_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func TestInterruptAndContinueConversation(t *testing.T) {
	terminal := make(chan struct{})
	requests := make(chan map[string]string, 2)
	interrupt := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tasks" && r.Method == http.MethodPost:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			requests <- body
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"task_id":"task"}`)
		case strings.HasPrefix(r.URL.Path, "/runs/"):
			b, _ := io.ReadAll(r.Body)
			interrupt <- r.Method + " " + r.URL.Path + " " + string(b)
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"status":"accepted"}`)
		default:
			fmt.Fprint(w, `data: {"ID":"info","Type":"task_info","Delta":"{\"run_id\":\"run-1\",\"conv_id\":\"conv-1\"}"}`+"\n\n", event("answer_delta", "existing output"))
			w.(http.Flusher).Flush()
			select {
			case <-terminal:
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, `data: {"ID":"end","Type":"task_interrupted","Delta":"{}"}`+"\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ServerURL: server.URL}), 160, 14)
	defer func() { model.(tui.Model).Close() }()
	model, _ = model.Update(tea.PasteMsg{Content: "first"})
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, cmd = model.Update(receive(t, cmd))
	<-requests
	for i := 0; i < 3; i++ {
		model, cmd = model.Update(receive(t, cmd))
	}
	model, _ = model.Update(tea.PasteMsg{Content: "next draft"})
	model, interruptCmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model, duplicate := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if duplicate != nil {
		t.Fatal("duplicate interrupt")
	}
	model, _ = model.Update(receive(t, interruptCmd))
	if got := <-interrupt; got != "POST /runs/run-1/interrupt " {
		t.Fatalf("request: %q", got)
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "waiting") || strings.Contains(view, "Interrupted") {
		t.Fatalf("accepted status: %s", view)
	}
	model, enter := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if enter != nil {
		t.Fatal("accepted request unlocked submission")
	}
	close(terminal)
	model, cmd = model.Update(receive(t, cmd))
	if cmd != nil {
		t.Fatal("subscription still open")
	}
	view := ansi.Strip(model.View().Content)
	for _, s := range []string{"Interrupted", "existing output", "next draft"} {
		if !strings.Contains(view, s) {
			t.Fatalf("missing %q: %s", s, view)
		}
	}
	model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, _ = model.Update(receive(t, cmd))
	body := <-requests
	if len(body) != 2 || body["input"] != "next draft" || body["conv_id"] != "conv-1" {
		t.Fatalf("continuation: %#v", body)
	}
}

func TestUnrequestedInterruptionRequiresCurrentConversationConfirmation(t *testing.T) {
	for _, tc := range []struct{ name, info, reason string }{
		{"missing", "", "missing valid task_info"},
		{"conflict", `data: {"ID":"info","Type":"task_info","Delta":"{\"conv_id\":\"other\"}"}` + "\n\n", "conversation conflict"},
		{"confirmed", `data: {"ID":"info","Type":"task_info","Delta":"{\"conv_id\":\"original\"}"}` + "\n\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tc.info, `data: {"ID":"end","Type":"task_interrupted","Delta":"{}"}`+"\n\n")
			}))
			defer server.Close()
			var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{TaskID: "task", ServerURL: server.URL, ConversationID: "original"}), 160, 14)
			defer func() { model.(tui.Model).Close() }()
			cmd := model.Init()
			count := 2
			if tc.info != "" {
				count++
			}
			for i := 0; i < count; i++ {
				model, cmd = model.Update(receive(t, cmd))
			}
			view := ansi.Strip(model.View().Content)
			if !strings.Contains(view, "Interrupted") || (tc.reason != "" && !strings.Contains(view, tc.reason)) {
				t.Fatalf("status: %s", view)
			}
			model, _ = model.Update(tea.PasteMsg{Content: "draft"})
			model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if (cmd != nil) != (tc.reason == "") {
				t.Fatalf("unexpected submission permission: %s", view)
			}
		})
	}
}

func TestDeferredInterruptionAfterRunIdentity(t *testing.T) {
	info := make(chan struct{})
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/runs/") {
			requests <- r.URL.Path
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"status":"accepted"}`)
			return
		}
		w.(http.Flusher).Flush()
		select {
		case <-info:
		case <-r.Context().Done():
			return
		}
		fmt.Fprint(w, `data: {"ID":"info","Type":"task_info","Delta":"{\"run_id\":\"run-1\",\"conv_id\":\"conv-1\"}"}`+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{TaskID: "task", ServerURL: server.URL}), 160, 14)
	defer func() { model.(tui.Model).Close() }()
	cmd := model.Init()
	model, cmd = model.Update(receive(t, cmd))
	model, esc := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if esc != nil || !strings.Contains(ansi.Strip(model.View().Content), "Waiting for run") {
		t.Fatalf("deferred status: %s", ansi.Strip(model.View().Content))
	}
	close(info)
	model, cmd = model.Update(receive(t, cmd))
	batch, ok := receive(t, cmd).(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("expected stream and interrupt commands")
	}
	model, _ = model.Update(receive(t, batch[0]))
	if got := <-requests; got != "/runs/run-1/interrupt" {
		t.Fatalf("request: %s", got)
	}
}

func TestInterruptionResponseAndManualRetry(t *testing.T) {
	for _, tc := range []struct {
		name       string
		code       int
		body, want string
		retry      bool
	}{
		{"accepted", 202, `{"status":"accepted"}`, "accepted", false},
		{"already requested", 409, `{"status":"already_requested"}`, "accepted", false},
		{"terminal", 409, `{"status":"terminal"}`, "Run ended", false},
		{"missing", 404, `{"error":"unknown run"}`, "unknown run", true},
		{"unavailable", 503, `{"detail":"unavailable"}`, "unavailable", true},
		{"other", 500, `{"message":"failure"}`, "failure", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 3)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/runs/") {
					requests <- r.URL.Path
					w.WriteHeader(tc.code)
					fmt.Fprint(w, tc.body)
					return
				}
				fmt.Fprint(w, `data: {"ID":"info","Type":"task_info","Delta":"{\"run_id\":\"run-1\",\"conv_id\":\"conv-1\"}"}`+"\n\n")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{TaskID: "task", ServerURL: server.URL}), 160, 14)
			defer func() { model.(tui.Model).Close() }()
			cmd := model.Init()
			for i := 0; i < 2; i++ {
				model, cmd = model.Update(receive(t, cmd))
			}
			model, _ = model.Update(tea.PasteMsg{Content: "draft"})
			model, request := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			model, _ = model.Update(receive(t, request))
			if got := <-requests; got != "/runs/run-1/interrupt" {
				t.Fatal(got)
			}
			view := ansi.Strip(model.View().Content)
			if !strings.Contains(view, tc.want) || !strings.Contains(view, "draft") || strings.Contains(view, "Interrupted") {
				t.Fatal(view)
			}
			model, enter := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if enter != nil {
				t.Fatal("submission unlocked")
			}
			model, retry := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			if (retry != nil) != tc.retry {
				t.Fatalf("retry = %v", retry != nil)
			}
			if retry != nil {
				model, duplicate := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
				_ = model
				if duplicate != nil {
					t.Fatal("concurrent retry")
				}
				_ = receive(t, retry)
				if got := <-requests; got != "/runs/run-1/interrupt" {
					t.Fatal(got)
				}
			}
		})
	}
}

func TestDeferredInterruptionDoesNotSurviveFailedCreation(t *testing.T) {
	var count int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tasks" {
			count++
			if count == 1 {
				w.WriteHeader(503)
				return
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"task_id":"task"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/runs/") {
			t.Error("failed task interruption leaked into retry")
			w.WriteHeader(202)
			return
		}
		fmt.Fprint(w, `data: {"ID":"info","Type":"task_info","Delta":"{\"run_id\":\"run-2\",\"conv_id\":\"conv\"}"}`+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{ServerURL: server.URL}), 160, 14)
	defer func() { model.(tui.Model).Close() }()
	model, _ = model.Update(tea.PasteMsg{Content: "first"})
	model, create := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, esc := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if esc != nil || !strings.Contains(ansi.Strip(model.View().Content), "Waiting for run") {
		t.Fatal("missing deferred status")
	}
	model, _ = model.Update(receive(t, create))
	if !strings.Contains(ansi.Strip(model.View().Content), "first") {
		t.Fatal("missing restored input")
	}
	model, create = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model, stream := model.Update(receive(t, create))
	for i := 0; i < 2; i++ {
		model, stream = model.Update(receive(t, stream))
	}
	if _, ok := receiveBatchWithoutRunning(stream); ok {
		t.Fatal("unexpected deferred interrupt")
	}
}

func receiveBatchWithoutRunning(cmd tea.Cmd) (tea.BatchMsg, bool) {
	// A batch command returns immediately; a subscription command waits for SSE.
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		batch, ok := msg.(tea.BatchMsg)
		return batch, ok
	case <-time.After(50 * time.Millisecond):
		return nil, false
	}
}

func TestInterruptionWaitingSurvivesReconnectAndReplay(t *testing.T) {
	resume := make(chan string, 1)
	var connections int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/runs/") {
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprint(w, `{"status":"accepted"}`)
			return
		}
		connections++
		if connections == 1 {
			fmt.Fprint(w, `data: {"ID":"info","Type":"task_info","Delta":"{\"run_id\":\"run\",\"conv_id\":\"conv\"}"}`+"\n\n")
			w.(http.Flusher).Flush()
			<-resume
			return
		}
		resume <- r.Header.Get("Last-Event-ID")
		fmt.Fprint(w, `data: {"ID":"info","Type":"task_info","Delta":"{\"run_id\":\"run\",\"conv_id\":\"conv\"}"}`+"\n\n", `data: {"ID":"end","Type":"task_interrupted","Delta":"{}"}`+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{TaskID: "task", ServerURL: server.URL}), 160, 14)
	defer func() { model.(tui.Model).Close() }()
	cmd := model.Init()
	for i := 0; i < 2; i++ {
		model, cmd = model.Update(receive(t, cmd))
	}
	model, request := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model, _ = model.Update(receive(t, request))
	resume <- "disconnect"
	model, cmd = model.Update(receive(t, cmd))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "waiting for terminal confirmation") {
		t.Fatalf("waiting state lost on disconnect: %s", view)
	}
	model, cmd = model.Update(receive(t, cmd))
	if got := <-resume; got != "info" {
		t.Fatalf("resume cursor: %q", got)
	}
	model, cmd = model.Update(receive(t, cmd))
	if cmd != nil || !strings.Contains(ansi.Strip(model.View().Content), "Interrupted") {
		t.Fatal("replayed terminal did not complete interruption")
	}
}

func TestTerminalWinsOverLateInterruptionResponse(t *testing.T) {
	for _, terminal := range []string{"task_completed", "task_interrupted"} {
		for _, code := range []int{http.StatusAccepted, http.StatusServiceUnavailable} {
			for _, stage := range []string{"terminal", "creating", "running"} {
				t.Run(fmt.Sprintf("%s/%d/%s", terminal, code, stage), func(t *testing.T) {
					sendTerminal := make(chan struct{})
					requests := make(chan map[string]string, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if strings.HasPrefix(r.URL.Path, "/runs/") {
							w.WriteHeader(code)
							if code == http.StatusAccepted {
								fmt.Fprint(w, `{"status":"accepted"}`)
							} else {
								fmt.Fprint(w, `{"error":"unavailable"}`)
							}
							return
						}
						if r.URL.Path == "/tasks" {
							var body map[string]string
							_ = json.NewDecoder(r.Body).Decode(&body)
							requests <- body
							w.WriteHeader(http.StatusCreated)
							fmt.Fprint(w, `{"task_id":"next"}`)
							return
						}
						run := "run"
						if r.URL.Path == "/tasks/next/events" {
							run = "next-run"
						}
						fmt.Fprintf(w, "data: {\"ID\":\"info\",\"Type\":\"task_info\",\"Delta\":%q}\n\n", fmt.Sprintf(`{"run_id":%q,"conv_id":"conv"}`, run))
						w.(http.Flusher).Flush()
						if run == "next-run" {
							<-r.Context().Done()
							return
						}
						select {
						case <-sendTerminal:
						case <-r.Context().Done():
							return
						}
						fmt.Fprintf(w, "data: {\"ID\":\"end\",\"Type\":%q,\"Delta\":\"{}\"}\n\n", terminal)
						w.(http.Flusher).Flush()
						<-r.Context().Done()
					}))
					defer server.Close()
					var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{TaskID: "task", ServerURL: server.URL}), 400, 14)
					defer func() { model.(tui.Model).Close() }()
					cmd := model.Init()
					connected := receive(t, cmd)
					model, cmd = model.Update(connected)
					info := receive(t, cmd)
					model, cmd = model.Update(info)
					model, request := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
					// Capture an actual HTTP result before cancellation, but delay its UI delivery.
					response := receive(t, request)
					close(sendTerminal)
					end := receive(t, cmd)
					model, _ = model.Update(end)
					want := "Completed"
					if terminal == "task_interrupted" {
						want = "Interrupted"
					}
					if !strings.Contains(ansi.Strip(model.View().Content), want) {
						t.Fatal("missing terminal status")
					}
					model, _ = model.Update(tea.PasteMsg{Content: "continue"})
					var create tea.Cmd
					if stage != "terminal" {
						model, create = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
						if create == nil {
							t.Fatal("terminal did not permit continuation")
						}
					}
					if stage == "running" {
						model, cmd = model.Update(receive(t, create))
						body := <-requests
						if body["conv_id"] != "conv" || body["input"] != "continue" || len(body) != 2 {
							t.Fatalf("continuation: %#v", body)
						}
						for i := 0; i < 2; i++ {
							model, cmd = model.Update(receive(t, cmd))
						}
					}
					before := ansi.Strip(model.View().Content)
					for _, late := range []tea.Msg{response, connected, info, end} {
						model, _ = model.Update(late)
						if got := ansi.Strip(model.View().Content); got != before {
							t.Fatalf("late message changed %s view: %s", stage, got)
						}
					}
					if stage == "creating" {
						model, cmd = model.Update(receive(t, create))
						<-requests
						for i := 0; i < 2; i++ {
							model, cmd = model.Update(receive(t, cmd))
						}
					}
					model, enter := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
					if (enter != nil) != (stage == "terminal") {
						t.Fatal("late message changed submission permission")
					}
				})
			}
		}
	}
}

func TestInterruptionStatusSurvivesReconnectAndConversationConflict(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		for _, code := range []int{http.StatusAccepted, http.StatusServiceUnavailable} {
			t.Run(fmt.Sprintf("conflict=%t/%d", conflict, code), func(t *testing.T) {
				disconnect := make(chan struct{})
				connections := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasPrefix(r.URL.Path, "/runs/") {
						w.WriteHeader(code)
						if code == http.StatusAccepted {
							fmt.Fprint(w, `{"status":"accepted"}`)
						} else {
							fmt.Fprint(w, `{"error":"executor unavailable"}`)
						}
						return
					}
					connections++
					if connections == 1 {
						conv := "conv"
						if conflict {
							conv = "other"
						}
						fmt.Fprintf(w, "data: {\"ID\":\"info\",\"Type\":\"task_info\",\"Delta\":%q}\n\n", fmt.Sprintf(`{"run_id":"run","conv_id":%q}`, conv))
						w.(http.Flusher).Flush()
						select {
						case <-disconnect:
						case <-r.Context().Done():
						}
						return
					}
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				}))
				defer server.Close()
				var model tea.Model = resize(t, tui.NewWithTask("", tui.TaskOptions{TaskID: "task", ConversationID: "conv", ServerURL: server.URL}), 400, 14)
				defer func() { model.(tui.Model).Close() }()
				cmd := model.Init()
				for i := 0; i < 2; i++ {
					model, cmd = model.Update(receive(t, cmd))
				}
				model, _ = model.Update(tea.PasteMsg{Content: "draft"})
				model, request := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
				view := ansi.Strip(model.View().Content)
				if !strings.Contains(view, "Requesting interruption") || (conflict && !strings.Contains(view, "conversation conflict")) {
					t.Fatalf("request state: %s", view)
				}
				model, _ = model.Update(receive(t, request))
				check := func() {
					t.Helper()
					view := ansi.Strip(model.View().Content)
					want := "waiting for terminal confirmation"
					if code == http.StatusServiceUnavailable {
						want = "executor unavailable"
					}
					if !strings.Contains(view, want) || !strings.Contains(view, "draft") || (conflict && !strings.Contains(view, "conversation conflict")) {
						t.Fatalf("lost status: %s", view)
					}
				}
				check()
				close(disconnect)
				for i := 0; i < 2; i++ {
					model, cmd = model.Update(receive(t, cmd))
					check()
				}
				model, enter := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if enter != nil {
					t.Fatal("reconnect unlocked submission")
				}
				model, retry := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
				if (retry != nil) != (code == http.StatusServiceUnavailable) {
					t.Fatal("incorrect retry permission")
				}
				if retry != nil {
					model, _ = model.Update(receive(t, retry))
					check()
				}
			})
		}
	}
}
