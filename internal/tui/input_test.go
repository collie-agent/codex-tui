package tui_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func edit(t *testing.T, model tea.Model, msg tea.Msg) tea.Model {
	t.Helper()
	next, cmd := model.Update(msg)
	if cmd != nil {
		t.Fatal("editing produced an unexpected external effect")
	}
	return next
}

func inputLine(t *testing.T, model tea.Model) string {
	t.Helper()
	view := model.View()
	lines := strings.Split(ansi.Strip(view.Content), "\n")
	if view.Cursor == nil || view.Cursor.Y < 0 || view.Cursor.Y >= len(lines) {
		t.Fatal("missing or invalid input caret")
	}
	line := lines[view.Cursor.Y]
	if view.Cursor.X < 0 || view.Cursor.X >= ansi.StringWidth(line) {
		t.Fatalf("caret outside input row: %+v", view.Cursor)
	}
	if !utf8.ValidString(line) {
		t.Fatal("input contains broken UTF-8")
	}
	return strings.TrimRight(strings.TrimPrefix(line, "  "), " ")
}

func TestInputEditingAndCopyWithoutSelection(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 100, 24)
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := inputLine(t, model); got != "› Ask Codex to do anything" {
		t.Fatalf("empty input = %q", got)
	}
	for _, r := range "Hello 世界" {
		model = edit(t, model, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := inputLine(t, model); got != "› Hello 世界" {
		t.Fatalf("typed input = %q", got)
	}
	before := model.View().Content
	for _, key := range []tea.KeyPressMsg{{Code: 'c', Mod: tea.ModCtrl}} {
		model = edit(t, model, key)
		if model.View().Content != before {
			t.Fatal("Ctrl+C changed input or produced a reply")
		}
	}
	for _, want := range []string{"Hello 世", "Hello ", "Hello", "Hell", "Hel", "He", "H", "Ask Codex to do anything"} {
		model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
		if got := inputLine(t, model); got != strings.TrimRight("› "+want, " ") {
			t.Fatalf("after backspace = %q, want %q", got, want)
		}
	}
	_, cmd := model.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+D must still quit after editing")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+D did not request exit")
	}
}

func TestDeletesWholeGraphemes(t *testing.T) {
	for _, character := range []string{"中", "界", "🙂", "e\u0301", "👩‍💻", "🇨🇳", "क्ष"} {
		t.Run(character, func(t *testing.T) {
			model := resize(t, tui.New("/workspace"), 40, 10)
			for _, r := range "A" + character {
				model = edit(t, model, tea.KeyPressMsg{Code: r, Text: string(r)})
			}
			if got := inputLine(t, model); got != "› A"+character {
				t.Fatalf("Unicode input = %q", got)
			}
			model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
			if got := inputLine(t, model); got != "› A" {
				t.Fatalf("backspace left part of a character: %q", got)
			}
		})
	}
}

func TestCaretEditing(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 40, 10)
	for _, msg := range []tea.Msg{
		tea.PasteMsg{Content: "A中👩‍💻B"},
		tea.KeyPressMsg{Code: tea.KeyLeft},
		tea.KeyPressMsg{Code: tea.KeyLeft},
		tea.KeyPressMsg{Code: tea.KeyDelete},
		tea.KeyPressMsg{Code: 'x', Text: "x"},
		tea.KeyPressMsg{Code: tea.KeyHome},
		tea.KeyPressMsg{Code: tea.KeyRight},
		tea.KeyPressMsg{Code: tea.KeyDelete},
		tea.KeyPressMsg{Code: tea.KeyEnd},
		tea.KeyPressMsg{Code: tea.KeyDelete},
	} {
		model = edit(t, model, msg)
	}
	if got := inputLine(t, model); got != "› AxB" {
		t.Fatalf("caret edits = %q", got)
	}
	if model.View().Cursor.X != 7 {
		t.Fatal("caret did not follow the edits")
	}
}

func TestLongInputAndNarrowScreenEditing(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 80, 12)
	text := strings.Repeat("中文👩‍💻", 25) + "TAIL"
	model = edit(t, model, tea.PasteMsg{Content: text})
	for _, size := range [][2]int{{20, 10}, {4, 4}, {1, 1}, {2, 2}, {0, 0}, {20, 10}} {
		model = resize(t, model, size[0], size[1])
		model = edit(t, model, tea.KeyPressMsg{Code: 'z', Text: "z"})
		model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
		if size[0] > 0 {
			inputLine(t, model)
		}
	}
	if got := inputLine(t, model); !strings.HasSuffix(got, "TAIL") {
		t.Fatalf("long input did not scroll to caret: %q", got)
	}
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	model = edit(t, model, tea.KeyPressMsg{Code: '!', Text: "!"})
	if got := inputLine(t, model); !strings.HasSuffix(got, "TAI!") {
		t.Fatalf("could not edit scrolled input: %q", got)
	}
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyHome})
	if got := inputLine(t, model); !strings.HasPrefix(got, "› 中文👩‍💻") {
		t.Fatalf("Home did not reveal start: %q", got)
	}
	model = resize(t, model, 400, 12)
	if got := inputLine(t, model); got != "› "+strings.TrimSuffix(text, "L")+"!" {
		t.Fatal("resizing lost input")
	}
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	if strings.TrimSpace(lines[11]) != "" || strings.TrimSpace(lines[8]) != "" || strings.TrimSpace(lines[10]) != "" {
		t.Fatal("editing damaged the bottom layout")
	}
}

