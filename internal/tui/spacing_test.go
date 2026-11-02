package tui_test

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
	"strings"
	"testing"
)

func TestAllTextUsesTwoCellMarginsAndInputCaret(t *testing.T) {
	var m tea.Model = tui.New("/项目")
	for _, size := range [][2]int{{80, 15}, {12, 8}, {5, 5}, {4, 4}, {2, 2}, {1, 1}, {0, 0}, {80, 15}} {
		m = resize(t, m, size[0], size[1])
		m = edit(t, m, tea.PasteMsg{Content: "中🙂abcdef"})
		m = resize(t, m, size[0], size[1])
		if size[0] > 4 {
			for _, row := range strings.Split(visible(m), "\n") {
				if !strings.HasPrefix(row, "  ") || !strings.HasSuffix(row, "  ") {
					t.Fatalf("missing margins at width %d: %q", size[0], row)
				}
			}
			c := m.View().Cursor
			if c == nil || c.X < 2 || c.X >= size[0]-2 {
				t.Fatalf("caret outside content: %+v", c)
			}
		}
	}
	m = edit(t, m, tea.KeyPressMsg{Code: tea.KeyHome})
	if m.View().Cursor.X != 4 {
		t.Fatalf("input caret not offset by margin: %+v", m.View().Cursor)
	}
	rows := strings.Split(ansi.Strip(m.View().Content), "\n")
	if !strings.HasPrefix(rows[1], "  >_") || !strings.HasPrefix(rows[2], "  /项目") || strings.TrimSpace(rows[14]) != "" {
		t.Fatalf("inconsistent text origins:\n%s", visible(m))
	}
}

func TestToolRegionsHaveSingleBlankSeparatorsAndWrapWithinMargins(t *testing.T) {
	m, _ := toolModel(t,
		toolFrame("answer_delta", `{"text":"before"}`),
		toolFrame("tool_call", `{"call_id":"a","name":"read_file","arguments":{"path":"中文🙂abcdefghijk"}}`),
		toolFrame("tool_call", `{"call_id":"b","name":"list_dir","arguments":{"path":"."}}`),
		toolFrame("answer_delta", `{"text":"after"}`),
	)
	rows := strings.Split(visible(m), "\n")
	if strings.TrimSpace(rows[2]) != "before" || strings.TrimSpace(rows[3]) != "" || !strings.Contains(rows[4], "+ expand") || !strings.Contains(rows[5], "read_file") || !strings.Contains(rows[6], "list_dir") || strings.TrimSpace(rows[7]) != "" || strings.TrimSpace(rows[8]) != "after" {
		t.Fatalf("incorrect region spacing:\n%s", visible(m))
	}
	m = resize(t, m, 18, 60)
	for _, row := range strings.Split(visible(m), "\n") {
		if !strings.HasPrefix(row, "  ") || !strings.HasSuffix(row, "  ") {
			t.Fatalf("wrapped text crossed margins: %q", row)
		}
	}
	m = clickTool(t, m, "+ expand")
	for _, size := range [][2]int{{5, 60}, {1, 1}, {0, 0}, {18, 60}} {
		m = resize(t, m, size[0], size[1])
	}
	if !strings.Contains(visible(m), "− collapse") {
		t.Fatal("resize lost expansion")
	}
}
