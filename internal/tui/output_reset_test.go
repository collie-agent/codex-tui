package tui_test

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
)

func TestOutputResetRetractsFailedAnswer(t *testing.T) {
	m, _ := toolModel(t,
		toolFrame("answer_delta", `{"text":"earlier"}`),
		toolFrame("usage", `{}`),
		toolFrame("answer_delta", `{"text":"错误"}`),
		toolFrame("usage", `{}`),
		toolFrame("output_reset", `{"text_bytes":30,"answer_bytes":6}`),
		toolFrame("answer_delta", `{"text":"correct"}`),
	)
	view := visible(m)
	if !strings.Contains(view, "earlier") || !strings.Contains(view, "correct") || strings.Contains(view, "错误") {
		t.Fatalf("failed answer was not retracted: %s", view)
	}
}

func TestOutputResetKeepsOtherAgentsAnswers(t *testing.T) {
	m, _ := toolModel(t,
		agentFrame("worker", "root", "answer_delta", `{"text":"失败"}`),
		agentFrame("reviewer", "root", "answer_delta", `{"text":"keep"}`),
		agentFrame("worker", "root", "output_reset", `{"answer_bytes":6,"text_bytes":9}`),
		agentFrame("worker", "root", "answer_delta", `{"text":"重试"}`),
	)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if view := visible(m); strings.Contains(view, "失败") || !strings.Contains(view, "重试") {
		t.Fatalf("worker: %s", view)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if view := visible(m); !strings.Contains(view, "keep") {
		t.Fatalf("reviewer: %s", view)
	}
}