func TestPasteStaysOnOneLine(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 80, 12)
	model = edit(t, model, tea.PasteMsg{Content: "a\nb\rc\td\x1b\x00\u0085e"})
	if got := inputLine(t, model); got != "› a b c de" {
		t.Fatalf("paste allowed line breaks or terminal controls: %q", got)
	}
	resize(t, model, 80, 12)
}

func TestNarrowInputShowsWholeGraphemeWhenItFitsTheTerminal(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 20, 8)
	model = edit(t, model, tea.PasteMsg{Content: "A👩‍💻"})
	model = resize(t, model, 2, 2)
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyEnd})
	if got := inputLine(t, model); got != "" || model.View().Cursor.X != 0 {
		t.Fatalf("caret after full-width emoji = %+v on %q, want an empty insertion row", model.View().Cursor, got)
	}
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	if got := inputLine(t, model); got != "👩‍💻" {
		t.Fatalf("input at two terminal columns = %q, want the whole emoji", got)
	}
	model = resize(t, model, 1, 1)
	inputLine(t, model)
	model = resize(t, model, 20, 8)
	if got := inputLine(t, model); got != "› A👩‍💻" {
		t.Fatalf("widening changed the draft to %q", got)
	}
}

func TestInputAreaGrowsAndShrinksBelowHalfScreenLimit(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 14, 16)
	checkFirstLine := func(y int) {
		t.Helper()
		rows := strings.Split(ansi.Strip(model.View().Content), "\n")
		if !strings.HasPrefix(strings.TrimSpace(rows[y]), "› abcdefg") {
			t.Fatalf("first editing row at y=%d = %q; screen: %q", y, rows[y], rows)
		}
	}
	model = edit(t, model, tea.PasteMsg{Content: "abcdefg"})
	checkFirstLine(13)
	model = edit(t, model, tea.PasteMsg{Content: "h"})
	checkFirstLine(12)
	model = resize(t, model, 30, 16)
	checkFirstLine(13)
	model = resize(t, model, 14, 16)
	checkFirstLine(12)
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	checkFirstLine(13)
}

func TestWrappedDraftGrowsScrollsAndSubmitsOriginalText(t *testing.T) {
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- body.Input
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task_id":"wrapped"}`))
	}))
	defer server.Close()

	model := resize(t, tui.NewWithTask("/workspace", tui.TaskOptions{ServerURL: server.URL}), 14, 6)
	const first = "A中e\u0301👩‍💻B"
	const second = "甲乙丙C"
	const third = "中文😀D"
	model = edit(t, model, tea.KeyPressMsg{Code: 'A', Text: "A"})
	if got := inputLine(t, model); got != "› A" {
		t.Fatalf("single row = %q", got)
	}
	model = edit(t, model, tea.PasteMsg{Content: strings.TrimPrefix(first, "A") + second})
	view := model.View()
	rows := strings.Split(ansi.Strip(view.Content), "\n")
	if got := strings.TrimSpace(rows[view.Cursor.Y-1]); got != "› "+first {
		t.Fatalf("first wrapped row = %q", got)
	}
	if got := strings.TrimSpace(inputLine(t, model)); got != second || view.Cursor.X != 11 {
		t.Fatalf("second wrapped row = %q, cursor = %+v", got, view.Cursor)
	}

	model = edit(t, model, tea.PasteMsg{Content: third})
	view = model.View()
	rows = strings.Split(ansi.Strip(view.Content), "\n")
	if strings.Contains(ansi.Strip(view.Content), first) {
		t.Fatal("the first row remained visible after reaching the height limit")
	}
	if got := strings.TrimSpace(rows[view.Cursor.Y-1]); got != second {
		t.Fatalf("row above the caret after scrolling = %q", got)
	}
	if got := strings.TrimSpace(inputLine(t, model)); got != third {
		t.Fatalf("third wrapped row = %q", got)
	}
	for range 4 {
		model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	if got := strings.TrimSpace(inputLine(t, model)); got != second {
		t.Fatalf("deleting the last wrapped row left %q at the caret", got)
	}
	if !strings.Contains(ansi.Strip(model.View().Content), first) {
		t.Fatal("deleting the last wrapped row did not reveal the first row")
	}

	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	model = resize(t, model, 80, 6)
	if got := inputLine(t, model); got != "› "+first+second || model.View().Cursor.X != 17 {
		t.Fatalf("widened row = %q, cursor = %+v", got, model.View().Cursor)
	}
	model = resize(t, model, 14, 6)
	model = edit(t, model, tea.KeyPressMsg{Code: '!', Text: "!"})
	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter did not submit the draft")
	}
	cmd()
	select {
	case got := <-requests:
		if want := first + "甲乙丙!C"; got != want {
			t.Fatalf("submitted draft = %q, want %q", got, want)
		}
	default:
		t.Fatal("Enter sent no task request")
	}
}
