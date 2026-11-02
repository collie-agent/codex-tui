package tui

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Copy through the desktop clipboard as well as OSC 52: some terminals silently
// discard OSC 52 writes. Run helpers in a command so the UI stays responsive.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		writeSystemClipboard(text)
		return tea.SetClipboard(text)()
	}
}

func writeSystemClipboard(text string) {
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates, []string{"pbcopy"})
	case "linux":
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			candidates = append(candidates, []string{"wl-copy"})
		}
		if os.Getenv("DISPLAY") != "" {
			candidates = append(candidates, []string{"xclip", "-selection", "clipboard"}, []string{"xsel", "--clipboard", "--input"})
		}
	}
	// One deadline bounds all attempts; absent helpers leave OSC 52 available.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, args := range candidates {
		path, err := exec.LookPath(args[0])
		if err != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, path, args[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if cmd.Run() == nil {
			return
		}
	}
}
