package tui

import "testing"

func TestConversationUsageTotals(t *testing.T) {
	m := New(".")
	m.applyEvent(taskEvent{Type: "task_info", Delta: `{"conv_id":"conv","model_name":"model-a"}`})
	m.applyEvent(taskEvent{Type: "usage", Delta: `{"input_tokens":"20","cached_input_tokens":"4","output_tokens":"5"}`})
	m.applyEvent(taskEvent{Type: "task_completed", Delta: `{}`})
	m.applyEvent(taskEvent{Type: "task_info", Delta: `{"conv_id":"conv","model_name":"model-b"}`})
	m.applyEvent(taskEvent{Type: "usage", Delta: `{"input_tokens":"30","cached_input_tokens":"6","output_tokens":"7"}`})
	if got := m.usage.label(); got != "0K in · 0K cache in · 0K out" {
		t.Fatal(got)
	}
	if m.modelName != "model-b" {
		t.Fatal(m.modelName)
	}
	m.applyEvent(taskEvent{Type: "usage", Delta: `{"input_tokens":"unknown","cached_input_tokens":"0","output_tokens":"1"}`})
	if got := m.usage.label(); got != "0K cache in · 0K out" {
		t.Fatal(got)
	}
}

func TestUsageKDisplay(t *testing.T) {
	u := tokenUsage{Input: "12345", Cached: "2500", Output: "1000"}
	if got := u.label(); got != "12K in · 2K cache in · 1K out" {
		t.Fatal(got)
	}
}

func TestUsageMissingMetrics(t *testing.T) {
	var u tokenUsage
	if u.label() != "" {
		t.Fatal(u.label())
	}
	u.add(tokenUsage{Input: "0"})
	if u.label() != "0K in" {
		t.Fatal(u.label())
	}
	u.add(tokenUsage{Output: "2000"})
	if u.label() != "0K in · 2K out" {
		t.Fatal(u.label())
	}
}
