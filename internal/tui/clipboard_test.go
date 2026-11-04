package tui_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/collie-agent/codex-tui/internal/tui"
)

func TestCtrlCCopiesSelectionToSystemClipboard(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux clipboard integration")
	}
	dir := t.TempDir()
	destination := filepath.Join(dir, "clipboard")
	// A controlled clipboard executable exercises the actual Ctrl+C command
	// without changing the developer's clipboard or requiring a display server.
	script := "#!/bin/sh\n[ \"$1 $2\" = \"-selection clipboard\" ] || exit 1\n/bin/cat > \"$COPY_TEST_DESTINATION\"\n"
	if err := os.WriteFile(filepath.Join(dir, "xclip"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("DISPLAY", ":test")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("COPY_TEST_DESTINATION", destination)
	model := resize(t, tui.New("/项目"), 80, 10)
	model = edit(t, model, tea.PasteMsg{Content: "/项目"})
	model = drag(t, model, [2]int{4, 7}, [2]int{8, 7})
	_, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("missing copy command")
	}
	cmd()
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("Ctrl+C did not reach system clipboard: %v", err)
	}
	if string(got) != "/项目" {
		t.Fatalf("clipboard = %q", got)
	}
}
