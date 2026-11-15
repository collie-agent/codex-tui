package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestPerformanceFixtureShowsHistoryAndStreamTail(t *testing.T) {
	m := performanceFixture(t, 2, false)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"baseline-tail", "Connected", "中文🙂", "+ expand"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in fixture:\n%s", want, view)
		}
	}
	m = performanceUpdate(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	for _, want := range []string{"回到底部", "plain-000", "Markdown history", "func main()", "input", "output"} {
		if !strings.Contains(ansi.Strip(m.View().Content), want) {
			t.Fatalf("historical viewport missing %q", want)
		}
	}
	m = performanceReturn(t, m)
	m = performanceEvent(m, "answer_delta", `{"text":" visible-delta"}`)
	if !strings.Contains(ansi.Strip(m.View().Content), "baseline-tail 中文🙂 visible-delta") {
		t.Fatalf("stream delta did not reach viewport: %s", ansi.Strip(m.View().Content))
	}
	m = performanceEvent(m, "task_completed", `{}`)
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Completed") || strings.Contains(view, "Conversation error") {
		t.Fatal("completion did not reach viewport")
	}
}

func performanceUpdate(m Model, msg tea.Msg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func performanceEvent(m Model, kind, delta string) Model {
	return performanceUpdate(m, streamMessage{event: &taskEvent{Type: kind, Delta: delta}})
}

func performanceText(text string) string {
	data, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		panic(err)
	}
	return string(data)
}

// Feed the same decoded events as the transport, without timing network I/O.
// Commands are intentionally not run: this fixture owns the event sequence.
func performanceFixture(tb testing.TB, units int, expanded bool) Model {
	tb.Helper()
	m := NewWithTask("/baseline", TaskOptions{TaskID: "baseline"})
	tb.Cleanup(m.Close)
	m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = performanceUpdate(m, streamMessage{connected: true})
	m = performanceEvent(m, "task_info", `{"conv_id":"baseline-conversation","model_name":"baseline-model"}`)
	for i := 0; i < units; i++ {
		m = performanceEvent(m, "text", performanceText(fmt.Sprintf("plain-%03d 中文🙂 deterministic history", i)))
		m = performanceEvent(m, "text", performanceText("## Markdown history\n\n**bold** and *emphasis* with [link](https://example.com).\n\n- first item\n- 中文🙂 second item\n\n> quoted history\n\n```go\nfunc main() { println(42) }\n```\n\n| Key | Value |\n| --- | --- |\n| input | output |"))
		call := fmt.Sprintf("call-%03d", i)
		m = performanceEvent(m, "tool_call", fmt.Sprintf(`{"call_id":%q,"name":"read_file","arguments":{"path":"baseline.txt"}}`, call))
		result, _ := json.Marshal(map[string]any{"call_id": call, "result": []map[string]string{{"type": "text", "text": "tool-result-start\n" + strings.Repeat("large tool result 中文🙂 0123456789\n", 128) + "tool-result-end"}}})
		m = performanceEvent(m, "tool_result", string(result))
		if expanded {
			m = performanceClick(tb, m, "+ expand")
		}
		m = performanceEvent(m, "text", performanceText("unit-end 中文🙂"))
		m = performanceReturn(tb, m)
	}
	return performanceEvent(m, "answer_delta", performanceText("baseline-tail 中文🙂"))
}

func performanceClick(tb testing.TB, m Model, label string) Model {
	tb.Helper()
	for y, row := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if x := strings.Index(row, label); x >= 0 {
			column := ansi.StringWidth(row[:x])
			m = performanceUpdate(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: column, Y: y})
			return performanceUpdate(m, tea.MouseReleaseMsg{Button: tea.MouseLeft, X: column, Y: y})
		}
	}
	tb.Fatalf("missing clickable %q", label)
	return m
}

func TestPerformanceFixtureExpandedDetails(t *testing.T) {
	m := performanceFixture(t, 2, true)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"tool-result-end", "baseline-tail", "中文🙂"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded fixture missing %q:\n%s", want, view)
		}
	}
}

func performanceReturn(tb testing.TB, m Model) Model {
	tb.Helper()
	if strings.Contains(ansi.Strip(m.View().Content), "回到底部") {
		return performanceClick(tb, m, "回到底部")
	}
	return m
}

