package tui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func TestCtrlAAndCtrlEMoveToEdgesOfCurrentWrappedLine(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  rune
		x    int
		want string
	}{
		{"start", 'a', 4, "› abcdefg!中e\u0301👩‍💻CD世界😀F"},
		{"end", 'e', 11, "› abcdefg中e\u0301👩‍💻CD!世界😀F"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := resize(t, tui.New("/workspace"), 14, 12)
			model = edit(t, model, tea.PasteMsg{Content: "abcdefg中e\u0301👩‍💻CD世界😀F"})
			model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
			model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
			model = edit(t, model, tea.KeyPressMsg{Code: tt.key, Mod: tea.ModCtrl})
			if row, cursor := inputLine(t, model), model.View().Cursor; row != "  中e\u0301👩‍💻CD" || cursor.X != tt.x {
				t.Fatalf("Ctrl+%c left the caret at %+v on %q", tt.key, cursor, row)
			}
			model = edit(t, model, tea.KeyPressMsg{Code: '!', Text: "!"})
			model = resize(t, model, 80, 12)
			if got := inputLine(t, model); got != tt.want {
				t.Fatalf("draft after Ctrl+%c and insertion = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestFilteredPasteKeepsCaretAtEndOfWrappedLine(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 14, 12)
	model = edit(t, model, tea.PasteMsg{Content: "abcdefgABCDEFG"})
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	model = edit(t, model, tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	before := model.View().Cursor
	if row := inputLine(t, model); row != "› abcdefg" || before.X != 11 {
		t.Fatalf("Ctrl+E left the caret at %+v on %q", before, row)
	}
	model = edit(t, model, tea.PasteMsg{Content: "\x1b\x00\u0085"})
	if after := model.View().Cursor; after.X != before.X || after.Y != before.Y {
		t.Fatalf("filtered paste moved the caret from %+v to %+v", before, after)
	}
}

func TestVerticalNavigationRestoresColumnAfterShortWrappedLine(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 14, 12)
	// The next double-width character cannot fit in the middle line's last cell.
	model = edit(t, model, tea.PasteMsg{Content: "abcdefgABCDEF中文e\u0301😀"})
	for _, step := range []struct {
		key  rune
		row  string
		x, y int
	}{
		{tea.KeyDown, "  中文e\u0301😀", 11, 9},
		{tea.KeyUp, "  ABCDEF", 10, 8},
		{tea.KeyUp, "› abcdefg", 11, 7},
		{tea.KeyUp, "› abcdefg", 11, 7},
		{tea.KeyDown, "  ABCDEF", 10, 8},
		{tea.KeyDown, "  中文e\u0301😀", 11, 9},
		{tea.KeyDown, "  中文e\u0301😀", 11, 9},
	} {
		key := tea.KeyPressMsg{Code: step.key}
		model = edit(t, model, key)
		if row, cursor := inputLine(t, model), model.View().Cursor; row != step.row || cursor.X != step.x || cursor.Y != step.y {
			t.Fatalf("%s left the caret at %+v on %q, want (%d, %d) on %q", key, cursor, row, step.x, step.y, step.row)
		}
	}
}

func TestVerticalNavigationLandsOnCompleteGraphemeBoundary(t *testing.T) {
	model := resize(t, tui.New("/workspace"), 14, 12)
	model = edit(t, model, tea.PasteMsg{Content: "abcdefg中e\u0301👩‍💻CD世界😀F"})
	model = edit(t, model, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	for range 2 {
		model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	}
	model = edit(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	if row, cursor := inputLine(t, model), model.View().Cursor; row != "  中e\u0301👩‍💻CD" || cursor.X != 7 {
		t.Fatalf("Up left the caret at %+v on %q, want the boundary before the emoji", cursor, row)
	}
	model = edit(t, model, tea.KeyPressMsg{Code: '!', Text: "!"})
	model = resize(t, model, 80, 12)
	if got := inputLine(t, model); got != "› abcdefg中e\u0301!👩‍💻CD世界😀F" {
		t.Fatalf("draft after moving and inserting = %q", got)
	}
}
