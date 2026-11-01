package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type interruptionState string

const (
	interruptIdle       interruptionState = ""
	interruptDeferred   interruptionState = "deferred"
	interruptRequesting interruptionState = "requesting"
	interruptWaiting    interruptionState = "waiting"
	interruptFailed     interruptionState = "failed"
)

type interruption struct{ stream *subscription }
type interruptMessage struct {
	source   *interruption
	err      error
	terminal bool
}

func (m Model) requestInterruption() (tea.Model, tea.Cmd) {
	if m.interruptState == interruptRequesting || m.interruptState == interruptWaiting {
		return m, nil
	}
	pending := &interruption{stream: m.stream}
	m.interrupt, m.interruptState = pending, interruptRequesting
	m.interruptStatus = "Requesting interruption"
	options, runID := m.options.normalized(), m.runID
	return m, func() tea.Msg {
		req, err := http.NewRequestWithContext(pending.stream.ctx, http.MethodPost, options.ServerURL+"/runs/"+url.PathEscape(runID)+"/interrupt", nil)
		if err != nil {
			return interruptMessage{source: pending, err: err}
		}
		resp, err := options.Client.Do(req)
		if err != nil {
			return interruptMessage{source: pending, err: err}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if err != nil {
			return interruptMessage{source: pending, err: err}
		}
		var result struct {
			Status  string `json:"status"`
			Error   string `json:"error"`
			Detail  string `json:"detail"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &result)
		if resp.StatusCode == http.StatusAccepted && result.Status == "accepted" || resp.StatusCode == http.StatusConflict && result.Status == "already_requested" {
			return interruptMessage{source: pending}
		}
		if resp.StatusCode == http.StatusConflict && result.Status == "terminal" {
			return interruptMessage{source: pending, terminal: true}
		}
		reason := result.Error
		if reason == "" {
			reason = result.Detail
		}
		if reason == "" {
			reason = result.Message
		}
		if reason == "" {
			reason = strings.TrimSpace(string(body))
		}
		return interruptMessage{source: pending, err: fmt.Errorf("HTTP %s %s", resp.Status, reason)}
	}
}
func (m Model) finishInterruption(msg interruptMessage) (tea.Model, tea.Cmd) {
	// A terminal event or a newer task always wins over an old HTTP response.
	if m.completed || msg.source != m.interrupt || msg.source.stream != m.stream {
		return m, nil
	}
	if msg.err != nil {
		m.interruptState = interruptFailed
		m.interruptStatus = "Interrupt error: " + safeRemote(msg.err.Error()) + " · ESC to retry"
	} else {
		m.interruptState = interruptWaiting
		m.interruptStatus = "Interrupt accepted · waiting for terminal confirmation"
		if msg.terminal {
			m.interruptStatus = "Run ended · waiting for terminal confirmation"
		}
	}
	return m, nil
}

// Keep protocol progress independent of transport recovery and ownership errors.
func (m Model) taskStatus() string {
	if m.completed || m.interruptStatus == "" {
		return m.connection
	}
	status := m.interruptStatus
	if m.conversationError != "" {
		status += " · " + m.conversationError
	}
	if m.connection != m.conversationError {
		status += " · " + m.connection
	}
	return status
}
