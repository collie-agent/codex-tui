package tui_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func toolFrame(kind, delta string) string {
	payload, _ := json.Marshal(map[string]string{"ID": fmt.Sprint(eventSequence.Add(1)), "Type": kind, "Delta": delta})
	return "data: " + string(payload) + "\n\n"
}

// Tool tests exercise SSE and the public model, without inspecting output storage.
func toolModel(t *testing.T, frames ...string) (tea.Model, tea.Cmd) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, frame := range frames {
			fmt.Fprint(w, frame)
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	attached := tui.NewWithTask(".", tui.TaskOptions{TaskID: "tools", ServerURL: server.URL})
	t.Cleanup(attached.Close)
	var m tea.Model = resize(t, attached, 100, 60)
	cmd := m.Init()
	for i := 0; i < len(frames)+1; i++ {
		m, cmd = m.Update(receive(t, cmd))
	}
	return m, cmd
}

func visible(m tea.Model) string { return ansi.Strip(m.View().Content) }

func clickTool(t *testing.T, m tea.Model, label string) tea.Model {
	t.Helper()
	for y, row := range strings.Split(visible(m), "\n") {
		if x := strings.Index(row, label); x >= 0 {
			x = ansi.StringWidth(row[:x])
			m, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
			m, _ = m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x, Y: y})
			return m
		}
	}
	t.Fatalf("missing %q in\n%s", label, visible(m))
	return m
}

func TestToolRegionGroupsSummariesAndExpandsInCallOrder(t *testing.T) {
	m, _ := toolModel(t,
		toolFrame("tool_call", `{"call_id":"a","name":"read_file","arguments":{"path":"a.txt"}}`),
		toolFrame("tool_call", `{"call_id":"b","name":"list_dir","arguments":{"path":"."}}`),
		toolFrame("tool_call", `{"call_id":"c","name":"read_file","arguments":{"path":"b.py"}}`),
		toolFrame("tool_result", `{"call_id":"a","result":[{"type":"text","text":"secret-a"}]}`),
	)
	view := visible(m)
	if !strings.Contains(view, "read_file a.txt, b.py") || strings.Index(view, "read_file") > strings.Index(view, "list_dir .") || strings.Contains(view, "secret-a") || strings.Contains(view, "lines") {
		t.Fatalf("unexpected collapsed region:\n%s", view)
	}
	m = clickTool(t, m, "+ expand")
	view = visible(m)
	a, b, c := strings.Index(view, `"path": "a.txt"`), strings.Index(view, `"path": "."`), strings.Index(view, `"path": "b.py"`)
	if a < 0 || b <= a || c <= b || !strings.Contains(view, "secret-a") || !strings.Contains(view, "running") {
		t.Fatalf("incorrect call details order:\n%s", view)
	}
	m = clickTool(t, m, "− collapse")
	if strings.Contains(visible(m), "secret-a") {
		t.Fatal("collapse leaked result")
	}
}

func TestToolRegionsKeepLateResultsAndIndependentExpansion(t *testing.T) {
	m, _ := toolModel(t,
		toolFrame("answer_delta", `{"text":"intro"}`),
		toolFrame("tool_call", `{"call_id":"a","name":"first","arguments":"raw argument"}`),
		toolFrame("tool_result", `{"call_id":"a","result":[{"type":"text","text":"short-result"}]}`),
		toolFrame("text", `{"text":"intro"}`),
		toolFrame("tool_call", `{"call_id":"b","name":"second","arguments":{}}`),
		toolFrame("answer_delta", `{"text":"boundary"}`),
		toolFrame("tool_call", `{"call_id":"c","name":"third","arguments":{}}`),
		toolFrame("tool_error", `{"call_id":"b","error":"partial failure","result":[{"type":"text","text":"late-result"},{"type":"image","data":"encoded"}]}`),
	)
	view := visible(m)
	if strings.Count(view, "intro") != 1 || strings.Count(view, "+ expand") != 2 || !strings.Contains(view, "failed") || strings.Contains(view, "partial failure") || strings.Contains(view, "short-result") {
		t.Fatalf("wrong region boundaries or preview:\n%s", view)
	}
	m = clickTool(t, m, "+ expand")
	view = visible(m)
	if !strings.Contains(view, "short-result") || !strings.Contains(view, "partial failure") || !strings.Contains(view, `"type": "image"`) || strings.Index(view, "late-result") > strings.Index(view, "boundary") || strings.Count(view, "+ expand") != 1 {
		t.Fatalf("late result or independent expansion lost:\n%s", view)
	}
	m = resize(t, m, 35, 60)
	if !strings.Contains(visible(m), "− collapse") || strings.Count(visible(m), "+ expand") != 1 {
		t.Fatal("resize reset region state")
	}
}

