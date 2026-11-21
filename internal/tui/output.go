package tui

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

type answerChunk struct{ index, bytes int }

func safeRemote(text string) string {
	return expandDisplayTabs(escapeRemoteControls(text))
}

// Preserve line breaks and tabs for layout while escaping other remote controls.
func escapeRemoteControls(text string) string {
	var out strings.Builder
	for _, r := range text {
		if r != '\n' && r != '\t' && (unicode.IsControl(r) || r == '\u2028' || r == '\u2029') {
			q := strconv.QuoteRune(r)
			out.WriteString(q[1 : len(q)-1])
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func expandDisplayTabs(text string) string {
	if !strings.ContainsRune(text, '\t') {
		return text
	}
	const tabWidth = 4
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.ContainsRune(line, '\t') {
			continue
		}
		var expanded strings.Builder
		column := 0
		for segment, rest, found := strings.Cut(line, "\t"); ; segment, rest, found = strings.Cut(rest, "\t") {
			expanded.WriteString(segment)
			if !found {
				break
			}
			// Tabs follow terminal columns, including wide and styled text.
			column += ansi.StringWidth(segment)
			spaces := tabWidth - column%tabWidth
			expanded.WriteString(strings.Repeat(" ", spaces))
			column += spaces
		}
		lines[i] = expanded.String()
	}
	return strings.Join(lines, "\n")
}
func (m *Model) applyEvent(event taskEvent) {
	if m.routeAgentEvent(event) {
		return
	}
	// Only output-changing event families invalidate physical rows. Transport,
	// usage and lifecycle metadata are rendered outside the output layout.
	switch event.Type {
	case "agent_switch", "assistant_delta", "answer_delta", "output_reset", "text", "tool_call", "tool_result", "tool_error":
		m.invalidateOutputLayout()
	}
	var delta struct{ Text string }
	switch event.Type {
	case "agent_switch":
		var transition struct {
			Source      string `json:"src_agent_name"`
			Destination string `json:"dst_agent_name"`
		}
		if json.Unmarshal([]byte(event.Delta), &transition) != nil || strings.TrimSpace(transition.Source) == "" || strings.TrimSpace(transition.Destination) == "" {
			return
		}
		// A handoff is a separate reading item; later deltas must not merge into it.
		m.appendResponse("从" + transition.Source + "切换到" + transition.Destination)
		m.streaming, m.textInterrupted = false, false
	case "task_info":
		var info struct {
			ModelName string `json:"model_name"`
			ConvID    string `json:"conv_id"`
			RunID     string `json:"run_id"`
		}
		if json.Unmarshal([]byte(event.Delta), &info) != nil {
			return
		}
		m.modelName = info.ModelName
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
	case "assistant_delta":
		if err := json.Unmarshal([]byte(event.Delta), &delta); err != nil {
			m.connection = "Connection error: invalid assistant Delta"
			m.Close()
			return
		}
		if delta.Text == "" {
			return
		}
		// Process text is committed as a whole and is never part of answer rollback.
		// Keep the next answer separate and suppress the legacy text duplicate.
		m.appendResponse(delta.Text)
		m.streaming, m.textInterrupted = true, true
	case "answer_delta":
		if err := json.Unmarshal([]byte(event.Delta), &delta); err != nil {
			m.connection = "Connection error: invalid answer Delta"
			m.Close()
			return
		}
		if delta.Text == "" {
			return
		}
		// Both event names carry decoded text and share streaming/rollback state.
		if m.attemptEnded {
			m.attemptChunks = nil
			m.attemptEnded = false
		}
		if !m.streaming || m.textInterrupted {
			m.appendResponse("")
			m.streaming = true
			m.textInterrupted = false
		}
		m.responses[len(m.responses)-1] += delta.Text
		m.attemptChunks = append(m.attemptChunks, answerChunk{len(m.responses) - 1, len(delta.Text)})
	case "output_reset":
		var reset struct {
			AnswerBytes int `json:"answer_bytes"`
		}
		if json.Unmarshal([]byte(event.Delta), &reset) != nil || reset.AnswerBytes < 0 {
			return
		}
		remaining := reset.AnswerBytes
		for _, chunk := range m.attemptChunks {
			remaining -= chunk.bytes
		}
		if remaining > 0 {
			return
		}
		// Validate the entire rollback before changing any visible text.
		retracted := append([]string(nil), m.responses...)
		remaining = reset.AnswerBytes
		for i := len(m.attemptChunks) - 1; i >= 0 && remaining > 0; i-- {
			chunk := m.attemptChunks[i]
			n := min(remaining, chunk.bytes)
			s := retracted[chunk.index]
			if n > len(s) || !utf8.ValidString(s[:len(s)-n]) {
				return
			}
			retracted[chunk.index] = s[:len(s)-n]
			remaining -= n
		}
		m.responses = retracted
		m.attemptChunks = nil
		m.attemptEnded = false
		m.streaming, m.textInterrupted = false, false
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
		// Usage can precede validation failure and output_reset. Keep rollback
		// records until the next request starts emitting its answer.
		m.attemptEnded = true
		// Usage is per HTTP attempt, not a task completion signal or cumulative total.
		var usage tokenUsage
		if json.Unmarshal([]byte(event.Delta), &usage) == nil {
			m.usage.add(usage)
			// Usage ends one HTTP attempt, not the task. Preserve its text and
			// start a new reading item for the next attempt (including retries).
			m.streaming, m.textInterrupted = false, false
		}
	case "task_completed", "task_interrupted":
		m.finishTimer(event.Type == "task_completed")
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
	m.invalidateOutputLayout()
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

// tokenUsage preserves unknown counts instead of presenting them as zero.
type tokenUsage struct {
	Input  string `json:"input_tokens"`
	Cached string `json:"cached_input_tokens"`
	Output string `json:"output_tokens"`
}

func (u tokenUsage) label() string {
	var parts []string
	for _, metric := range []struct{ value, name string }{
		{u.Input, "in"}, {u.Cached, "cache in"}, {u.Output, "out"},
	} {
		n, ok := new(big.Int).SetString(metric.value, 10)
		if !ok || n.Sign() < 0 {
			continue
		}
		// Keep exact totals; only the displayed value is truncated to K.
		parts = append(parts, n.Quo(n, big.NewInt(1000)).String()+"K "+metric.name)
	}
	return strings.Join(parts, " · ")
}

// Totals survive task boundaries within this conversation. Unknown counts make
// that dimension unknown: a partial sum would understate the actual usage.
func (u *tokenUsage) add(next tokenUsage) {
	add := func(total, value string) string {
		if value == "" {
			return total
		}
		if total == "" {
			total = "0"
		}
		a, okA := new(big.Int).SetString(total, 10)
		b, okB := new(big.Int).SetString(value, 10)
		if !okA || !okB || a.Sign() < 0 || b.Sign() < 0 {
			return "unknown"
		}
		return a.Add(a, b).String()
	}
	u.Input = add(u.Input, next.Input)
	u.Cached = add(u.Cached, next.Cached)
	u.Output = add(u.Output, next.Output)
}
