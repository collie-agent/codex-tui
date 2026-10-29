package tui_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

var eventSequence atomic.Uint64

func event(kind, text string) string {
	delta, _ := json.Marshal(map[string]string{"text": text})
	payload, _ := json.Marshal(map[string]string{"ID": fmt.Sprint(eventSequence.Add(1)), "Type": kind, "Delta": string(delta)})
	return "data: " + string(payload) + "\r\n\r\n"
}
func receive(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing subscription command")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		return msg
	case <-time.After(3 * time.Second):
		t.Fatal("subscription blocked")
		return nil
	}
}
func TestAttachReplayAndCompletion(t *testing.T) {
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks/user-task/events" || r.Header.Get("Accept") != "text/event-stream" {
			t.Error("incorrect subscription request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{event("text_delta", "你好 "), event("text_delta", "world"), event("text", "你好 world"), event("text", "你好 world"), event("usage", ""), event("task_completed", "")}
		frames[0] = ": heartbeat\r\n" + strings.Replace(frames[0], `,"ID"`, ",\r\ndata: "+`"ID"`, 1)
		frames[1] = strings.ReplaceAll(frames[1], "\r\n", "\r")
		for _, frame := range frames {
			for _, part := range []string{frame[:7], frame[7:]} {
				fmt.Fprint(w, part)
				w.(http.Flusher).Flush()
			}
		}
		<-r.Context().Done()
		close(closed)
	}))
	defer server.Close()
	attached := tui.NewWithTask("/work", tui.TaskOptions{TaskID: "user-task", ServerURL: server.URL})
	defer attached.Close()
	var model tea.Model = resize(t, attached, 100, 12)
	cmd := model.Init()
	for i := 0; i < 7; i++ {
		model, cmd = model.Update(receive(t, cmd))
		if i == 1 && !strings.Contains(ansi.Strip(model.View().Content), "你好") {
			t.Fatal("delta was not rendered before full text")
		}
	}
	if cmd != nil {
		t.Fatal("completed subscription kept reading")
	}
	view := ansi.Strip(model.View().Content)
	if strings.Count(view, "你好 world") != 2 || !strings.Contains(view, "Completed") || !strings.Contains(view, "user-task") {
		t.Fatalf("incorrect replay: %s", view)
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("completion did not close connection")
	}
	for _, size := range [][2]int{{1, 1}, {2, 8}, {0, 0}, {20, 9}} {
		model = resize(t, model, size[0], size[1])
	}
}
func TestAttachDisconnectAndExit(t *testing.T) {
	for _, quit := range []bool{false, true} {
		t.Run(fmt.Sprint(quit), func(t *testing.T) {
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, event("text", "safe\x1b[2J\n下一行"), event("usage", ""), event("tool_error", ""))
				w.(http.Flusher).Flush()
				if quit {
					<-r.Context().Done()
				}
				close(closed)
			}))
			defer server.Close()
			attached := tui.NewWithTask("/work", tui.TaskOptions{TaskID: "task", ServerURL: server.URL})
			defer attached.Close()
			var model tea.Model = resize(t, attached, 100, 12)
			cmd := model.Init()
			for i := 0; i < 4; i++ {
				model, cmd = model.Update(receive(t, cmd))
			}
			model, _ = model.Update(tea.KeyPressMsg{Text: "draft", Code: 'd'})
			model, enter := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if enter != nil {
				t.Fatal("enter submitted task")
			}
			if quit {
				model, cmd = model.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("did not quit")
				}
			} else {
				model, cmd = model.Update(receive(t, cmd))
				if cmd == nil {
					t.Fatal("missing reconnect")
				}
			}
			view := ansi.Strip(model.View().Content)
			if strings.Contains(view, "Completed") || !strings.Contains(view, `safe\x1b[2J`) || !strings.Contains(view, "draft") {
				t.Fatalf("incorrect state: %s", view)
			}
			if !quit && !strings.Contains(view, "Connection error") {
				t.Fatal("missing connection error")
			}
			select {
			case <-closed:
			case <-time.After(3 * time.Second):
				t.Fatal("exit did not close connection")
			}
		})
	}
}
