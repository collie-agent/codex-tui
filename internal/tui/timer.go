package tui

import (
	"fmt"
	"time"
)

func (m *Model) startTimer() {
	m.startedAt = time.Now()
	m.timerNow = m.startedAt
}

func elapsedLabel(elapsed time.Duration) string {
	seconds := max(0, int(elapsed/time.Second))
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
}

func (m *Model) finishTimer(success bool) {
	if m.startedAt.IsZero() || m.completed {
		return
	}
	finished := time.Now()
	label := "Worked for "
	if !success {
		label = "Interrupted after "
	}
	// Store the final label in this run's output so later tasks cannot change it.
	m.appendResponse("\n" + label + elapsedLabel(finished.Sub(m.startedAt)) + " • " + finished.Format("3:04 PM"))
}
