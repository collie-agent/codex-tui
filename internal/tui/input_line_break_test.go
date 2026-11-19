package tui_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func TestCtrlJInsertsLineAndBlankLineWithoutSubmitting(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 40, 14)
	model = edit(t, model, tea.PasteMsg{Content: "你好世界"})
	for range 2 {
		model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	lineBreak := tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}
	model = edit(t, model, lineBreak)
	view := model.View()
	if got := inputLine(t, model); strings.TrimSpace(got) != "世界" || view.Cursor.X != 4 || view.Cursor.Y < 1 {
		t.Fatalf("after Ctrl+J, cursor = %+v on row %q, want column 4 before 世界", view.Cursor, got)
	}
	rows := strings.Split(ansi.Strip(view.Content), "\n")
	if got := strings.TrimSpace(rows[view.Cursor.Y-1]); got != "› 你好" {
		t.Fatalf("row above the cursor = %q, want › 你好", got)
	}

	model = edit(t, model, lineBreak)
	view = model.View()
	rows = strings.Split(ansi.Strip(view.Content), "\n")
	if got := inputLine(t, model); strings.TrimSpace(got) != "世界" || view.Cursor.X != 4 || view.Cursor.Y < 2 {
		t.Fatalf("after the second Ctrl+J, cursor = %+v on row %q, want column 4 before 世界", view.Cursor, got)
	}
	if got := strings.TrimSpace(rows[view.Cursor.Y-1]); got != "" {
		t.Fatalf("row above the cursor = %q, want a blank row", got)
	}
	if got := strings.TrimSpace(rows[view.Cursor.Y-2]); got != "› 你好" {
		t.Fatalf("row above the blank row = %q, want › 你好", got)
	}
}

func TestEnterSubmitsOnlyExplicitLineBreaksInWrappedDraft(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tasks" {
			t.Errorf("request = %s %s, want POST /tasks", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		requests <- body.Input
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task_id":"multiline"}`))
	}))
	defer server.Close()

	model := resize(t, tui.NewWithTask("/workspace", tui.TaskOptions{ServerURL: server.URL}), 14, 14)
	model = edit(t, model, tea.PasteMsg{Content: "abcdefg你好世界"})
	for range 2 {
		model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	lineBreak := tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}
	model = edit(t, model, lineBreak)
	model = edit(t, model, lineBreak)
	view := model.View()
	if got := inputLine(t, model); strings.TrimSpace(got) != "世界" || view.Cursor.Y < 3 {
		t.Fatalf("cursor = %+v on row %q, want 世界 after three input rows", view.Cursor, got)
	}
	rows := strings.Split(ansi.Strip(view.Content), "\n")
	if got := strings.TrimSpace(rows[view.Cursor.Y-3]); got != "› abcdefg" {
		t.Fatalf("first wrapped row = %q, want › abcdefg", got)
	}
	if got := strings.TrimSpace(rows[view.Cursor.Y-2]); got != "你好" {
		t.Fatalf("partial wrapped row = %q, want 你好", got)
	}
	select {
	case got := <-requests:
		t.Fatalf("editing submitted %q before Enter", got)
	default:
	}

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, submit := model.Update(enter)
	if submit == nil {
		t.Fatal("Enter did not start a task")
	}
	edit(t, model, enter)
	submit()
	select {
	case got := <-requests:
		if want := "abcdefg你好\n\n世界"; got != want {
			t.Fatalf("submitted draft = %q, want %q", got, want)
		}
	default:
		t.Fatal("Enter sent no task request")
	}
	select {
	case got := <-requests:
		t.Fatalf("unexpected second task request with draft %q", got)
	default:
	}
}

func TestMultilineTaskCardPreservesYouPrefixInDraft(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 40, 24)
	model = edit(t, model, tea.PasteMsg{Content: "first"})
	model = edit(t, model, tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	model = edit(t, model, tea.PasteMsg{Content: "You: second"})
	model, submit := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if submit == nil {
		t.Fatal("Enter did not submit multiline draft")
	}
	content := ansi.Strip(model.View().Content)
	if strings.Count(content, "first") != 1 || strings.Count(content, "You: second") != 1 {
		t.Fatalf("expected both draft lines exactly once in task card: %q", content)
	}
}
