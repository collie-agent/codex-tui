package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TaskOptions supplies the subscription boundary; an empty server uses localhost.
type TaskOptions struct {
	TaskID, ServerURL string
	Client            *http.Client
}

// normalized keeps creation and subscription on the same connection defaults.
func (o TaskOptions) normalized() TaskOptions {
	o.ServerURL = strings.TrimRight(o.ServerURL, "/")
	if o.ServerURL == "" {
		o.ServerURL = "http://localhost:8080"
	}
	if o.Client == nil {
		o.Client = http.DefaultClient
	}
	return o
}
func newSubscription(options TaskOptions) *subscription {
	ctx, cancel := context.WithCancel(context.Background())
	return &subscription{ctx: ctx, cancel: cancel, messages: make(chan streamMessage), options: options.normalized()}
}

type streamMessage struct {
	source    *subscription
	connected bool
	event     *taskEvent
	err       error
}
type taskEvent struct{ ID, Type, Delta string }
type subscription struct {
	mu       sync.Mutex
	cursor   string
	seen     map[string]bool
	ack      chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	messages chan streamMessage
	options  TaskOptions
}

func NewWithTask(directory string, options TaskOptions) Model {
	m := New(directory)
	m.options = options
	if options.TaskID == "" {
		return m
	}
	m.stream = newSubscription(options)
	m.taskID = options.TaskID
	m.connection = "Connecting"
	return m
}

// Close also covers program termination outside the Ctrl+D update path.
func (m Model) Close() {
	if m.create != nil {
		m.create.cancel()
	}
	if m.stream != nil {
		m.stream.cancel()
	}
}
func (s *subscription) next() tea.Msg {
	select {
	case msg := <-s.messages:
		return msg
	case <-s.ctx.Done():
		return nil
	}
}
func (s *subscription) send(msg streamMessage) bool {
	msg.source = s
	select {
	case s.messages <- msg:
		return true
	case <-s.ctx.Done():
		return false
	}
}

// acknowledge advances the resume cursor only after the UI applies the event.
func (s *subscription) acknowledge(event taskEvent) {
	s.mu.Lock()
	if event.ID != "" {
		s.cursor = event.ID
		if s.seen == nil {
			s.seen = make(map[string]bool)
		}
		s.seen[event.ID] = true
	}
	ack := s.ack
	s.mu.Unlock()
	if ack != nil {
		select {
		case ack <- struct{}{}:
		default:
		}
	}
}
func (s *subscription) run() {
	s.ack = make(chan struct{}, 1)
	delay := 200 * time.Millisecond
	for s.ctx.Err() == nil {
		err := s.read()
		if err == nil || s.ctx.Err() != nil {
			return
		}
		if !s.send(streamMessage{err: err}) {
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 3*time.Second)
	}
}
func (s *subscription) read() error {
	options := s.options.normalized()
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, options.ServerURL+"/tasks/"+url.PathEscape(s.options.TaskID)+"/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	s.mu.Lock()
	cursor := s.cursor
	s.mu.Unlock()
	if cursor != "" {
		req.Header.Set("Last-Event-ID", cursor)
	}
	resp, err := options.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	if !s.send(streamMessage{connected: true}) {
		return nil
	}
	// SSE frames end at blank lines, independent of HTTP read boundaries.
	reader := bufio.NewReader(resp.Body)
	skipLF := false
	readLine := func() (string, error) {
		var line strings.Builder
		for {
			b, err := reader.ReadByte()
			if err != nil {
				return line.String(), err
			}
			if skipLF {
				skipLF = false
				if b == '\n' {
					continue
				}
			}
			if b == '\r' {
				skipLF = true
				return line.String(), nil
			}
			if b == '\n' {
				return line.String(), nil
			}
			line.WriteByte(b)
		}
	}
	var data []string
	dispatch := func() (bool, error) {
		if len(data) == 0 {
			return false, nil
		}
		var event taskEvent
		if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &event); err != nil {
			return false, fmt.Errorf("invalid event: %w", err)
		}
		if !json.Valid([]byte(event.Delta)) {
			return false, fmt.Errorf("invalid event Delta")
		}
		if event.Type == "text" || event.Type == "text_delta" {
			var delta struct{ Text string }
			if err := json.Unmarshal([]byte(event.Delta), &delta); err != nil {
				return false, fmt.Errorf("invalid text Delta: %w", err)
			}
		}
		s.mu.Lock()
		duplicate := event.ID != "" && s.seen[event.ID]
		s.mu.Unlock()
		if duplicate {
			return false, nil
		}
		if !s.send(streamMessage{event: &event}) {
			return true, nil
		}
		// Wait for the model, so a reconnect cannot race ahead of applied events.
		select {
		case <-s.ack:
		case <-s.ctx.Done():
			return true, nil
		}
		return event.Type == "task_completed", nil
	}
	for {
		line, err := readLine()
		// CR, LF and CRLF all delimit SSE lines, including split CRLF pairs.
		if err == nil {
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if line == "" {
				done, e := dispatch()
				data = nil
				if done || e != nil {
					return e
				}
			} else if !strings.HasPrefix(line, ":") {
				field, value, _ := strings.Cut(line, ":")
				value = strings.TrimPrefix(value, " ")
				if field == "data" {
					data = append(data, value)
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("connection closed; completion unconfirmed")
			}
			return err
		}
	}
}
