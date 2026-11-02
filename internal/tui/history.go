package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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
	ToolCalls []struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Status    string `json:"status"`
		Result    string `json:"result"`
		Error     string `json:"error"`
	} `json:"tool_calls"`
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
		var region *toolRegion
		for _, call := range run.ToolCalls {
			if region == nil {
				region = &toolRegion{}
				m.output = append(m.output, outputItem{region: region})
			}
			tool := &toolOutput{name: call.Name, arguments: prettyJSON(call.Arguments), status: call.Status}
			if call.Result != "" {
				tool.results = append(tool.results, call.Result)
			}
			if call.Error != "" {
				tool.results = append(tool.results, "Error: "+call.Error)
			}
			region.calls = append(region.calls, tool)
		}
		var result struct {
			Answer string `json:"answer"`
		}
		if json.Unmarshal(run.Result, &result) == nil && result.Answer != "" {
			m.appendResponse(result.Answer)
		} else if len(run.Result) > 0 && string(run.Result) != "null" {
			m.appendResponse(prettyJSON(string(run.Result)))
		}
		if run.Error != "" {
			m.appendResponse("Error: " + run.Error)
		}
	}
	m.connection = fmt.Sprintf("Loaded %d runs", len(msg.snapshot.Runs))
	return m, nil
}
