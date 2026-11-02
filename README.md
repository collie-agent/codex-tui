# codex-tui

A Bubble Tea v2 terminal interface for Codex tasks.

## Features

- **Live task output:** submit tasks to the local task server and read assistant
  responses and tool activity over SSE. Interrupted connections reconnect and
  events with previously received IDs are skipped to avoid duplicate output.
- **Conversation continuity:** keep subsequent tasks in the same conversation,
  or load saved runs with `--conv-id` before continuing the conversation.
- **Interruption and drafts:** press Esc to request interruption and prepare the
  next input while a task is running. Submit the draft once the current task has
  finished and the server has confirmed its conversation.
- **Task timing:** a live `Working (7s • esc to interrupt)` indicator updates
  every second, with a blank row above and below. Completed tasks retain
  `Worked for 3m 3s • 11:25 AM` in their output.
- **Expandable tool calls:** consecutive calls form a tool call region, grouped
  by tool name in the collapsed view. Click `+ expand` to inspect each call's
  arguments, status and results; click `− collapse` to return to summaries.
- **Output navigation:** scroll with the mouse wheel or PgUp/PgDn, return to the
  latest output with the return-to-bottom button, and click a pinned task goal
  to jump back to its original text. Task goals appear as full-width gray cards.
- **Unicode editing and copying:** edit Chinese text, combining characters and
  emoji without splitting characters; select visible text with the mouse and
  copy through desktop clipboard helpers or OSC 52.
- **Model and token usage:** the status row shows the conversation ID, the model
  name reported by the server, and accumulated input, cached input and output
  token counts received during the current instance. Counts are displayed in
  whole thousands (`K`, rounded down); unknown counts are omitted. Loading saved
  runs does not reconstruct historical usage totals.

## Quick start

Run from the directory you want displayed in the header:

```sh
go run github.com/collie-agent/codex-tui
```

When working in this repository, use `go run .`. Requires Go 1.27.3 or later
and an interactive terminal. True color terminals give the closest match to
`prd/init.png`; terminals with fewer colors may approximate the palette.

Press **Ctrl+D** to exit. Drag with the left mouse button to select visible text,
then press **Ctrl+C** to copy it. With no selection, Ctrl+C does nothing.
Task requests require a running task server at `http://localhost:8080`:
Enter sends the input to `POST /tasks` and subscribes to the returned task's SSE output.
The input supports single-line typing and pasting, Backspace/Delete, Left/Right
and Home/End. Long input scrolls horizontally with the caret. Pasted line breaks
and tabs become spaces. While a task is being created or running, you can edit
the next draft; Enter submits it after the current task completes and confirms
its conversation. Creation failures retain the input for manual retry.

Once a task has started, press **Esc** to request interruption, even while editing
a draft or reading earlier output. The request being accepted does not mean the
run has stopped: wait for the Interrupted confirmation before pressing Enter to
submit the draft. Continuing creates a new run in the same conversation; it does
not resume the original execution position. Interruption does not undo operations
already performed by tools.

Tasks within one running instance share a conversation. The first task establishes
the conversation, and the status row shows its ID. To continue an existing
conversation in a new instance:

```sh
go run . --conv-id=YOUR_CONVERSATION_ID
```

This loads all saved agent runs in the conversation, including goals, results,
errors and expandable tool calls, without starting or resuming a task.
Loading failures appear in the task header and preserve your input draft.
Running runs show their persisted snapshot; this does not subscribe to live updates.
Every submitted task uses the specified conversation and must receive matching
conversation information from the server. A missing or conflicting confirmation
blocks further submission and displays the reason. An accepted queue request
alone does not confirm the conversation or task completion. The server continues
previous run results and errors; the displayed snapshot contains only persisted data.
Omitting `--conv-id`, or passing an empty value, starts a new conversation on the
first submission. Conversation identity is retained only for the current process.
The old `--task-id` startup option is no longer accepted.

## Controls

