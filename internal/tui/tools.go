package tui

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type outputItem struct {
	response int
	region   *toolRegion
}
type toolOutput struct {
	name, arguments string
	results         []string
	status          string
}

// A region owns expansion while calls retain identity for late results.
type toolRegion struct {
	calls    []*toolOutput
	expanded bool
}
type outputRow struct {
	text          string
	block, offset int
	toggle        *toolRegion
}
type toolPress struct {
	tool  *toolRegion
	x, y  int
	moved bool
}

func prettyJSON(raw string) string {
	var formatted bytes.Buffer
	if json.Indent(&formatted, []byte(raw), "", "  ") == nil {
		return formatted.String()
	}
	return raw
}

func (m *Model) applyTool(event taskEvent) {
	var delta struct {
		Name      string          `json:"name"`
		CallID    string          `json:"call_id"`
		Arguments json.RawMessage `json:"arguments"`
		Result    json.RawMessage `json:"result"`
		Error     json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal([]byte(event.Delta), &delta); err != nil {
		m.appendResponse("Tool event error: " + event.Delta)
		return
	}
	if m.tools == nil {
		m.tools = make(map[string]*toolOutput)
	}
	tool := m.tools[delta.CallID]
	// Missing IDs cannot safely associate separate events with one another.
	if tool == nil || delta.CallID == "" {
		tool = &toolOutput{name: delta.Name, status: "running"}
		if delta.CallID != "" {
			m.tools[delta.CallID] = tool
		}
		var region *toolRegion
		if len(m.output) > 0 {
			region = m.output[len(m.output)-1].region
		}
		if region == nil {
			region = &toolRegion{}
			m.output = append(m.output, outputItem{region: region})
		}
		region.calls = append(region.calls, tool)
	}
	if delta.Name != "" {
		tool.name = delta.Name
	}
	if event.Type == "tool_call" {
		args := string(delta.Arguments)
		_ = json.Unmarshal(delta.Arguments, &args)
		tool.arguments = prettyJSON(args)
		return
	}
	// Later partial results must not erase an observed failure.
	if tool.status != "failed" {
		tool.status = "completed"
	}
	if event.Type == "tool_error" {
		tool.status = "failed"
		message := string(delta.Error)
		_ = json.Unmarshal(delta.Error, &message)
		tool.results = append(tool.results, "Error: "+message)
	}
	if len(delta.Result) > 0 && string(delta.Result) != "null" {
		tool.results = append(tool.results, renderToolContent(delta.Result))
	}
}

func renderToolContent(raw json.RawMessage) string {
	var content []json.RawMessage
	if json.Unmarshal(raw, &content) != nil {
		return prettyJSON(string(raw))
	}
	parts := make([]string, 0, len(content))
	for _, item := range content {
		var block struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		if json.Unmarshal(item, &block) == nil && block.Type == "text" && block.Text != nil {
			parts = append(parts, *block.Text)
		} else {
			parts = append(parts, prettyJSON(string(item)))
		}
	}
	return strings.Join(parts, "\n")
}

// Rendering and hit testing share physical rows, including Unicode wrapping.
func (m Model) outputLayout() []outputRow {
	var rows []outputRow
	// Logical lines and byte offsets remain stable when terminal width changes.
	block := 0
	wrap := func(text string) []outputRow {
		var wrapped []outputRow
		for _, logical := range strings.Split(safeRemote(text), "\n") {
			offset := 0
			for _, line := range strings.Split(ansi.Hardwrap(logical, max(1, m.contentWidth()), true), "\n") {
				wrapped = append(wrapped, outputRow{text: line, block: block, offset: offset})
				offset += len(line)
			}
			block++
		}
		return wrapped
	}
	appendText := func(text string) { rows = append(rows, wrap(text)...) }
	items := m.output
	// Keep models containing plain historical responses compatible with the viewport.
	if len(items) == 0 {
		for i := range m.responses {
			items = append(items, outputItem{response: i})
		}
	}
	for index, item := range items {
		// Separate reading units once per region, never once per call.
		if index > 0 && ((item.region == nil) != (items[index-1].region == nil)) {
			appendText("")
		}
		if item.region == nil {
			appendText(m.responses[item.response])
			continue
		}
		region := item.region
		label := "+ expand"
		if region.expanded {
			label = "− collapse"
		}
		for _, line := range wrap(label) {
			line.toggle = region
			rows = append(rows, line)
		}
		if region.expanded {
			for _, tool := range region.calls {
				appendText("● " + tool.name + " [" + tool.status + "]")
				if tool.arguments != "" {
					appendText(tool.arguments)
				}
				if len(tool.results) > 0 {
					appendText(strings.Join(tool.results, "\n"))
				}
			}
			continue
		}
		var names []string
		groups := make(map[string][]string)
		for _, tool := range region.calls {
			if _, ok := groups[tool.name]; !ok {
				names = append(names, tool.name)
			}
			summary := strings.ReplaceAll(callSummary(tool.name, tool.arguments), "\n", `\n`)
			if tool.status != "completed" {
				summary += " [" + tool.status + "]"
			}

			groups[tool.name] = append(groups[tool.name], summary)
		}
		for _, name := range names {
			appendText(strings.ReplaceAll(name, "\n", `\n`) + " " + strings.Join(groups[name], ", "))
		}
	}

	return rows
}
func callSummary(name, arguments string) string {
	var args map[string]json.RawMessage
	if json.Unmarshal([]byte(arguments), &args) == nil {
		// Prefer the operation's main argument; unknown tools keep all parameters.
		name = strings.ToLower(name)
		var keys []string
		switch {
		case strings.Contains(name, "search"), strings.Contains(name, "grep"), name == "glob":
			keys = []string{"query", "pattern", "search_term", "search"}
		case strings.Contains(name, "command"), strings.Contains(name, "exec"), name == "shell":
			keys = []string{"command", "cmd"}
		case strings.Contains(name, "file"), strings.Contains(name, "dir"):
			keys = []string{"path", "file_path", "directory", "dir_path"}
		}
		for _, key := range keys {
			var value string
			if json.Unmarshal(args[key], &value) == nil {
				if key == "command" || key == "cmd" {
					var argv []string
					if json.Unmarshal(args["args"], &argv) == nil && len(argv) > 0 {
						value += " " + strings.Join(argv, " ")
					}
				}
				return value
			}
		}
	}
	var compact bytes.Buffer
	if json.Compact(&compact, []byte(arguments)) == nil {
		return compact.String()
	}
	return arguments
}

func (m Model) toolAt(x, y int) *toolRegion {
	x -= m.margin()
	if x < 0 || x >= m.contentWidth() || y < 1 || y >= m.layout().inputStart {
		return nil
	}
	rows := m.outputLayout()
	start, end := m.outputRange(len(rows))
	index := start + y - 1
	if index >= end || x >= ansi.StringWidth(rows[index].text) {
		return nil
	}
	return rows[index].toggle
}
