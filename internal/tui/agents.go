package tui

import "encoding/json"

// Both SSE routing and saved history use the same per-instance reading model.
func (m *Model) agentView(id, name string) Model {
	if m.agents == nil {
		m.agents = make(map[string]Model)
	}
	if agent, exists := m.agents[id]; exists {
		return agent
	}
	agent := New(m.directory)
	agent.readOnly = true
	agent.width, agent.height = m.width, m.height
	agent.taskID = name + " · " + id
	agent.connection = "Running"
	m.agentOrder = append(m.agentOrder, id)
	return agent
}

// Instance IDs isolate independent delegations; names are display labels only.
func (m *Model) routeAgentEvent(event taskEvent) bool {
	if event.AgentInstanceID == "" || event.ParentAgentInstanceID == "" || event.Type == "task_info" || event.Type == "task_completed" || event.Type == "task_interrupted" {
		return false
	}
	agent := m.agentView(event.AgentInstanceID, event.AgentName)
	// Clear routing metadata before handing content to the shared conversation view.
	content := event
	content.AgentName, content.AgentInstanceID, content.ParentAgentInstanceID = "", "", ""
	if event.Type == "agent_lifecycle" {
		var lifecycle struct {
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(event.Delta), &lifecycle) == nil {
			agent.connection = safeRemote(lifecycle.Status)
		}
	} else {
		before := agent.render()
		agent.applyEvent(content)
		if before != agent.render() {
			agent.selection = textSelection{}
		}
	}
	m.agents[event.AgentInstanceID] = agent
	return true
}

func (m *Model) switchAgent(reverse bool) {
	index := 0
	for i, name := range m.agentOrder {
		if name == m.activeAgent {
			index = i + 1
			break
		}
	}
	step := 1
	if reverse {
		step = -1
	}
	index = (index + step + len(m.agentOrder) + 1) % (len(m.agentOrder) + 1)
	m.activeAgent = ""
	if index > 0 {
		m.activeAgent = m.agentOrder[index-1]
	}
}