| Key / action | Behavior |
| --- | --- |
| Enter | Submit the input when no task is active and conversation confirmation permits it. |
| Esc | Request interruption of the current task; when no task is active, return to the bottom if the return button is visible. |
| Ctrl+D | Exit the TUI. |
| Left / Right, Home / End | Move the input caret. |
| Backspace / Delete | Delete the previous / next whole character. |
| Mouse wheel, PgUp / PgDn | Scroll task output and pause automatic following. |
| Click return-to-bottom | Resume following the latest output. |
| Click pinned task goal | Jump to the original goal and pause following. |
| Click `+ expand` / `− collapse` | Toggle tool call details. |
| Left-button drag, then Ctrl+C | Select and copy visible text. |

## Selection and clipboard

Selection covers the title, directory, visible input (including its placeholder),
and status. Drag in either direction or across rows. Both cells of a wide
character select the whole character. Endpoints are inclusive; clicking without
dragging clears the selection. Editing, moving the input caret, pasting, or
resizing also clears it. Copy preserves intervening newlines and omits trailing
layout spaces. Long input copies only the portion currently visible.

To copy, drag with the mouse until the text is highlighted, then press Ctrl+C.
The terminal must deliver mouse drag events and Ctrl+C to the application.
Clicking without dragging does not create a selection.

Copy requires at least one working clipboard path:

| Environment | Required tool and conditions |
| --- | --- |
| Linux / Wayland | `wl-copy` from `wl-clipboard` must be on `PATH`; `WAYLAND_DISPLAY` must be set and the process must have access to that compositor session. |
| Linux / X11 or XWayland | `xclip` or `xsel` must be on `PATH`; `DISPLAY` must be set and the process must have access to that X server. The app writes the CLIPBOARD selection. |
| macOS | The system `pbcopy` command must be on `PATH` and have access to the desktop clipboard. |
| Terminal clipboard (OSC 52) | The terminal must support and permit OSC 52 clipboard writes. SSH and terminal multiplexers such as tmux must pass these sequences through. This is also the available path on platforms without a supported desktop helper. |

The app tries `wl-copy` first when `WAYLAND_DISPLAY` is set, then `xclip` and
`xsel` when `DISPLAY` is set. On macOS it tries `pbcopy`. Desktop helper attempts
run asynchronously with a shared two-second timeout. The app also sends an
OSC 52 request after those attempts, even if a desktop helper succeeded.
Neither installing a helper without access to its display session nor setting
a display variable alone is sufficient.

For Debian/Ubuntu, install the helper matching your desktop session:

```sh
sudo apt install wl-clipboard  # Wayland
sudo apt install xclip         # X11 / XWayland; xsel is another option
```

If copying fails, check that the selection is highlighted, that the matching
helper is available on `PATH`, and that the app can access the desktop session.
For SSH, a desktop helper runs on the remote machine; copying to your local
clipboard requires OSC 52 support in your local terminal and passthrough along
the connection. A terminal can silently ignore OSC 52, so sending the request
does not confirm that the clipboard changed. Verify by pasting into another
application.

## Manual verification

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

To check selection and the actual system clipboard with one of the paths above:

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
appearance was not manually inspected. Automated tests check OSC 52 request messages and exercise the desktop-helper
path with a controlled executable; they do not access the real system clipboard.

## Development checks

Run checks with:

```sh
go test ./...
go vet ./...
```

## Reading task output

Use the mouse wheel to scroll task output, or PgUp/PgDn to move by a page with
one row of overlap. Scrolling pauses automatic following; use the return-to-bottom
button to resume. Once a run's goal scrolls out of view, it stays in a gray bar
below the task status. The bar follows the run currently being read, including
historical runs, and supports text selection and copying. Long goals are shortened
to fit one row; click the pinned goal to jump to its original text and pause
automatic following. Dragging over the pinned goal still selects text for copying.

Submitted task goals appear as full-width gray cards with a `›` prefix, light
text, and one blank row above and below. Long goals wrap within the text margins.
Saved run goals use the same style, with run metadata shown separately.

The timer starts when task creation succeeds and the app begins subscribing to
output. Each task starts a new timer. Completion records show elapsed whole
seconds and the local completion time; interrupted tasks show `Interrupted after`
instead. Connection errors do not stop the timer or confirm completion. Saved run
snapshots do not reconstruct historical timing.

While reading earlier output, the return-to-bottom button replaces the live timer.
Very short terminals omit the live indicator to preserve room for task output.
