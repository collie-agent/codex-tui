# codex-tui

A Bubble Tea v2 terminal interface for Codex tasks.

Run from the directory you want displayed in the header:

```sh
go run github.com/collie-agent/codex-tui
```

When working in this repository, use `go run .`. Requires Go 1.27.3 or later
and an interactive terminal. True color terminals give the closest match to
`prd/init.png`; terminals with fewer colors may approximate the palette.

Press **Ctrl+D** to exit. Drag with the left mouse button to select visible text,
then press **Ctrl+C** to copy it. With no selection, Ctrl+C does nothing. This preview
shows fixed model and quota text. Task requests connect to `http://localhost:8080`:
Enter sends the input to `POST /tasks` and subscribes to the returned task's SSE output.
The input supports single-line typing and pasting, Backspace/Delete, Left/Right
and Home/End. Long input scrolls horizontally with the caret. Pasted line breaks
and tabs become spaces. While a task is being created or running, you can edit
the next draft; Enter submits it after the current task completes and confirms
its conversation. Creation failures retain the input for manual retry.

Tasks within one running instance share a conversation. The first task establishes
the conversation, and the status row shows its ID. To continue an existing
conversation in a new instance:

```sh
go run . --conv-id=YOUR_CONVERSATION_ID
```

This opens an empty input screen without loading history or starting a task.
Every submitted task uses the specified conversation and must receive matching
conversation information from the server. A missing or conflicting confirmation
blocks further submission and displays the reason. An accepted queue request
alone does not confirm the conversation or task completion. The server continues
previous run results and errors; full chat history restoration is not promised.
Omitting `--conv-id`, or passing an empty value, starts a new conversation on the
first submission. Conversation identity is retained only for the current process.
The old `--task-id` startup option is no longer accepted.

Selection covers the title, directory, visible input (including its placeholder),
and status. Drag in either direction or across rows. Both cells of a wide
character select the whole character. Endpoints are inclusive; clicking without
dragging clears the selection. Editing, moving the input caret, pasting, or
resizing also clears it. Copy preserves intervening newlines and omits trailing
layout spaces. Long input copies only the portion currently visible.

Copy uses Bubble Tea's `tea.SetClipboard`, which sends OSC 52 to the terminal's
system clipboard (`c`), without requiring `xclip`, `wl-copy`, or OS bindings.
The terminal must support and permit OSC 52 clipboard writes; SSH and terminal
multiplexers must pass them through. Unsupported or blocked writes may be ignored
silently; emitting the request does not confirm clipboard success.
See the [Bubble Tea clipboard documentation](https://pkg.go.dev/charm.land/bubbletea/v2#SetClipboard).

To repeat the editing demo in an interactive terminal:

1. Run `go run .` and check the empty hint `Ask Codex to do anything`.
2. Type or paste `Hello 中文👩‍💻`. Press Backspace once to delete the whole emoji,
   then delete the remaining text and check that the hint returns.
3. Type `A中B`, press Left, then Backspace, then type `文`: the input becomes
   `A文B`. Try Home, Right, Delete and End to move and edit within the line.
4. Paste a long line, such as `中文 hello ` repeated 20 times. Narrow the terminal
   to about 20 columns, append and delete text, and use Home/End to inspect both
   ends. Widen it again and check the full text is retained. The gray input area
   stays above the status row throughout.
5. With a task server running on port 8080, press Enter to submit a task and
   check that its output appears. Edit the next draft while it runs; repeated
   Enter must not create another task until completion and conversation confirmation.
   Press Ctrl+D to exit and check that the normal terminal is restored.

To check selection and the actual system clipboard in an OSC 52 capable terminal:

1. Run `go run .`, drag over `OpenAI Codex`, and check its selection highlight.
   Press Ctrl+C and paste into another application; expect exactly `OpenAI Codex`.
2. Repeat in reverse, then select from the title into the directory and check
   that the pasted text contains the same rows without colors or escape codes.
3. Type `A中文👩‍💻B`, select Chinese/emoji text (try either half of a wide
   character), copy, and paste elsewhere. Check whole characters and no corruption.
4. Select status text and repeat. Try a long, horizontally scrolled input and
   a narrow terminal; only visible selected text should be copied.
5. Click without dragging and press Ctrl+C. The clipboard should stay unchanged
   and the app should stay running. Repeat with a selection, then Ctrl+D to exit.

Clipboard verification on 2026-10-29: Ubuntu/X11 via XWayland (`DISPLAY=:0`),
xterm 390 (`TERM=xterm-256color`, 100×24, `allowWindowOps: true` to permit OSC 52).
The built application received Unicode input and SGR mouse drag events selecting
`中文`. Ctrl+C updated the real system clipboard (independently read with `xclip`),
and xterm's `insert-selection(CLIPBOARD)` pasted exactly `中文` into a separate
Python receiver after the app exited. Ctrl+D exited with code 0, left the alternate
screen, and restored the app PTY's original terminal attributes. This verifies
real clipboard transfer and paste; mouse events were injected, and graphical
appearance was not manually inspected. The normal automated tests only observe
clipboard request messages and never access the system clipboard.

Run checks with:

```sh
go test ./...
go vet ./...
```
