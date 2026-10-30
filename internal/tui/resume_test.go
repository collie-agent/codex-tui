package tui_test

import (
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

func TestResumeAcknowledgedEventsAndDeduplicate(t *testing.T) {
	first := event("answer_delta", "same")
	second := event("answer_delta", "same")
	completion := event("task_completed", "")
	var calls atomic.Int32
	headers := make(chan string, 4)
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Get("Last-Event-ID")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, first)
			return
		}
		fmt.Fprint(w, first, second, completion)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	defer server.Close()
	attached := tui.NewWithTask("/work", tui.TaskOptions{TaskID: "resume", ServerURL: server.URL})
	defer attached.Close()
	var model tea.Model = resize(t, attached, 100, 12)
	cmd := model.Init()
	sawError := false
	for i := 0; i < 6; i++ {
		model, cmd = model.Update(receive(t, cmd))
		sawError = sawError || strings.Contains(ansi.Strip(model.View().Content), "Connection error")
	}
	view := ansi.Strip(model.View().Content)
	if !sawError || cmd != nil || !strings.Contains(view, "Completed") || !strings.Contains(view, "samesame") || strings.Contains(view, "samesamesame") {
		t.Fatalf("incorrect resumed view: %s", view)
	}
	if h := <-headers; h != "" {
		t.Fatalf("initial cursor %q", h)
	}
	h := <-headers
	if h == "" || !strings.Contains(first, `"ID":"`+h+`"`) {
		t.Fatalf("incorrect resume cursor %q for %s", h, first)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("completion did not release connection")
	}
	if calls.Load() != 2 {
		t.Fatalf("unexpected requests: %d", calls.Load())
	}
}

func TestExitCancelsPendingRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "unavailable", 503) }))
	defer server.Close()
	attached := tui.NewWithTask("/work", tui.TaskOptions{TaskID: "retry", ServerURL: server.URL})
	defer attached.Close()
	var model tea.Model = resize(t, attached, 100, 12)
	model, cmd := model.Update(receive(t, model.Init()))
	if cmd == nil || !strings.Contains(ansi.Strip(model.View().Content), "Connection error") {
		t.Fatal("missing retry status")
	}
	_, quit := model.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("did not quit")
	}
	if msg := receive(t, cmd); msg != nil {
		t.Fatalf("late retry message: %v", msg)
	}
	time.Sleep(250 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("retried after exit: %d", calls.Load())
	}
}
