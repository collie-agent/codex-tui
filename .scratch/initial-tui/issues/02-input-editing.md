# 02: 输入区基础编辑

**What to build:** 用户可以在底部输入区输入和删除文字，空值时显示提示；Enter 暂不发送。编辑过程中保持布局和既定退出行为。

**Blocked by:** 01 — 初始全屏布局与退出。

**Status:** ready-for-agent

- [x] 输入区可输入和删除文本，界面即时显示编辑结果。
- [x] 空输入显示 Ask Codex to do anything；有内容时显示输入内容，删除至空后恢复提示。
- [x] 中文及宽字符可正常显示和删除，不出现字符损坏。
- [x] Enter 不发送内容、不触发模型请求或产生回复。
- [x] Ctrl+C 在无选区时不执行操作，Ctrl+D 仍可退出。
- [x] 长输入及窄屏下仍能编辑，输入内容不侵占状态栏或破坏底部布局。
- [x] 沿用应用级事件输入到可见界面与外部效果的测试边界，验证输入、删除、空值提示、Enter、快捷键及窄屏编辑行为。
- [x] 提供可重复的手动编辑演示步骤；不扩展为多行编辑器、发送功能或历史记录。


## Comments

### 2026-10-29 — Implementation and verification

- Added a single-line input component behind the existing application event boundary. Supports typing, bracketed paste, Backspace/Delete, Left/Right and Home/End. Enter remains a no-op; no send, reply, service request or history is introduced. Ctrl+C remains a no-op without selection, and Ctrl+D exits.
- Character movement and deletion use Unicode grapheme boundaries, including Chinese, combining marks, joined emoji and flags. The terminal caret follows the input; horizontal scrolling preserves complete characters and keeps a caret cell available. Resizing retains the full text, including while the screen has zero dimensions. Very narrow screens shorten the prompt to preserve editing space; characters wider than the available cells remain stored until more space is available.
- Pasted line breaks and tabs become spaces, and control characters are removed so input cannot add rows or terminal control sequences.
- `go test ./...` and `go vet ./...` passed. Application-level tests cover typing/deletion, empty-hint restoration, Unicode graphemes, caret edits, Enter/Ctrl+C without effects, Ctrl+D's quit effect, long input, editing during repeated resizes (including 1-column and zero-sized screens), retained text, and bottom layout/status preservation.
- Built the executable and exercised it in a real PTY at 100×24, then resized to 20×10. Verified Unicode typing and emoji Backspace, restoration of the empty hint, Enter/Ctrl+C leaving the process running, bracketed paste and editing the scrolled tail at narrow width, Ctrl+D exit code 0, alternate-screen exit, and exact restoration of terminal attributes. This is terminal-protocol verification; graphical-terminal visual inspection was not performed.
- Repeatable interactive editing demo steps are in `README.md`.
