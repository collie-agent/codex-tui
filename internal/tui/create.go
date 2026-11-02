package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type creation struct {
	cancel context.CancelFunc
	input  string
}
type createMessage struct {
	source *creation
	taskID string
	err    error
}

func (m Model) submitTask() (tea.Model, tea.Cmd) {
	if m.history != nil || m.conversationError != "" || m.create != nil || (m.taskID != "" && !m.completed && m.stream != nil) || strings.TrimSpace(m.input.text) == "" {
		return m, nil
	}
	// Detach the completed subscription before accepting any late messages.
	if m.stream != nil {
		m.stream.cancel()
		m.stream = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	pending := &creation{cancel: cancel, input: m.input.text}
	// Conversation identity survives tasks, but each task must confirm its own ownership.
	m.conversationConfirmed = false
	m.runID, m.interrupt, m.interruptState, m.interruptStatus = "", nil, interruptIdle, ""
	m.create = pending
	m.taskID, m.connection, m.completed, m.streaming = "Creating", "Creating task", false, false
	m.tools = nil
	m.appendRun("You: "+pending.input, pending.input)
	// Clear only the submitted draft now: an asynchronous response must never erase
	// edits made while the POST was in flight.
	m.input = lineInput{}
	options := m.options.normalized()
	return m, func() tea.Msg {
		// HTTP uses conv_id; ConvId belongs to the server's internal queue protocol.
		// Sending the queue field here silently creates a different conversation.
		body, _ := json.Marshal(struct {
			Input  string `json:"input"`
			ConvID string `json:"conv_id,omitempty"`
		}{pending.input, m.conversationID})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, options.ServerURL+"/tasks", bytes.NewReader(body))
		if err != nil {
			return createMessage{source: pending, err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := options.Client.Do(req)
		if err != nil {
			return createMessage{source: pending, err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			return createMessage{source: pending, err: fmt.Errorf("HTTP %s", resp.Status)}
		}
		var result struct {
			TaskID string `json:"task_id"`
		}
		if err = json.NewDecoder(resp.Body).Decode(&result); err == nil && strings.TrimSpace(result.TaskID) == "" {
			err = fmt.Errorf("missing task_id")
		}
		return createMessage{source: pending, taskID: result.TaskID, err: err}
	}
}
func (m Model) finishCreation(msg createMessage) (tea.Model, tea.Cmd) {
	if m.create == nil || msg.source != m.create {
		return m, nil
	}
	pending := m.create
	pending.cancel()
	m.create = nil
	if msg.err != nil {
		m.interruptState, m.interruptStatus = interruptIdle, ""
		m.connection = "Create error: " + safeRemote(msg.err.Error()) + " · retry manually"
		// Keep both the retry input and any draft typed during the failed request.
		m.input.text = pending.input + m.input.text
		m.input.caret = len(m.input.text)
		m.stream = nil
		return m, nil
	}
	if m.stream != nil {
		m.stream.cancel()
	}
	options := m.options.normalized()
	options.TaskID = msg.taskID
	m.stream = newSubscription(options)
	m.taskID, m.connection = msg.taskID, "Connecting"
	return m, m.Init()
}
