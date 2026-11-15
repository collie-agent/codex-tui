package tui

// Bubble Tea passes Model by value. Fork mutable reading data before an update
// so a retained model (or another agent view) keeps its own content and gestures.
func (m *Model) forkReadingState() {
	m.responses = append([]string(nil), m.responses...)
	m.output = append([]outputItem(nil), m.output...)
	m.attemptChunks = append([]answerChunk(nil), m.attemptChunks...)
	m.agentOrder = append([]string(nil), m.agentOrder...)
	if m.agents != nil {
		agents := make(map[string]Model, len(m.agents))
		for id, agent := range m.agents {
			agents[id] = agent
		}
		m.agents = agents
	}
	tools := make(map[*toolOutput]*toolOutput)
	cloneTool := func(old *toolOutput) *toolOutput {
		if next := tools[old]; next != nil {
			return next
		}
		next := *old
		next.results = append([]string(nil), old.results...)
		tools[old] = &next
		return &next
	}
	regions := make(map[*toolRegion]*toolRegion)
	for i := range m.output {
		old := m.output[i].region
		if old == nil {
			continue
		}
		next := *old
		next.calls = make([]*toolOutput, len(old.calls))
		for j, tool := range old.calls {
			next.calls[j] = cloneTool(tool)
		}
		regions[old] = &next
		m.output[i].region = &next
	}
	if m.tools != nil {
		next := make(map[string]*toolOutput, len(m.tools))
		for id, tool := range m.tools {
			next[id] = cloneTool(tool)
		}
		m.tools = next
	}
	// Cached hit targets must refer to this model's regions after the fork.
	if m.outputCache != nil {
		cache := *m.outputCache
		cache.rows = append([]outputRow(nil), cache.rows...)
		for i := range cache.rows {
			if cache.rows[i].toggle != nil {
				cache.rows[i].toggle = regions[cache.rows[i].toggle]
			}
		}
		m.outputCache = &cache
	}
	if m.toolPress.tool != nil {
		m.toolPress.tool = regions[m.toolPress.tool]
	}
}
