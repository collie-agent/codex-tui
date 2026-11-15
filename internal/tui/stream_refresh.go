package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const streamFrameInterval = time.Second / 30

// A frame memo belongs to one model value. Updates copy it before mutation, so
// rendering a newer model cannot replace an older model's displayed frame.
type streamFrame struct {
	deadline time.Time
	view     *tea.View
}

type streamRefreshMessage struct{ source *subscription }

func (m *Model) forkStreamFrame(msg tea.Msg) {
	if m.streamFrame != nil {
		frame := *m.streamFrame
		m.streamFrame = &frame
	}
	// Interactions must use current content and coordinates, even mid-frame.
	if _, streaming := msg.(streamMessage); !streaming {
		m.streamFrame = nil
	}
}

func (m *Model) prepareStreamFrame(msg streamMessage) {
	if msg.event == nil || (msg.event.Type != "assistant_delta" && msg.event.Type != "answer_delta") {
		m.streamFrame = nil
		return
	}
	if m.stream != nil && m.streamFrame == nil {
		m.streamFrame = &streamFrame{deadline: time.Now().Add(streamFrameInterval)}
	}
}

// Capture only transport and timing: a pending command must not retain a model
// snapshot and its history or mutable display memo.
func (m Model) nextStreamCommand() tea.Cmd {
	s := m.stream
	if m.streamFrame == nil {
		return s.next
	}
	deadline := m.streamFrame.deadline
	return func() tea.Msg { return s.nextFrame(deadline) }
}

// Each subscription command owns one cancellable wait. A frame wake replaces
// that wait; it never starts a second reader or acknowledges an unapplied event.
func (s *subscription) nextFrame(deadline time.Time) tea.Msg {
	if s.ctx.Err() != nil {
		return nil
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return nil
	case <-timer.C:
		if s.ctx.Err() != nil {
			return nil
		}
		return streamRefreshMessage{source: s}
	case msg := <-s.messages:
		return msg
	}
}
