package tui

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

// Remote controls are escaped before any styling, wrapping or clipboard access.
func safeRemote(text string) string {
	var out strings.Builder
	for _, r := range text {
		if r != '\n' && (unicode.IsControl(r) || r == '\u2028' || r == '\u2029') {
			q := strconv.QuoteRune(r)
			out.WriteString(q[1 : len(q)-1])
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}
func (m *Model) applyEvent(event taskEvent) {
	var delta struct{ Text string }
	switch event.Type {
	case "task_info":
		var info struct {
			ConvID string `json:"conv_id"`
			RunID  string `json:"run_id"`
		}
		if json.Unmarshal([]byte(event.Delta), &info) != nil {
			return
		}
		// Run identity is scoped to this task; conversation identity is verified independently.
		if strings.TrimSpace(info.RunID) != "" {
			m.runID = info.RunID
		}
		// Delta contains a second JSON document; only conv_id identifies the conversation.
		if strings.TrimSpace(info.ConvID) != "" {
			// Never switch an instance to a different conversation, even if later
			// events return to the original ID or report completion.
			if m.conversationID != "" && m.conversationID != info.ConvID {
				m.conversationError = "Protocol error: conversation conflict; submission blocked"
				m.connection = m.conversationError
			} else if m.conversationError == "" {
				m.conversationID = info.ConvID
				m.conversationConfirmed = true
			}
		}
	case "answer_delta":
		if err := json.Unmarshal([]byte(event.Delta), &delta); err != nil {
			m.connection = "Connection error: invalid answer Delta"
			m.Close()
			return
		}
		if delta.Text == "" {
			return
		}
		// Answer deltas are decoded by the server.
		if !m.streaming || m.textInterrupted {
			m.appendResponse("")
			m.streaming = true
			m.textInterrupted = false
		}
		m.responses[len(m.responses)-1] += delta.Text
	case "text":
		if err := json.Unmarshal([]byte(event.Delta), &delta); err != nil {
			m.connection = "Connection error: invalid text Delta"
			m.Close()
			return
		}
		// A streaming answer already rendered its text; standalone text starts a response.
		if !m.streaming && delta.Text != "" {
			m.appendResponse(delta.Text)
		}
		m.streaming = false
		m.textInterrupted = false
	case "tool_call", "tool_result", "tool_error":
		// A call ends the contiguous delta run, but its final text may arrive
		// after tool events. Retain that merge target until another text starts.
		if event.Type == "tool_call" && m.streaming {
			m.textInterrupted = true
		}
		m.applyTool(event)
	case "usage":
		// Usage precedes the final text in the server stream; keep its merge target.
	case "task_completed", "task_interrupted":
		m.completed = true
		if !m.conversationConfirmed && m.conversationError == "" {
			m.conversationError = "Conversation error: missing valid task_info; submission blocked"
		}
		status := "Completed"
		if event.Type == "task_interrupted" {
			status = "Interrupted"
		}
		if m.conversationError != "" {
			status += " · " + m.conversationError
		}
		m.connection = status
	}
}

// appendRun starts a distinct reading unit for both saved and newly submitted runs.
func (m *Model) appendRun(text, prompt string) {
	m.appendResponse(text)
	m.output[len(m.output)-1].runStart = true
	m.output[len(m.output)-1].prompt = prompt
}

func (m *Model) appendResponse(text string) {
	if len(m.output) == 0 {
		for i := range m.responses {
			m.output = append(m.output, outputItem{response: i})
		}
	}
	m.responses = append(m.responses, text)
	m.output = append(m.output, outputItem{response: len(m.responses) - 1})
}
func (m Model) outputRows() []string {
	layout := m.outputLayout()
	rows := make([]string, len(layout))
	for i, row := range layout {
		rows[i] = row.text
	}
	return rows
}
