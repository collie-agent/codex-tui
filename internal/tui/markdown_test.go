package tui_test

import (
	"strings"
	"testing"
)

func TestAssistantMarkdownRenderedInView(t *testing.T) {
	m, _ := toolModel(t, event("text", "# Overview\n\n**Important** and `inline`\n\n- item\n\n```go\nfmt.Println(42)\n```"))
	view := visible(m)
	for _, text := range []string{"Overview", "Important", "inline", "item", "fmt.Println(42)"} {
		if !strings.Contains(view, text) {
			t.Fatalf("missing %q: %s", text, view)
		}
	}
	for _, markup := range []string{"# Overview", "**Important**", "`inline`", "```"} {
		if strings.Contains(view, markup) {
			t.Fatalf("raw markdown %q: %s", markup, view)
		}
	}
}

func TestStreamingMarkdownCanBeSelectedAndCopiedAfterResize(t *testing.T) {
	m, _ := toolModel(t, event("answer_delta", "**Hello"), event("answer_delta", " world**"))
	m = resize(t, m, 36, 18)
	lines := strings.Split(visible(m), "\n")
	for y, line := range lines {
		if x := strings.Index(line, "Hello world"); x >= 0 {
			m = drag(t, m, [2]int{x, y}, [2]int{x + 10, y})
			copySelection(t, m, "Hello world")
			return
		}
	}
	t.Fatalf("completed streaming markdown missing: %s", visible(m))
}

func TestMarkdownHeadingsDisplayWithoutSourceMarkers(t *testing.T) {
	m, _ := toolModel(t, event("text", "## Details\n\n### Example\n\n#### Notes"))
	view := visible(m)
	for _, heading := range []string{"Details", "Example", "Notes"} {
		if !strings.Contains(view, heading) {
			t.Fatalf("missing heading %q", heading)
		}
	}
	if strings.Contains(view, "##") {
		t.Fatalf("heading source markers visible: %s", view)
	}
}
