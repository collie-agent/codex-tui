package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func resize(t *testing.T, model tea.Model, width, height int) tea.Model {
	t.Helper()
	model, cmd := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	if cmd != nil {
		t.Fatal("resize produced an unexpected effect")
	}
	view := model.View()
	if !view.AltScreen {
		t.Fatal("expected alternate screen")
	}
	content := ansi.Strip(view.Content)
	if width == 0 || height == 0 {
		if content != "" {
			t.Fatal("zero-sized screen must be empty")
		}
		return model
	}
	lines := strings.Split(content, "\n")
	if len(lines) != height {
		t.Fatalf("got %d rows, want %d", len(lines), height)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) != width {
			t.Fatalf("row width = %d, want %d: %q", ansi.StringWidth(line), width, line)
		}
	}
	return model
}

func TestInitialLayoutAndResize(t *testing.T) {
	var model tea.Model = tui.New("/workspace/项目")
	for _, size := range [][2]int{{120, 30}, {100, 20}, {120, 40}} {
		model = resize(t, model, size[0], size[1])
		lines := strings.Split(ansi.Strip(model.View().Content), "\n")
		if strings.TrimSpace(lines[1]) != "" || strings.TrimSpace(lines[2]) != "" {
			t.Fatal("expected empty top rows")
		}
		if strings.TrimSpace(lines[3]) != "" {
			t.Fatal("expected flexible blank space")
		}
		if !strings.Contains(lines[len(lines)-3], "Ask Codex to do anything") {
			t.Fatal("input is not anchored at bottom")
		}
		if strings.TrimSpace(lines[len(lines)-1]) != "" {
			t.Fatal("status should be empty before data arrives")
		}
	}
}

func TestConstrainedScreens(t *testing.T) {
	var model tea.Model = tui.New("/very/long/目录/that/must/not/wrap")
	for _, size := range [][2]int{{20, 10}, {1, 1}, {4, 2}, {30, 3}, {30, 4}, {0, 0}, {80, 24}} {
		model = resize(t, model, size[0], size[1])
		if size == [2]int{20, 10} {
			lines := strings.Split(ansi.Strip(model.View().Content), "\n")
			if strings.TrimSpace(lines[7]) != "› Ask Codex to d" {
				t.Fatalf("unexpected truncated input: %q", lines[7])
			}
			if strings.Contains(lines[9], "weekly") {
				t.Fatal("status should be truncated")
			}
		}
	}
}

func TestControlKeys(t *testing.T) {
	var model tea.Model = tui.New("/workspace")
	model = resize(t, model, 100, 24)
	before := model.View().Content
	next, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd != nil || next.View().Content != before {
		t.Fatal("Ctrl+C must not quit, copy, or change the view")
	}
	_, cmd = next.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+D must quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+D did not request exit")
	}
}

func TestCursorTracksInputOnShortScreens(t *testing.T) {
	for height := 1; height <= 10; height++ {
		model := resize(t, tui.New("/workspace"), 40, height)
		view := model.View()
		lines := strings.Split(ansi.Strip(view.Content), "\n")
		if view.Cursor == nil || !strings.Contains(lines[view.Cursor.Y], "Ask Codex to do anything") {
			t.Fatalf("height %d: cursor is not on input row", height)
		}
	}
}