func TestToolSummariesUseOperationAndPreserveFullArguments(t *testing.T) {
	m, _ := toolModel(t,
		toolFrame("tool_call", `{"call_id":"a","name":"exec_command","arguments":{"command":"go test ./...","timeout":30}}`),
		toolFrame("tool_call", `{"call_id":"b","name":"search","arguments":{"query":"needle","path":"src"}}`),
		toolFrame("tool_call", `{"call_id":"c","name":"mystery","arguments":{"path":"keep","extra":2}}`),
		toolFrame("tool_call", `{"call_id":"d","name":"read_file","arguments":"unparsed readable"}`),
	)
	view := visible(m)
	for _, want := range []string{"exec_command go test ./...", "search needle", `mystery {"path":"keep","extra":2}`, "read_file unparsed readable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing summary %q:\n%s", want, view)
		}
	}
	m = clickTool(t, m, "+ expand")
	if !strings.Contains(visible(m), `"timeout": 30`) || !strings.Contains(visible(m), `"path": "src"`) {
		t.Fatal("summary discarded full arguments")
	}
}

func TestExpandedRegionUpdatesLiveAndFailureRemainsVisible(t *testing.T) {
	frames := make(chan string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		for {
			select {
			case frame := <-frames:
				fmt.Fprint(w, frame)
				w.(http.Flusher).Flush()
			case <-r.Context().Done():
				return
			}
		}
	}))
	defer server.Close()
	attached := tui.NewWithTask(".", tui.TaskOptions{TaskID: "live", ServerURL: server.URL})
	defer attached.Close()
	var m tea.Model = resize(t, attached, 100, 60)
	m, cmd := m.Update(receive(t, m.Init()))
	send := func(kind, delta string) {
		t.Helper()
		frames <- toolFrame(kind, delta)
		m, cmd = m.Update(receive(t, cmd))
	}
	send("tool_call", `{"call_id":"a","name":"read_file","arguments":{"path":"a"}}`)
	m = clickTool(t, m, "+ expand")
	send("tool_call", `{"call_id":"b","name":"read_file","arguments":{"path":"b"}}`)
	send("tool_error", `{"call_id":"a","error":"failure","result":[{"type":"text","text":"partial"}]}`)
	send("tool_result", `{"call_id":"a","result":[{"type":"text","text":"final-result"}]}`)
	view := visible(m)
	if !strings.Contains(view, `"path": "b"`) || !strings.Contains(view, "final-result") || !strings.Contains(view, "partial") || !strings.Contains(view, "failed") || !strings.Contains(view, "running") || strings.Contains(view, "Completed") {
		t.Fatalf("live details or status lost:\n%s", view)
	}
	m = clickTool(t, m, "− collapse")
	if !strings.Contains(visible(m), "read_file a [failed], b [running]") {
		t.Fatalf("status is not associated with its summary:\n%s", visible(m))
	}
	send("tool_result", `{"call_id":"b","result":[{"type":"text","text":"hidden-result"}]}`)
	if strings.Contains(visible(m), "hidden-result") || !strings.Contains(visible(m), "failed") || !strings.Contains(visible(m), "+ expand") {
		t.Fatalf("collapsed update or failure lost:\n%s", visible(m))
	}
	m = clickTool(t, m, "+ expand")
	if !strings.Contains(visible(m), "hidden-result") {
		t.Fatal("short result cannot expand")
	}
}

func TestHistoricalRegionClickDragCopyAndResize(t *testing.T) {
	tail, _ := json.Marshal(map[string]string{"text": strings.Repeat("tail\n", 45)})
	m, _ := toolModel(t,
		toolFrame("answer_delta", `{"text":"anchor"}`),
		toolFrame("tool_call", `{"call_id":"a","name":"alpha","arguments":{}}`),
		toolFrame("tool_result", `{"call_id":"a","result":[{"type":"text","text":"copy🙂 result  "}]}`),
		toolFrame("answer_delta", `{"text":"between"}`),
		toolFrame("tool_call", `{"call_id":"b","name":"beta","arguments":{}}`),
		toolFrame("tool_result", `{"call_id":"b","result":[{"type":"text","text":"other-secret"}]}`),
		toolFrame("answer_delta", string(tail)),
	)
	m = resize(t, m, 70, 26)
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if strings.TrimSpace(strings.Split(visible(m), "\n")[2]) != "anchor" {
		t.Fatal("did not reach history")
	}
	var toggleY int
	for y, row := range strings.Split(visible(m), "\n") {
		if strings.Contains(row, "+ expand") {
			toggleY = y
			break
		}
	}
	// Margin clicks and drags returning to their origin cannot expand a region.
	m, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: toggleY})
	m, _ = m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: 0, Y: toggleY})
	m, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 2, Y: toggleY})
	m, _ = m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 4, Y: toggleY})
	m, _ = m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 2, Y: toggleY})
	m, _ = m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: 2, Y: toggleY})
	if strings.Contains(visible(m), "− collapse") {
		t.Fatal("margin click or drag expanded region")
	}
	m = clickTool(t, m, "+ expand")
	copied := false
	for y, row := range strings.Split(visible(m), "\n") {
		if strings.Contains(row, "copy🙂 result") {
			copied = true
			m = drag(t, m, [2]int{2, y}, [2]int{60, y})
			copySelection(t, m, "copy🙂 result  ")
		}
	}
	if !copied {
		t.Fatal("expanded result was not visible for copying")
	}
	for _, width := range []int{28, 100, 70} {
		m = resize(t, m, width, 26)
	}
	view := visible(m)
	if strings.TrimSpace(strings.Split(view, "\n")[2]) != "anchor" || strings.Contains(view, "other-secret") || !strings.Contains(view, "+ expand") || !strings.Contains(view, "− collapse") {
		t.Fatalf("history or independent expansion lost:\n%s", view)
	}
	m = clickTool(t, m, "− collapse")
	if strings.TrimSpace(strings.Split(visible(m), "\n")[2]) != "anchor" {
		t.Fatal("collapse moved history")
	}
	for i := 0; i < 10; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if !strings.Contains(visible(m), "tail") || strings.Contains(visible(m), "回到底部") {
		t.Fatal("page down did not resume follow")
	}
}

