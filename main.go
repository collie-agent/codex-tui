package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func main() {
	conversationID := flag.String("conv-id", "", "continue an existing conversation without loading history")
	flag.Parse()
	directory, err := os.Getwd()
	if err == nil {
		model := tui.NewWithTask(directory, tui.TaskOptions{ConversationID: *conversationID})
		defer model.Close()
		_, err = tea.NewProgram(model).Run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "codex-tui:", err)
		os.Exit(1)
	}
}
