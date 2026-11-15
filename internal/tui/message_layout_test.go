package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"os"
	"runtime"
	"strings"
	"testing"
)

// A fixed history must not dominate allocation when only the current answer changes.
func TestStreamingHistoryAllocationBudget(t *testing.T) {
	base := performanceFixture(t, 20, false)
	_ = base.View()
	result := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m := performanceEvent(base, "answer_delta", performanceText(" **fresh 中文🙂**"))
			m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
			performanceView = m.View()
		}
	})
	if result.AllocedBytesPerOp() > 2<<20 {
		t.Fatalf("stream update allocated %d bytes/op; budget is 2 MiB", result.AllocedBytesPerOp())
	}
	if !strings.Contains(ansi.Strip(performanceView.Content), "fresh 中文🙂") {
		t.Fatal("latest answer missing")
	}
}

func TestMessageLayoutTracksMarkdownRollbackAndResize(t *testing.T) {
	m := NewWithTask("/layout", TaskOptions{TaskID: "layout"})
	defer m.Close()
	m = performanceUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m = performanceEvent(m, "answer_delta", performanceText("**中文🙂"))
	m = performanceUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	previous := m
	before := previous.View().Content
	m = performanceEvent(m, "answer_delta", performanceText(" formatted**"))
	m = performanceUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "中文🙂 formatted") || strings.Contains(view, "**") {
		t.Fatalf("Markdown completion stale: %s", view)
	}
	if previous.View().Content != before {
		t.Fatal("new response changed previous model")
	}
	m = performanceEvent(m, "output_reset", `{"answer_bytes":24}`)
	// Replace the whole response through the protocol, retaining its response slot.
	m = performanceEvent(m, "answer_delta", performanceText("replacement 中文🙂"))
	for _, width := range []int{30, 100, 40, 80} {
		m = performanceUpdate(m, tea.WindowSizeMsg{Width: width, Height: 40})
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "replacement") || strings.Contains(view, "formatted") {
			t.Fatalf("replacement stale at width %d: %s", width, view)
		}
	}
}

func TestEvictedHistoryCanBeReadAgain(t *testing.T) {
	m := NewWithTask("/layout", TaskOptions{TaskID: "layout"})
	defer m.Close()
	m = performanceUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 200})
	for i := 0; i < 150; i++ {
		m = performanceEvent(m, "text", performanceText("historical 中文🙂"))
	}
	for _, width := range []int{80, 60, 100, 80} {
		m = performanceUpdate(m, tea.WindowSizeMsg{Width: width, Height: 200})
		if got := strings.Count(ansi.Strip(m.View().Content), "historical 中文🙂"); got != 150 {
			t.Fatalf("history after eviction: %d responses", got)
		}
	}
}

// Force a presented frame so refresh coalescing cannot hide layout work.
func BenchmarkMessageLayoutStream(b *testing.B) {
	for _, units := range []int{2, 20} {
		b.Run(fmt.Sprintf("History%d", units), func(b *testing.B) {
			base := performanceFixture(b, units, false)
			_ = base.View()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m := performanceEvent(base, "answer_delta", performanceText(" **fresh 中文🙂**"))
				m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
				performanceView = m.View()
			}
		})
	}
}

func TestMessageLayoutFixedHistoryMemory(t *testing.T) {
	if os.Getenv("TUI_PERF_PROFILE_DIR") == "" {
		t.Skip("opt-in retained heap measurement")
	}
	m := performanceFixture(t, 20, false)
	for cycle := 0; cycle <= 100; cycle++ {
		m = performanceEvent(m, "answer_delta", performanceText(" x"))
		m = performanceUpdate(m, tea.WindowSizeMsg{Width: 80 + cycle%21, Height: 40})
		performanceView = m.View()
		m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
		performanceView = m.View()
		if cycle == 5 || cycle == 50 || cycle == 100 {
			runtime.GC()
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			t.Logf("cycle=%d HeapAlloc=%d", cycle, stats.HeapAlloc)
			if stats.HeapAlloc > 4<<20 {
				t.Fatalf("fixed-history heap exceeds 4 MiB: %d", stats.HeapAlloc)
			}
			runtime.KeepAlive(m)
		}
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "baseline-tail") {
		t.Fatal("source tail lost")
	}
}