func TestRawMultilineSummaryStaysOnOneLogicalLine(t *testing.T) {
	m, _ := toolModel(t, toolFrame("tool_call", `{"call_id":"a","name":"unknown","arguments":"raw\nsecond line"}`))
	if !strings.Contains(visible(m), `unknown raw\nsecond line [running]`) {
		t.Fatalf("summary created extra logical lines:\n%s", visible(m))
	}
	m = clickTool(t, m, "+ expand")
	rows := strings.Split(visible(m), "\n")
	for i := range rows {
		rows[i] = strings.TrimSpace(rows[i])
	}
	if !strings.Contains(strings.Join(rows, "\n"), "raw\nsecond line") {
		t.Fatal("details lost original argument lines")
	}
}

func TestNewAssistantTextAfterCallsStartsAnotherRegion(t *testing.T) {
	m, _ := toolModel(t,
		toolFrame("answer_delta", `{"text":"planning"}`),
		toolFrame("tool_call", `{"call_id":"a","name":"first","arguments":{}}`),
		toolFrame("answer_delta", `{"text":"new answer"}`),
		toolFrame("tool_call", `{"call_id":"b","name":"second","arguments":{}}`),
	)
	view := visible(m)
	if !strings.Contains(view, "planning") || !strings.Contains(view, "new answer") || strings.Count(view, "+ expand") != 2 {
		t.Fatalf("new assistant text merged across calls:\n%s", view)
	}
}

func TestExecAndGlobSummariesShowCommandArgumentsAndPattern(t *testing.T) {
	m, _ := toolModel(t,
		toolFrame("tool_call", `{"call_id":"a","name":"exec","arguments":{"command":"git","args":["status","--short"]}}`),
		toolFrame("tool_call", `{"call_id":"b","name":"glob","arguments":{"pattern":"**/*.go","path":"src"}}`),
	)
	if !strings.Contains(visible(m), "exec git status --short") || !strings.Contains(visible(m), "glob **/*.go") {
		t.Fatalf("lost command or search operation:\n%s", visible(m))
	}
}

func TestReadFileSummaryDeduplicatesPathAndRetainsDetails(t *testing.T) {
	m, _ := toolModel(t,
		toolFrame("tool_call", `{"call_id":"a","name":"read_file","arguments":{"path":"a.txt","offset":1}}`),
		toolFrame("tool_call", `{"call_id":"b","name":"read_file","arguments":{"path":"a.txt","offset":5}}`),
		toolFrame("tool_error", `{"call_id":"b","error":"bad"}`),
	)
	view := visible(m)
	if strings.Count(view, "a.txt") != 1 || !strings.Contains(view, "[running]") || !strings.Contains(view, "[failed]") {
		t.Fatalf("incorrect summary: %s", view)
	}
	m = clickTool(t, m, "+ expand")
	if !strings.Contains(visible(m), `"offset": 1`) || !strings.Contains(visible(m), `"offset": 5`) {
		t.Fatalf("missing details: %s", visible(m))
	}
}

func TestApplyPatchShowsFilesAndPatchDetails(t *testing.T) {
	m, _ := toolModel(t, toolFrame("tool_call", `{"call_id":"a","name":"apply_patch","arguments":{"patch":"*** Begin Patch\n*** Update File: a.txt\n@@\n-old\n+new\n*** Add File: b.txt\n+hello\n*** End Patch"}}`))
	view := visible(m)
	if !strings.Contains(view, "apply_patch a.txt, b.txt") || strings.Contains(view, "+hello") {
		t.Fatalf("incorrect patch summary: %s", view)
	}
	m = clickTool(t, m, "+ expand")
	if !strings.Contains(visible(m), "+hello") || !strings.Contains(visible(m), "-old") {
		t.Fatalf("missing patch details: %s", visible(m))
	}
}