var performanceView tea.View

func TestPerformanceChurnPreservesReadableHistory(t *testing.T) {
	m := performanceFixture(t, 2, false)
	for cycle := 1; cycle <= 3; cycle++ {
		m = performanceChurn(t, m, cycle)
		if view := ansi.Strip(m.View().Content); !strings.Contains(view, "baseline-tail 中文🙂") {
			t.Fatalf("cycle %d lost the stream tail: %s", cycle, view)
		}
	}
	// Re-read the oldest content through scrolling, rather than inspecting storage.
	for i := 0; i < 20; i++ {
		m = performanceUpdate(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "plain-000 中文🙂") {
		t.Fatalf("oldest history is no longer readable: %s", view)
	}
}

// Exercise real hit testing twice so every cycle restores the original expansion
// state. No synthetic cache invalidation or direct viewport mutation is needed.
func performanceChurn(tb testing.TB, m Model, cycle int) Model {
	tb.Helper()
	m = performanceReturn(tb, m)
	for page := 0; ; page++ {
		view := ansi.Strip(m.View().Content)
		label, opposite := "+ expand", "− collapse"
		if strings.Contains(view, opposite) {
			label, opposite = opposite, label
		}
		if strings.Contains(view, label) {
			m = performanceClick(tb, m, label)
			m = performanceClick(tb, m, opposite)
			break
		}
		if page == 1000 {
			tb.Fatal("could not reach a tool region through scrolling")
		}
		m = performanceUpdate(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
	m = performanceUpdate(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	performanceView = m.View()
	width := 80
	if cycle%2 == 0 {
		width = 100
	}
	m = performanceUpdate(m, tea.WindowSizeMsg{Width: width, Height: 40})
	m = performanceReturn(tb, m)
	performanceView = m.View()
	return m
}

func BenchmarkRenderPerformance(b *testing.B) {
	for _, size := range []struct {
		name  string
		units int
	}{{"Short", 2}, {"Long", 20}} {
		for _, expanded := range []bool{false, true} {
			label := "Collapsed"
			if expanded {
				label = "Expanded"
			}
			b.Run(size.name+"/"+label, func(b *testing.B) {
				base := performanceFixture(b, size.units, expanded)
				if !strings.Contains(ansi.Strip(base.View().Content), "baseline-tail") {
					b.Fatal("fixture tail is not visible")
				}
				b.Run("View", func(b *testing.B) {
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						performanceView = base.View()
					}
				})
				b.Run("Stream", func(b *testing.B) {
					// Each operation starts at the same body length. Copy only mutable slices
					// outside timing so calibration cannot grow the fixture or contaminate base.
					delta := streamMessage{event: &taskEvent{Type: "answer_delta", Delta: performanceText(" stream-tail 中文🙂 **bold**")}}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						m := base
						m.responses = append([]string(nil), base.responses...)
						m.attemptChunks = append([]answerChunk(nil), base.attemptChunks...)
						b.StartTimer()
						m = performanceUpdate(m, delta)
						performanceView = m.View()
					}
				})
				b.Run("Scroll", func(b *testing.B) {
					// Alternate page up/down so every iteration moves instead of hitting an edge.
					m := base
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						code := tea.KeyPgUp
						if i%2 == 1 {
							code = tea.KeyPgDown
						}
						m = performanceUpdate(m, tea.KeyPressMsg{Code: code})
						performanceView = m.View()
					}
				})
			})
		}
	}
}

// A separate measurement keeps the original comparison intact while ensuring
// each delta is actually presented, including when tool details are expanded.
func BenchmarkRenderPresentedStream(b *testing.B) {
	for _, units := range []int{2, 20} {
		for _, expanded := range []bool{false, true} {
			// Share immutable history across calibration and repeated samples, as
			// BenchmarkRenderPerformance does; each operation owns mutable slices.
			base := performanceFixture(b, units, expanded)
			delta := streamMessage{event: &taskEvent{Type: "answer_delta", Delta: performanceText(" **presented 中文🙂**")}}
			b.Run(fmt.Sprintf("History%d/Expanded%t", units, expanded), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					m := base
					m.responses = append([]string(nil), base.responses...)
					m.attemptChunks = append([]answerChunk(nil), base.attemptChunks...)
					b.StartTimer()
					m = performanceUpdate(m, delta)
					m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
					performanceView = m.View()
					b.StopTimer()
					if !strings.Contains(ansi.Strip(performanceView.Content), "presented 中文🙂") {
						b.Fatal("new frame did not present the delta")
					}
					b.StartTimer()
				}
			})
		}
	}
}

