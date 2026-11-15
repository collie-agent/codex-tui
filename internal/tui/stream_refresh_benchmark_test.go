package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// One operation is a deterministic burst at one frame boundary, followed by
// an input event. No wall-clock sleeps or transport goroutines are measured.
func BenchmarkStreamRefreshBurst(b *testing.B) {
	for _, units := range []int{2, 20} {
		b.Run(fmt.Sprintf("History%d", units), func(b *testing.B) {
			base := streamBenchmarkFixture(b, units)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				m := base
				m.responses = append([]string(nil), base.responses...)
				m.attemptChunks = append([]answerChunk(nil), base.attemptChunks...)
				m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
				b.StartTimer()
				for j := 0; j < 8; j++ {
					m = performanceEvent(m, "answer_delta", `{"text":" x"}`)
					performanceView = m.View()
				}
				m = performanceUpdate(m, tea.PasteMsg{Content: "responsive-input"})
				performanceView = m.View()
				b.StopTimer()
				if !strings.Contains(performanceView.Content, "responsive-input") {
					b.Fatal("input not visible after burst")
				}
				b.StartTimer()
			}
		})
	}
}

// Input latency here covers only the synchronous model boundary. Event-loop
// queueing, network delivery and terminal flushing are deliberately excluded.
func BenchmarkStreamRefreshInput(b *testing.B) {
	for _, units := range []int{2, 20} {
		b.Run(fmt.Sprintf("History%d", units), func(b *testing.B) {
			base := streamBenchmarkFixture(b, units)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				m := base
				m.responses = append([]string(nil), base.responses...)
				m.attemptChunks = append([]answerChunk(nil), base.attemptChunks...)
				m = performanceUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 40})
				for j := 0; j < 8; j++ {
					m = performanceEvent(m, "answer_delta", `{"text":" x"}`)
					performanceView = m.View()
				}
				b.StartTimer()
				m = performanceUpdate(m, tea.PasteMsg{Content: "responsive-input"})
				performanceView = m.View()
				b.StopTimer()
				if !strings.Contains(performanceView.Content, "responsive-input") {
					b.Fatal("input not visible")
				}
				b.StartTimer()
			}
		})
	}
}

// Reuse only untimed immutable history across calibration and repeated runs.
// Every run owns a fresh subscription; each operation copies mutable text slices.
var streamBenchmarkHistory = map[int]Model{}

func streamBenchmarkFixture(b *testing.B, units int) Model {
	base, ok := streamBenchmarkHistory[units]
	if !ok {
		base = performanceFixture(b, units, false)
		streamBenchmarkHistory[units] = base
	}
	base.stream = newSubscription(TaskOptions{TaskID: "baseline"})
	b.Cleanup(base.Close)
	return base
}
