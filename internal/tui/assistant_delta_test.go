package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAssistantDeltaSurvivesAnswerReset(t *testing.T) {
	m, _ := toolModel(t,
		toolFrame("assistant_delta", `{"text":"先检查代码"}`),
		toolFrame("text", `{"text":"先检查代码"}`),
		toolFrame("tool_call", `{"call_id":"a","name":"read_file","arguments":{"path":"main.go"}}`),
		toolFrame("answer_delta", `{"text":"错误"}`),
		toolFrame("output_reset", `{"answer_bytes":6,"text_bytes":99}`),
		toolFrame("answer_delta", `{"text":"正确答案"}`),
	)
	view := visible(m)
	if strings.Count(view, "先检查代码") != 1 || strings.Contains(view, "错误") || !strings.Contains(view, "正确答案") {
		t.Fatalf("process text or answer rollback incorrect: %s", view)
	}
	if strings.Index(view, "先检查代码") > strings.Index(view, "read_file") || strings.Index(view, "read_file") > strings.Index(view, "正确答案") {
		t.Fatalf("process/tool/answer order incorrect: %s", view)
	}
}

func TestAssistantDeltaRoutesToAgent(t *testing.T) {
	m, _ := toolModel(t,
		agentFrame("worker", "root", "assistant_delta", `{"text":"检查子任务"}`),
		agentFrame("worker", "root", "answer_delta", `{"text":"子任务答案"}`),
	)
	if strings.Contains(visible(m), "检查子任务") {
		t.Fatal("child process text leaked into root output")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if view := visible(m); !strings.Contains(view, "检查子任务") || !strings.Contains(view, "子任务答案") {
		t.Fatalf("child process text missing: %s", view)
	}
}
