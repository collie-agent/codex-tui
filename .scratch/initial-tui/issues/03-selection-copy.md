# 03: 鼠标选区与纯文本复制

**What to build:** 用户用鼠标拖动选择界面文字并看到选区，按 Ctrl+C 将选中的纯文本复制到系统剪贴板，可在其他应用中粘贴。选择覆盖标题、路径、可编辑输入内容和状态栏。

**Blocked by:** 02 — 输入区基础编辑（需要覆盖已可编辑的输入内容）。

**Status:** ready-for-agent

- [x] 应用管理鼠标按下、拖动与释放形成的选区，并以可辨识的视觉样式呈现。
- [x] 可以选择标题、路径、输入内容和状态栏文字，支持正向、反向和跨行选择。
- [x] 中文及宽字符的选择位置与显示位置一致，复制结果不包含损坏字符。
- [x] 有选区时 Ctrl+C 发起系统剪贴板写入，载荷仅包含选中的纯文本，不包含颜色或终端控制序列。
- [x] 无选区时 Ctrl+C 不执行操作、不写剪贴板；有无选区均不因 Ctrl+C 退出。
- [x] Ctrl+D 仍可退出；基础输入和尺寸自适应行为保持可用。
- [x] 沿用应用级测试边界，用鼠标及键盘事件序列验证选区呈现和复制载荷，覆盖反向、跨行、中文、宽字符及无选区情形。
- [x] 自动化验证观察复制请求而不修改真实系统剪贴板，不为内部辅助函数新增独立测试边界。
- [x] 说明选用的剪贴板传输方式及终端或系统依赖，在实际支持的终端中验证复制后可粘贴，并记录环境和结果；仅发出复制指令不能算实际剪贴板验证通过。

## Comments

### 2026-10-29 — Implementation and verification

- Added application-owned left-button click/drag/release selection with a light blue background and dark text. A new click clears selection. Endpoints are inclusive, ordered by screen position, clamped to the viewport, and expanded to complete Unicode graphemes when they intersect wide characters. Highlighting and copying share this range calculation against the rendered rows, including horizontally scrolled input. Editing, moving the caret, pasting, and resizing clear obsolete screen coordinates.
- Ctrl+C with selection returns Bubble Tea's `tea.SetClipboard` command; without selection it has no effect. Ctrl+D still exits. Copy strips ANSI styling, trims trailing layout spaces, and preserves intervening line breaks. No native clipboard dependency or additional module was introduced. Transport is OSC 52 system clipboard selection `c`; the terminal must permit clipboard writes, and intermediate multiplexers must pass them through. The protocol has no write-success acknowledgement.
- `go test ./...`, `go test -race ./...`, and `go vet ./...` passed. Tests remain at the application event/view/effect boundary and cover title/path/input/status in both directions, cross-line and blank-row selection, Chinese, joined emoji, flags, combining marks, both halves of wide characters, scrolled input, bounds clamping, clearing on click/edit/paste/resize, hover after release, repeated copy, no-selection Ctrl+C and Ctrl+D with selection. Command messages are compared with expected clipboard requests without running a renderer or accessing the real clipboard.
- Actual clipboard/paste check: Ubuntu XWayland/X11 (`DISPLAY=:0`), xterm 390, `TERM=xterm-256color`, 100×24, OSC 52 enabled with `allowWindowOps: true`. Ran the built application through a PTY bridge, typed `A中文B`, injected SGR click/motion/release events selecting `中文`, then Ctrl+C. An independent `xclip -selection clipboard -o` returned exactly `中文`. After Ctrl+D, xterm's `insert-selection(CLIPBOARD)` action pasted exactly `中文` into a separate Python receiver. Exit code was 0; alternate-screen exit and exact PTY attribute restoration were verified. This is a real system clipboard and paste check, beyond inspecting the OSC request. Mouse events were injected; graphical appearance was not manually inspected. Temporary validation tools were extracted under `/tmp`, without installing system packages or adding test dependencies.
- Repeatable selection/copy steps, behavior details, terminal dependencies, and validation results are recorded in `README.md`.
