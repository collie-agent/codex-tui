package tui_test

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func drag(t *testing.T, model tea.Model, from, to [2]int) tea.Model {
	t.Helper()
	before := model.View().Content
	if model.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("terminal mouse dragging is not enabled")
	}
	model = edit(t, model, tea.MouseClickMsg{X: from[0], Y: from[1], Button: tea.MouseLeft})
	model = edit(t, model, tea.MouseMotionMsg{X: to[0], Y: to[1], Button: tea.MouseLeft})
	if model.View().Content == before {
		t.Fatal("dragging did not visibly highlight the selection")
	}
	if ansi.Strip(model.View().Content) != ansi.Strip(before) {
		t.Fatal("highlighting changed visible text")
	}
	during := model.View().Content
	model = edit(t, model, tea.MouseReleaseMsg{X: to[0], Y: to[1], Button: tea.MouseLeft})
	if model.View().Content != during {
		t.Fatal("releasing the mouse lost the selection")
	}
	return model
}

func copySelection(t *testing.T, model tea.Model, want string) tea.Model {
	t.Helper()
	before := model.View().Content
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("selection did not produce a clipboard request")
	}
	// Executing this command only returns a request message. No Program runs,
	// so the renderer never writes OSC 52 or changes the real clipboard.
	request := cmd()
	if !reflect.DeepEqual(request, tea.SetClipboard(want)()) {
		t.Fatalf("clipboard request = %q, want payload %q", request, want)
	}
	if !utf8.ValidString(want) || ansi.Strip(want) != want {
		t.Fatal("expected copy payload must be valid plain text")
	}
	if next.View().Content != before {
		t.Fatal("copy changed the selection or input")
	}
	return next
}

func TestSelectionCopyRegionsAndDirections(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to [2]int
		want     string
	}{
		{"title", [2]int{3, 1}, [2]int{14, 1}, "OpenAI Codex"},
		{"path", [2]int{3, 2}, [2]int{9, 2}, "/项目"},
		{"input", [2]int{2, 7}, [2]int{10, 7}, "A中👩‍💻e\u0301B"},
		{"status", [2]int{2, 9}, [2]int{21, 9}, "GPT-6.1 Terra medium"},
		{"cross line", [2]int{3, 1}, [2]int{7, 2}, "OpenAI Codex (v0.165.0)\n   /项目"},
		{"blank rows", [2]int{3, 2}, [2]int{2, 7}, "/项目\n\n\n\n\n› A"},
	} {
		for _, reverse := range []bool{false, true} {
			name := tc.name
			if reverse {
				name += " reverse"
			}
			t.Run(name, func(t *testing.T) {
				model := resize(t, tui.New("/项目"), 80, 10)
				model = edit(t, model, tea.PasteMsg{Content: "A中👩‍💻e\u0301B"})
				from, to := tc.from, tc.to
				if reverse {
					from, to = to, from
				}
				model = drag(t, model, from, to)
				model = copySelection(t, model, tc.want)
				// Hovering after release cannot change a committed selection.
				model = edit(t, model, tea.MouseMotionMsg{X: 50, Y: 5})
				copySelection(t, model, tc.want)
				_, cmd := model.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
				if cmd == nil {
					t.Fatal("Ctrl+D did not quit with selection")
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("Ctrl+D did not request exit")
				}
			})
		}
	}
}

func TestSelectionWideCellBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text     string
		from, to int
		want     string
	}{
		{"A中B", 4, 5, "中B"}, // Starts on the second cell of 中.
		{"A中B", 2, 3, "A中"}, // Ends on the first cell of 中.
		{"A👩‍💻B", 4, 5, "👩‍💻B"},
		{"Ae\u0301B", 3, 4, "e\u0301B"},
		{"A🇨🇳B", 4, 5, "🇨🇳B"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			model := resize(t, tui.New("/workspace"), 40, 10)
			model = edit(t, model, tea.PasteMsg{Content: tc.text})
			model = drag(t, model, [2]int{tc.from, 7}, [2]int{tc.to, 7})
			copySelection(t, model, tc.want)
		})
	}
}

func TestSelectionClearingAndClamping(t *testing.T) {
	for _, change := range []tea.Msg{
		tea.MouseClickMsg{X: 4, Y: 1, Button: tea.MouseLeft},
		tea.KeyPressMsg{Code: 'x', Text: "x"},
		tea.KeyPressMsg{Code: tea.KeyLeft},
		tea.PasteMsg{Content: "new"},
		tea.WindowSizeMsg{Width: 20, Height: 5},
		tea.WindowSizeMsg{},
	} {
		model := resize(t, tui.New("/workspace"), 40, 10)
		model = drag(t, model, [2]int{3, 1}, [2]int{14, 1})
		model = edit(t, model, change)
		edit(t, model, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	}
	model := resize(t, tui.New("/workspace"), 20, 10)
	model = drag(t, model, [2]int{-10, -1}, [2]int{200, 100})
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	copySelection(t, model, strings.Join(lines, "\n"))
}

func TestSelectionCopiesVisibleScrolledInput(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 20, 10)
	model = edit(t, model, tea.PasteMsg{Content: strings.Repeat("中文", 30) + "TAIL"})
	line := strings.Split(ansi.Strip(model.View().Content), "\n")[7]
	model = drag(t, model, [2]int{2, 7}, [2]int{19, 7})
	copySelection(t, model, strings.TrimRight(ansi.Cut(line, 2, 20), " "))
}

func TestCopyPreservesContentSpaces(t *testing.T) {
	for _, tc := range []struct {
		text string
		end  int
		want string
	}{
		{"A B", 3, "A "},
		{"A  ", 20, "A  "},
		{"  ", 20, "  "},
	} {
		for _, reverse := range []bool{false, true} {
			model := resize(t, tui.New("/workspace"), 40, 10)
			model = edit(t, model, tea.PasteMsg{Content: tc.text})
			from, to := [2]int{2, 7}, [2]int{tc.end, 7}
			if reverse {
				from, to = to, from
			}
			model = drag(t, model, from, to)
			copySelection(t, model, tc.want)
		}
	}
}
