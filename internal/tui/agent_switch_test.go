package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAgentSwitchDisplaysInInitiatingAgentView(t *testing.T) {
	m, _ := toolModel(t,
		agentFrame("supervisor", "", "agent_switch", `{"src_agent_name":"supervisor","dst_agent_name":"worker"}`),
		agentFrame("worker", "root-id", "agent_switch", `{"src_agent_name":"worker","dst_agent_name":"supervisor"}`),
	)
	const outbound = "从supervisor切换到worker"
	const inbound = "从worker切换到supervisor"
	if strings.Count(visible(m), outbound) != 1 || strings.Contains(visible(m), inbound) {
		t.Fatalf("entry view switch notice: %s", visible(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if strings.Count(visible(m), inbound) != 1 || strings.Contains(visible(m), outbound) {
		t.Fatalf("child view switch notice: %s", visible(m))
	}
}

func TestAgentSwitchNoticeStaysSeparateFromSurroundingAnswers(t *testing.T) {
	m, _ := toolModel(t,
		agentFrame("worker", "root-id", "answer_delta", `{"text":"before switch"}`),
		agentFrame("worker", "root-id", "agent_switch", `{"src_agent_name":"worker","dst_agent_name":"helper"}`),
		agentFrame("worker", "root-id", "answer_delta", `{"text":"after switch"}`),
		agentFrame("worker", "root-id", "answer_delta", `{"text":" continued"}`),
	)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	view := visible(m)
	var lines []string
	for _, line := range strings.Split(view, "\n") {
		lines = append(lines, strings.TrimSpace(line))
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{"\nbefore switch\n", "\n从worker切换到helper\n", "\nafter switch continued\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("notice merged with answer, missing %q: %s", want, view)
		}
	}
}

func TestAgentSwitchIgnoresIncompleteNamesWithoutBreakingAnswer(t *testing.T) {
	for _, delta := range []string{
		`{}`, `{"src_agent_name":"worker"}`, `{"dst_agent_name":"helper"}`,
		`{"src_agent_name":"worker","dst_agent_name":"  "}`,
		`{"src_agent_name":12,"dst_agent_name":"helper"}`,
	} {
		t.Run(delta, func(t *testing.T) {
			m, _ := toolModel(t,
				agentFrame("worker", "root-id", "answer_delta", `{"text":"intact "}`),
				agentFrame("worker", "root-id", "agent_switch", delta),
				agentFrame("worker", "root-id", "answer_delta", `{"text":"answer"}`),
			)
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			if !strings.Contains(visible(m), "intact answer") || strings.Contains(visible(m), "切换到") {
				t.Fatalf("invalid switch corrupted output: %s", visible(m))
			}
		})
	}
}
