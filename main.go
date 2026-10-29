package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func main() {
	directory, err := os.Getwd()
	if err == nil {
		_, err = tea.NewProgram(tui.New(directory)).Run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "codex-tui:", err)
		os.Exit(1)
	}
}
