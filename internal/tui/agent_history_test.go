package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMultiAgentHistoryLoadsIndependentViewsAndTurns(t *testing.T) {
	call := toolSnapshot{Name: "read_file", Arguments: `{"path":"child.txt"}`, Status: "completed", Result: "saved contents"}
	final := json.RawMessage(`{"status":"completed","answer":"child final","Observation":["checked"],"CompletionEvidence":["verified"]}`)
	snapshot := conversationSnapshot{ConvID: "conv", Runs: []runSnapshot{{
		RunID: "run", ConvID: "conv", Goal: "inspect", Status: "completed",
		Result: json.RawMessage(`{"answer":"root final"}`), ToolCalls: []toolSnapshot{call},
		Agents: []agentSnapshot{
			{InstanceID: "root", Name: "supervisor", Goal: "inspect", Status: "completed", Result: json.RawMessage(`{"answer":"root final"}`)},
			{InstanceID: "child-1", ParentID: "root", Name: "worker", Goal: "read files", Status: "completed",
				CompletionCriteria: []string{"files checked"}, Result: final, ToolCalls: []toolSnapshot{call},
				Turns: []turnSnapshot{
					{ExecutionID: "turn-0", Result: json.RawMessage(`{"answer":"first attempt"}`), Error: "retry error"},
					{ExecutionID: "turn-1", Result: final, ToolCalls: []toolSnapshot{call}},
				}},
			{InstanceID: "nested", ParentID: "child-1", Name: "worker", Goal: "not started", Status: "unknown"},
			{InstanceID: "failed", ParentID: "root", Name: "", Status: "failed", Error: "saved failure"},
		},
	}}}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/conversations/conv/runs" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(snapshot)
	}))
	defer server.Close()
	m := NewWithTask(".", TaskOptions{ConversationID: "conv", ServerURL: server.URL})
	defer m.Close()
	m.width, m.height = 160, 80
	loaded, cmd := m.Update(m.Init()())
	m = loaded.(Model)
	if cmd != nil || requests != 1 || strings.Join(m.agentOrder, ",") != "child-1,nested,failed" {
		t.Fatalf("history order/transport: %v requests=%d cmd=%v", m.agentOrder, requests, cmd)
	}
	root := strings.Join(m.outputRows(), "\n")
	if strings.Count(root, "root final") != 1 || strings.Contains(root, "child final") || strings.Contains(root, "read_file") {
		t.Fatalf("root duplicated or mixed agent contents: %s", root)
	}
	if !strings.Contains(root, "    • worker · nested · unknown") {
		t.Fatalf("missing delegation tree: %s", root)
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	child := m.agents[m.activeAgent]
	text := ansi.Strip(m.View().Content)
	for _, want := range []string{"child-1", "Parent: root", "files checked", "Turn 1 · turn-0", "Turn 2 · turn-1", "first attempt", "retry error", "verified", "checked"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q: %s", want, text)
		}
	}
	if strings.Count(text, "child final") != 1 {
		t.Fatalf("duplicate turn/agent result: %s", text)
	}
	tools := 0
	for _, item := range child.output {
		if item.region != nil {
			tools += len(item.region.calls)
			item.region.expanded = true
		}
	}
	if tools != 1 || !strings.Contains(strings.Join(child.outputRows(), "\n"), "saved contents") {
		t.Fatal("tools duplicated across run, agent and turn, or details missing")
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !updated.(Model).agents["child-1"].readOnly {
		t.Fatal("saved child allowed submission")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	text = ansi.Strip(m.View().Content)
	if !strings.Contains(text, "Saved status: unknown") || !strings.Contains(text, "Parent: child-1") || strings.Contains(text, "child final") {
		t.Fatalf("unexecuted nested child inherited output or terminal state: %s", text)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if text = ansi.Strip(m.View().Content); !strings.Contains(text, "saved failure") || !strings.Contains(text, "failed") || strings.Contains(text, "supervisor") {
		t.Fatalf("nameless failed child lost or name inferred: %s", text)
	}
}

func TestSavedAgentWithoutTurnsUsesAggregateToolsAndFinalResult(t *testing.T) {
	m := New(".")
	m.appendSavedAgent(agentSnapshot{
		InstanceID: "worker", Status: "unknown",
		ToolCalls: []toolSnapshot{{Name: "pending_tool", Status: "running"}},
		Result:    json.RawMessage(`{"answer":"saved answer"}`), Error: "saved error",
	})
	text := strings.Join(m.responses, "\n")
	if !strings.Contains(text, "saved answer") || !strings.Contains(text, "saved error") || !strings.Contains(text, "unknown") {
		t.Fatalf("aggregate fallback lost saved data: %s", text)
	}
	for _, item := range m.output {
		if item.region != nil {
			call := item.region.calls[0]
			if call.status != "running" || len(call.results) != 0 {
				t.Fatal("unfinished tool inferred a result or terminal state")
			}
			return
		}
	}
	t.Fatal("aggregate tool omitted")
}
