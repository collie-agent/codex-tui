package tui_test

import (
	"strings"
	"testing"
)

func TestToolExpansionPreservesPreviousModelView(t *testing.T) {
	previous, _ := toolModel(t,
		toolFrame("tool_call", `{"call_id":"a","name":"read_file","arguments":{"path":"history.txt"}}`),
		toolFrame("tool_result", `{"call_id":"a","result":[{"type":"text","text":"isolated tool detail 中文🙂"}]}`),
	)
	before := previous.View().Content
	expanded := clickTool(t, previous, "+ expand")
	if !strings.Contains(visible(expanded), "isolated tool detail 中文🙂") {
		t.Fatal("expanded model does not show tool details")
	}
	if previous.View().Content != before {
		t.Fatal("expanding the next model changed the previous model's view")
	}
}