// Source bytes are measured separately from the whole live heap: metadata,
// derived layouts and process-wide renderer state cannot be subtracted exactly.
func TestRenderPerformanceSourceMemory(t *testing.T) {
	if os.Getenv("TUI_PERF_PROFILE_DIR") == "" {
		t.Skip("opt-in source residency measurement")
	}
	for _, units := range []int{2, 20} {
		t.Run(fmt.Sprintf("History%d", units), func(t *testing.T) {
			m := performanceFixture(t, units, false)
			payloadBytes := 0
			for _, text := range m.responses {
				payloadBytes += len(text)
			}
			for _, tool := range m.tools {
				payloadBytes += len(tool.arguments)
				for _, result := range tool.results {
					payloadBytes += len(result)
				}
			}
			runtime.GC()
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			t.Logf("units=%d source payload bytes=%d HeapAlloc=%d", units, payloadBytes, stats.HeapAlloc)
			runtime.KeepAlive(m)
		})
	}
}

// Opt-in retained-heap experiment: fixed history, increasing refresh/scroll/resize
// cycles, including tool expansion/collapse. Keep the model alive across GC;
// TotalAlloc is never used as residency.
func TestRenderPerformanceMemory(t *testing.T) {
	directory := os.Getenv("TUI_PERF_PROFILE_DIR")
	if directory == "" {
		t.Skip("set TUI_PERF_PROFILE_DIR to capture retained heaps")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, expanded := range []bool{false, true} {
		label := "Collapsed"
		if expanded {
			label = "Expanded"
		}
		t.Run(label, func(t *testing.T) {
			m := performanceFixture(t, 20, expanded)
			// Payload accounting is a diagnostic, not a behavior assertion or a
			// heap-size estimate: it excludes metadata, renderer globals and slack.
			payloadBytes := 0
			for _, text := range m.responses {
				payloadBytes += len(text)
			}
			for _, tool := range m.tools {
				payloadBytes += len(tool.arguments)
				for _, result := range tool.results {
					payloadBytes += len(result)
				}
			}
			t.Logf("%s retained source payload bytes=%d", label, payloadBytes)
			var warmHeap uint64
			for cycle := 0; cycle <= 100; cycle++ {
				if cycle > 0 {
					m = performanceChurn(t, m, cycle)
				}
				if cycle != 0 && cycle != 5 && cycle != 15 && cycle != 25 && cycle != 50 && cycle != 75 && cycle != 100 {
					continue
				}
				runtime.GC()
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				t.Logf("%s cycles=%d HeapAlloc=%d HeapObjects=%d TotalAlloc=%d", label, cycle, stats.HeapAlloc, stats.HeapObjects, stats.TotalAlloc)
				if cycle == 5 {
					warmHeap = stats.HeapAlloc
				}
				if stats.HeapAlloc > 4<<20 {
					t.Errorf("cycle %d: live heap %d exceeds 4 MiB budget", cycle, stats.HeapAlloc)
				}
				if cycle > 5 && stats.HeapAlloc > warmHeap+(256<<10) {
					t.Errorf("cycle %d: heap grew %d bytes since warmup; budget 256 KiB", cycle, stats.HeapAlloc-warmHeap)
				}
				f, err := os.Create(filepath.Join(directory, fmt.Sprintf("%s-%02d.pprof", label, cycle)))
				if err != nil {
					t.Fatal(err)
				}
				err = pprof.WriteHeapProfile(f)
				closeErr := f.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
				runtime.KeepAlive(m)
			}
			if !strings.Contains(ansi.Strip(m.View().Content), "baseline-tail") {
				t.Fatal("fixed history lost its tail")
			}
		})
	}
}
