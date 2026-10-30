package tui_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func TestHistoryDuringSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, event("answer_delta", "zero\none\ntwo\nthree\nfour\nfive\nsix\nseven"), event("answer_delta", "eight\nnine"), event("task_completed", ""))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	attached := tui.NewWithTask(".", tui.TaskOptions{TaskID: "history", ServerURL: server.URL})
	defer attached.Close()
	var model tea.Model = resize(t, attached, 30, 10)
	cmd := model.Init()
	model, cmd = model.Update(receive(t, cmd)) // connected
	model, cmd = model.Update(receive(t, cmd)) // first response
	next := cmd
	model, _ = model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	first := strings.Split(ansi.Strip(model.View().Content), "\n")[1]
	model, cmd = model.Update(receive(t, next))
	if row := strings.Split(ansi.Strip(model.View().Content), "\n")[1]; row != first {
		t.Fatal("stream moved history")
	}
	next = cmd
	model, _ = model.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 15, Y: 5})
	if !strings.Contains(ansi.Strip(model.View().Content), "nine") {
		t.Fatal("return did not reveal latest output")
	}
	model, cmd = model.Update(receive(t, next))
	if cmd != nil {
		t.Fatal("completion kept reading")
	}
}
