package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type historyLoad struct {
	ctx    context.Context
	cancel context.CancelFunc
}
type conversationSnapshot struct {
	ConvID string        `json:"conv_id"`
	Runs   []runSnapshot `json:"runs"`
}
type runSnapshot struct {
	RunID     string          `json:"run_id"`
	ConvID    string          `json:"conv_id"`
	Goal      string          `json:"goal"`
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error"`
	ToolCalls []toolSnapshot  `json:"tool_calls"`
	Agents    []agentSnapshot `json:"agents"`
}
type toolSnapshot struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Status    string `json:"status"`
	Result    string `json:"result"`
	Error     string `json:"error"`
}
type turnSnapshot struct {
	ExecutionID string          `json:"execution_id"`
	Result      json.RawMessage `json:"result"`
	Error       string          `json:"error"`
	ToolCalls   []toolSnapshot  `json:"tool_calls"`
}
type agentSnapshot struct {
	InstanceID         string          `json:"agent_instance_id"`
	ParentID           string          `json:"parent_agent_instance_id"`
	Name               string          `json:"agent_name"`
	Goal               string          `json:"goal"`
	CompletionCriteria []string        `json:"completion_criteria"`
	Status             string          `json:"status"`
	Result             json.RawMessage `json:"result"`
	Error              string          `json:"error"`
	ToolCalls          []toolSnapshot  `json:"tool_calls"`
	Turns              []turnSnapshot  `json:"turns"`
}
type historyMessage struct {
	source   *historyLoad
	snapshot conversationSnapshot
	err      error
}

func (m Model) loadHistory() tea.Msg {
	msg := historyMessage{source: m.history}
	options := m.options.normalized()
	req, err := http.NewRequestWithContext(m.history.ctx, http.MethodGet, options.ServerURL+"/conversations/"+url.PathEscape(m.conversationID)+"/runs", nil)
	if err == nil {
		var resp *http.Response
		resp, err = options.Client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("HTTP %s", resp.Status)
			} else {
				err = json.NewDecoder(resp.Body).Decode(&msg.snapshot)
			}
		}
	}
	if err == nil && msg.snapshot.ConvID != m.conversationID {
		err = fmt.Errorf("conversation conflict")
	}
	if err == nil {
		for _, run := range msg.snapshot.Runs {
			if run.ConvID != "" && run.ConvID != m.conversationID {
				err = fmt.Errorf("conversation conflict")
				break
			}
		}
	}
	msg.err = err
	return msg
}

func (m Model) finishHistory(msg historyMessage) (tea.Model, tea.Cmd) {
	if m.history == nil || msg.source != m.history || m.history.ctx.Err() != nil {
		return m, nil
	}
	m.history.cancel()
	m.history = nil
	m.completed = true
	if msg.err != nil {
		m.connection = "History error: " + safeRemote(msg.err.Error())
		return m, nil
	}
	// Snapshots are already ordered by the server. They describe saved progress,
	// not completion events or permission to resume an executing run.
	for _, run := range msg.snapshot.Runs {
		m.appendRun("── Run "+run.RunID+" · "+run.Status+" ──\nYou: "+run.Goal, run.Goal)
		if len(run.Agents) == 0 {
			m.appendSnapshotTools(run.ToolCalls)
			m.appendSnapshotResult(run.Result)
		} else {
			m.appendAgentHistory(run)
		}
		if run.Error != "" {
			m.appendResponse("Error: " + run.Error)
		}
	}
	m.connection = fmt.Sprintf("Loaded %d runs", len(msg.snapshot.Runs))
	return m, nil
}

func (m *Model) appendSnapshotTools(calls []toolSnapshot) {
	if len(calls) == 0 {
		return
	}
	region := &toolRegion{}
	m.output = append(m.output, outputItem{region: region})
	for _, call := range calls {
		tool := &toolOutput{name: call.Name, arguments: prettyJSON(call.Arguments), status: call.Status}
		if call.Result != "" {
			tool.results = append(tool.results, call.Result)
		}
		if call.Error != "" {
			tool.results = append(tool.results, "Error: "+call.Error)
		}
		region.calls = append(region.calls, tool)
	}
}

func (m *Model) appendSnapshotResult(raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	var result map[string]json.RawMessage
	var answer string
	if json.Unmarshal(raw, &result) != nil || json.Unmarshal(result["answer"], &answer) != nil {
		m.appendResponse(prettyJSON(string(raw)))
		return
	}
	if answer != "" {
		m.appendResponse(answer)
	}
	// Preserve saved evidence without duplicating the answer's JSON envelope.
	for _, field := range []string{"Observation", "CompletionEvidence"} {
		value := result[field]
		if len(value) > 0 && string(value) != "null" && string(value) != "[]" {
			m.appendResponse(field + ": " + prettyJSON(string(value)))
		}
	}
}

func (m *Model) appendAgentHistory(run runSnapshot) {
	depths := make(map[string]int)
	var tree []string
	for _, saved := range run.Agents {
		depth := 0
		if saved.ParentID != "" {
			depth = depths[saved.ParentID] + 1
		}
		depths[saved.InstanceID] = depth
		tree = append(tree, strings.Repeat("  ", depth)+"• "+saved.Name+" · "+saved.InstanceID+" · "+saved.Status)
	}
	m.appendResponse("Agents · saved snapshot (Tab / Shift+Tab to switch)\n" + strings.Join(tree, "\n"))
	for _, saved := range run.Agents {
		if saved.ParentID == "" {
			m.appendSavedAgent(saved)
			continue
		}
		agent := m.agentView(saved.InstanceID, saved.Name)
		agent.connection = "Saved status: " + saved.Status
		agent.completed = true
		agent.conversationID = m.conversationID
		agent.appendRun("── Run "+run.RunID+" ──\nGoal: "+saved.Goal, saved.Goal)
		agent.appendSavedAgent(saved)
		m.agents[saved.InstanceID] = agent
	}
}

func (m *Model) appendSavedAgent(saved agentSnapshot) {
	m.appendResponse("Agent: " + saved.Name + " · " + saved.InstanceID + "\nSaved status: " + saved.Status + "\nGoal: " + saved.Goal)
	if saved.ParentID != "" {
		m.appendResponse("Parent: " + saved.ParentID)
	}
	if len(saved.CompletionCriteria) > 0 {
		m.appendResponse("Completion criteria:\n• " + strings.Join(saved.CompletionCriteria, "\n• "))
	}
	var lastResult json.RawMessage
	var lastError string
	if len(saved.Turns) == 0 {
		m.appendSnapshotTools(saved.ToolCalls)
	} else {
		// Turns own their tools; the agent and run lists are aggregate copies.
		for i, turn := range saved.Turns {
			m.appendResponse(fmt.Sprintf("── Turn %d · %s ──", i+1, turn.ExecutionID))
			m.appendSnapshotTools(turn.ToolCalls)
			m.appendSnapshotResult(turn.Result)
			if turn.Error != "" {
				m.appendResponse("Error: " + turn.Error)
			}
			lastResult, lastError = turn.Result, turn.Error
		}
	}
	var final, last any
	_ = json.Unmarshal(saved.Result, &final)
	_ = json.Unmarshal(lastResult, &last)
	if !reflect.DeepEqual(final, last) {
		m.appendSnapshotResult(saved.Result)
	}
	if saved.Error != "" && saved.Error != lastError {
		m.appendResponse("Error: " + saved.Error)
	}
}
