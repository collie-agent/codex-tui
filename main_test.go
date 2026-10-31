package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationCommandLine(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "codex-tui")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	for _, args := range [][]string{{"--help"}, {"--conv-id=", "--help"}, {"--conv-id=existing-conversation", "--help"}} {
		output, err := exec.Command(binary, args...).CombinedOutput()
		if err != nil || !strings.Contains(string(output), "-conv-id") || strings.Contains(string(output), "-task-id") {
			t.Fatalf("conversation flag/help %v: %v\n%s", args, err, output)
		}
	}
	output, err := exec.Command(binary, "--task-id=old-task").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "flag provided but not defined: -task-id") {
		t.Fatalf("legacy task flag accepted: %v\n%s", err, output)
	}
}
