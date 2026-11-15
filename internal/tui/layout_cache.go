package tui

import "slices"

// One layout belongs to one reading state and width. It is derived data only:
// replacement releases the previous layout; content and history are never evicted.
type outputLayoutCache struct {
	messages  []messageLayoutEntry
	width     int
	valid     bool
	rows      []outputRow
	responses []string
	expanded  []bool
}

func (m *Model) invalidateOutputLayout() {
	next := &outputLayoutCache{}
	if m.outputCache != nil {
		next.messages = m.outputCache.messages
	}
	m.outputCache = next
}

func (m Model) outputLayout() []outputRow {
	cache := m.outputCache
	if cache == nil {
		return m.buildOutputLayout()
	}
	valid := cache.valid && cache.width == m.contentWidth() && slices.Equal(cache.responses, m.responses) && len(cache.expanded) == len(m.output)
	if valid {
		for i, item := range m.output {
			if item.region != nil && item.region.expanded != cache.expanded[i] {
				valid = false
				break
			}
		}
	}
	if valid {
		return cache.rows
	}
	cache.rows = m.buildOutputLayout()
	cache.width, cache.valid = m.contentWidth(), true
	cache.responses = append([]string(nil), m.responses...)
	cache.expanded = make([]bool, len(m.output))
	for i, item := range m.output {
		if item.region != nil {
			cache.expanded[i] = item.region.expanded
		}
	}
	return cache.rows
}
