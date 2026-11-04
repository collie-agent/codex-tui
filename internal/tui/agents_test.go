package tui_test

import (
	"encoding/json"
	"fmt"
	"github.com/collie-agent/codex-tui/internal/tui"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func agentFrame(name, parent, kind, delta string) string {
	return agentInstanceFrame(name+"-instance", name, parent, kind, delta)
}

func agentInstanceFrame(instance, name, parent, kind, delta string) string {
	payload, _ := json.Marshal(map[string]string{"id": fmt.Sprint(eventSequence.Add(1)), "agent_instance_id": instance, "agent_name": name, "parent_agent_instance_id": parent, "type": kind, "delta": delta})
	return "data: " + string(payload) + "\n\n"
}

func TestAgentInstancesRouteToReusableIndependentViews(t *testing.T) {
	m, _ := toolModel(t,
		agentFrame("leader", "", "answer_delta", `{"text":"root answer"}`),
		agentFrame("researcher", "leader-id", "answer_delta", `{"text":"research "}`),
		agentFrame("reviewer", "leader-id", "answer_delta", `{"text":"review answer"}`),
		agentFrame("researcher", "another-instance", "answer_delta", `{"text":"answer"}`),
	)
	if !strings.Contains(visible(m), "root answer") || strings.Contains(visible(m), "research answer") {
		t.Fatalf("root output mixed: %s", visible(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !strings.Contains(visible(m), "research answer") || strings.Contains(visible(m), "root answer") || strings.Contains(visible(m), "review answer") {
		t.Fatalf("research view: %s", visible(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !strings.Contains(visible(m), "review answer") {
		t.Fatalf("review view: %s", visible(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !strings.Contains(visible(m), "root answer") {
		t.Fatalf("duplicate view created: %s", visible(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !strings.Contains(visible(m), "review answer") {
		t.Fatal("reverse navigation failed")
	}
}

func TestAgentViewRejectsEditingPastingAndSubmission(t *testing.T) {
	m, _ := toolModel(t, agentFrame("worker", "root", "answer_delta", `{"text":"worker output"}`))
	m, _ = m.Update(tea.KeyPressMsg{Text: "root draft", Code: 'r'})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	for _, msg := range []tea.Msg{
		tea.KeyPressMsg{Text: "forbidden draft", Code: 'f'},
		tea.PasteMsg{Content: "forbidden paste"},
		tea.KeyPressMsg{Code: tea.KeyEnter},
		tea.KeyPressMsg{Code: tea.KeyEscape},
	} {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd != nil {
			t.Fatal("read-only interaction issued a task command")
		}
	}
	if strings.Contains(visible(m), "forbidden") || strings.Contains(visible(m), "root draft") || m.View().Cursor != nil {
		t.Fatalf("subagent accepts input: %s", visible(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !strings.Contains(visible(m), "root draft") {
		t.Fatal("root draft lost")
	}
}

func TestAgentLifecycleCreatesViewBeforeTextAndUpdatesStatus(t *testing.T) {
	m, _ := toolModel(t,
		agentFrame("worker", "root", "agent_lifecycle", `{"status":"pending"}`),
		agentFrame("worker", "root", "agent_lifecycle", `{"status":"failed"}`),
	)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !strings.Contains(visible(m), "worker") || !strings.Contains(visible(m), "failed") {
		t.Fatalf("missing agent status: %s", visible(m))
	}
}

func TestAgentViewRetainsToolExpansionScrollingAndCopy(t *testing.T) {
	m, _ := toolModel(t,
		agentFrame("worker", "root", "tool_call", `{"call_id":"a","name":"read_file","arguments":{"path":"agent.txt"}}`),
		agentFrame("worker", "root", "tool_result", `{"call_id":"a","result":[{"type":"text","text":"agent secret"}]}`),
	)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = clickTool(t, m, "+ expand")
	if !strings.Contains(visible(m), "agent secret") {
		t.Fatal("tool details missing")
	}
	for y, row := range strings.Split(visible(m), "\n") {
		if x := strings.Index(row, "agent secret"); x >= 0 {
			m = drag(t, m, [2]int{x, y}, [2]int{x + 12, y})
			m = copySelection(t, m, "agent secret")
			break
		}
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !strings.Contains(visible(m), "agent secret") {
		t.Fatal("switch reset expansion")
	}
	m = resize(t, m, 45, 10)
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if !strings.Contains(visible(m), "read_file") {
		t.Fatalf("cannot scroll to tool: %s", visible(m))
	}
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if !strings.Contains(visible(m), "agent secret") {
		t.Fatalf("cannot page down to result: %s", visible(m))
	}
}

func TestAgentViewReceivesLiveAppendAndTaskCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w,
			agentFrame("worker", "root", "answer_delta", `{"text":"first "}`),
			agentFrame("worker", "root", "answer_delta", `{"text":"second"}`),
			agentFrame("worker", "root", "agent_lifecycle", `{"status":"completed"}`),
			toolFrame("task_completed", `{}`),
		)
	}))
	defer server.Close()
	attached := tui.NewWithTask(".", tui.TaskOptions{TaskID: "live", ServerURL: server.URL})
	defer attached.Close()
	var m tea.Model = resize(t, attached, 90, 20)
	cmd := m.Init()
	for i := 0; i < 2; i++ {
		m, cmd = m.Update(receive(t, cmd))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	for i := 0; i < 3; i++ {
		m, cmd = m.Update(receive(t, cmd))
	}
	if cmd != nil || !strings.Contains(visible(m), "first second") || !strings.Contains(visible(m), "completed") {
		t.Fatalf("live agent output or completion lost: %s", visible(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !strings.Contains(visible(m), "Completed") || strings.Contains(visible(m), "first second") {
		t.Fatalf("root completion lost: %s", visible(m))
	}
}

func TestSameNamedAgentInstancesKeepOutputToolsAndStatusSeparate(t *testing.T) {
	m, _ := toolModel(t,
		agentInstanceFrame("worker-1", "worker", "root", "answer_delta", `{"text":"first "}`),
		agentInstanceFrame("worker-2", "worker", "root", "text", `{"text":"second output"}`),
		agentInstanceFrame("worker-1", "worker", "root", "answer_delta", `{"text":"output"}`),
		agentInstanceFrame("worker-1", "worker", "root", "tool_call", `{"call_id":"a","name":"read_file","arguments":{"path":"first.txt"}}`),
		agentInstanceFrame("worker-2", "worker", "root", "tool_call", `{"call_id":"a","name":"read_file","arguments":{"path":"second.txt"}}`),
		agentInstanceFrame("worker-1", "worker", "root", "tool_result", `{"call_id":"a","result":[{"type":"text","text":"first secret"}]}`),
		agentInstanceFrame("worker-2", "worker", "root", "tool_result", `{"call_id":"a","result":[{"type":"text","text":"second secret"}]}`),
		agentInstanceFrame("worker-1", "worker", "root", "agent_lifecycle", `{"status":"failed"}`),
		agentInstanceFrame("worker-2", "worker", "root", "agent_lifecycle", `{"status":"completed"}`),
	)
	for _, expected := range []struct{ id, answer, path, secret, status, other string }{
		{"worker-1", "first output", "first.txt", "first secret", "failed", "second"},
		{"worker-2", "second output", "second.txt", "second secret", "completed", "first"},
	} {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = clickTool(t, m, "+ expand")
		view := visible(m)
		for _, want := range []string{expected.id, expected.answer, expected.path, expected.secret, expected.status} {
			if !strings.Contains(view, want) {
				t.Fatalf("missing %q in instance view: %s", want, view)
			}
		}
		if strings.Contains(view, expected.other) {
			t.Fatalf("other instance leaked: %s", view)
		}
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if strings.Contains(visible(m), "secret") {
		t.Fatal("agent output leaked to entry view")
	}
}
