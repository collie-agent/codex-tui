package tui_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func TestLineKillsPreserveExplicitBreaksAndSubmitEditedDraft(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tasks" {
			t.Errorf("request = %s %s, want POST /tasks", r.Method, r.URL.Path)
		}
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- body.Input
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task_id":"line-edited"}`))
	}))
	defer server.Close()

	ctrl := func(key rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: key, Mod: tea.ModCtrl} }
	model := resize(t, tui.NewWithTask("/workspace", tui.TaskOptions{ServerURL: server.URL}), 40, 14)
	model = edit(t, model, tea.PasteMsg{Content: "上👩‍💻"})
	checkUnchanged := func(key rune) {
		t.Helper()
		before := model.View()
		model = edit(t, model, ctrl(key))
		after := model.View()
		if before.Content != after.Content || after.Cursor == nil || after.Cursor.Position != before.Cursor.Position {
			t.Fatalf("Ctrl+%c at a line boundary changed the draft or caret", key)
		}
	}
	model = edit(t, model, ctrl('j'))
	model = edit(t, model, ctrl('j'))
	model = edit(t, model, tea.PasteMsg{Content: "下e\u0301X"})
	model = edit(t, model, ctrl('a'))
	checkUnchanged('u')
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	checkUnchanged('u')
	checkUnchanged('k')
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	model = edit(t, model, ctrl('e'))
	checkUnchanged('k')
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyEnd})
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	model = edit(t, model, ctrl('k'))
	model = edit(t, model, tea.KeyPressMsg{Code: '!', Text: "!"})

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, submit := model.Update(enter)
	if submit == nil {
		t.Fatal("Enter did not start a task")
	}
	edit(t, model, enter)
	submit()
	select {
	case got := <-requests:
		if want := "上👩‍💻\n\n下e\u0301!"; got != want {
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

func TestCtrlUDeletesOnlyPrefixOfCurrentWrappedLine(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 14, 12)
	model = edit(t, model, tea.PasteMsg{Content: "abcdefg中e\u0301👩‍💻CD尾F"})
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	// Column three is between the combining sequence and the emoji.
	model = edit(t, model, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if row, cursor := inputLine(t, model), model.View().Cursor; row != "  👩‍💻CD尾F" || cursor.X != 4 || cursor.Y != 9 {
		t.Fatalf("Ctrl+U left the caret at %+v on %q, want (4, 9) on the second of two rows", cursor, row)
	}
	model = resize(t, model, 40, 12)
	if row, cursor := inputLine(t, model), model.View().Cursor; row != "› abcdefg👩‍💻CD尾F" || cursor.X != 11 {
		t.Fatalf("widening left the caret at %+v on %q", cursor, row)
	}
}

func TestCtrlKDeletesOnlySuffixOfCurrentWrappedLine(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 14, 12)
	model = edit(t, model, tea.PasteMsg{Content: "abcdefg中e\u0301👩‍💻CD尾F"})
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	model = edit(t, model, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if row, cursor := inputLine(t, model), model.View().Cursor; row != "  中e\u0301尾F" || cursor.X != 7 || cursor.Y != 9 {
		t.Fatalf("Ctrl+K left the caret at %+v on %q, want (7, 9) on the second of two rows", cursor, row)
	}
	model = resize(t, model, 40, 12)
	if row, cursor := inputLine(t, model), model.View().Cursor; row != "› abcdefg中e\u0301尾F" || cursor.X != 14 {
		t.Fatalf("widening left the caret at %+v on %q", cursor, row)
	}
}

func TestCtrlUThenCtrlKClearsOriginalWrappedLine(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 14, 12)
	model = edit(t, model, tea.PasteMsg{Content: "abcdefgABCDEFG1234567"})
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	for range 3 {
		model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	model = edit(t, model, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if row := inputLine(t, model); row != "  EFG1234" {
		t.Fatalf("row after Ctrl+U = %q, want the remaining text to reflow", row)
	}
	model = edit(t, model, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	model = resize(t, model, 40, 12)
	if row, cursor := inputLine(t, model), model.View().Cursor; row != "› abcdefg1234567" || cursor.X != 11 {
		t.Fatalf("Ctrl+U then Ctrl+K left the caret at %+v on %q", cursor, row)
	}
}

func TestLineKillsRespectBothSidesOfSoftWrap(t *testing.T) {
	for _, tt := range []struct {
		name          string
		edge, noop    rune
		kill          rune
		before, after string
		beforeX       int
	}{
		{"end of middle row", 'e', 'k', 'u', "  ABCDEFG", "  1234567", 11},
		{"start of middle row", 'a', 'u', 'k', "  ABCDEFG", "  1234567", 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := func(key rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: key, Mod: tea.ModCtrl} }
			model := resize(t, tui.New("/workspace"), 14, 12)
			model = edit(t, model, tea.PasteMsg{Content: "abcdefgABCDEFG1234567"})
			model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
			model = edit(t, model, ctrl(tt.edge))
			before := model.View().Cursor
			model = edit(t, model, ctrl(tt.noop))
			if row, cursor := inputLine(t, model), model.View().Cursor; row != tt.before || cursor.X != tt.beforeX || cursor.Y != before.Y {
				t.Fatalf("Ctrl+%c moved the caret from %+v to %+v on %q", tt.noop, before, cursor, row)
			}
			model = edit(t, model, ctrl(tt.kill))
			if row, cursor := inputLine(t, model), model.View().Cursor; row != tt.after || cursor.X != 4 || cursor.Y != 9 {
				t.Fatalf("Ctrl+%c left the caret at %+v on %q", tt.kill, cursor, row)
			}
			model = resize(t, model, 40, 12)
			if got := inputLine(t, model); got != "› abcdefg1234567" {
				t.Fatalf("remaining draft = %q", got)
			}
		})
	}
}
