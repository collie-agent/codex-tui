package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestUsageSeparatesModelAttemptsWithoutCompletingTask(t *testing.T) {
	for _, parent := range []string{"", "root"} {
		for _, nextKind := range []string{"answer_delta", "text"} {
			t.Run(parent+"/"+nextKind, func(t *testing.T) {
				m, _ := toolModel(t,
					agentFrame("worker", parent, "answer_delta", `{"text":"first "}`),
					agentFrame("worker", parent, "answer_delta", `{"text":"attempt"}`),
					agentFrame("worker", parent, "usage", `{"input_tokens":"1000","output_tokens":"2000"}`),
					agentFrame("worker", parent, nextKind, `{"text":"second attempt"}`),
				)
				if parent != "" {
					m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
				}
				view := visible(m)
				lines := strings.Split(view, "\n")
				for i := range lines {
					lines[i] = strings.TrimSpace(lines[i])
				}
				text := strings.Join(lines, "\n")
				for _, want := range []string{"\nfirst attempt\n", "\nsecond attempt\n", "1K in", "2K out"} {
					if !strings.Contains(text, want) {
						t.Fatalf("missing %q after attempt boundary: %s", want, view)
					}
				}
				if strings.Contains(view, "Completed") {
					t.Fatal("usage incorrectly completed task")
				}
			})
		}
	}
}
