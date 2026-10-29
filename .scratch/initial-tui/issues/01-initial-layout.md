# 01: 初始全屏布局与退出

**What to build:** 用户启动 Bubble Tea 应用即可看到与已确认参考图一致的初始界面：顶部标题与实际工作目录、中间留白、底部灰色输入区和彩色状态栏。布局适应终端尺寸，Ctrl+D 可退出。

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [x] 提供可运行的启动方式，使用 Bubble Tea 实现全屏界面。
- [x] 显示固定标题 OpenAI Codex、版本 v0.165.0 及实际工作目录。
- [x] 按参考图呈现英文文案、深灰背景、灰色输入区和彩色状态栏；中间保留弹性留白。
- [x] 底部输入区显示 Ask Codex to do anything，状态栏固定展示 GPT-6.1 Terra medium、0 in、weekly 88% left、5h 100% left。
- [x] 窗口缩放后输入区与状态栏仍固定在底部；窄屏优先保留输入区，过长状态信息截断，不破坏布局。
- [x] Ctrl+D 退出并恢复终端；Ctrl+C 在尚无选区时不退出、不修改剪贴板。
- [x] 不连接模型服务或查询真实额度。
- [x] 从应用级事件输入验证初始界面、尺寸变化、窄屏截断和退出效果，不绑定内部字段或完整 ANSI 字节。
- [ ] 在实际终端对照参考图核对布局、配色和间距，并验证退出后的终端恢复；记录验证结果。


## Comments

### 2026-10-29 — Implementation and verification

- Added `main.go` and `internal/tui`: Bubble Tea v2 alternate screen, actual working directory, reference palette, flexible blank space, three-row input area and fixed status text. Very short screens reserve the input area first; all rows truncate by terminal cell width.
- Run with `go run .`; startup and verification instructions are in `README.md`.
- `go test ./...` and `go vet ./...` passed. Tests exercise the application event boundary for initial content, repeated resizes, wide-character paths, narrow/short/zero-sized screens, Ctrl+C with no external effect, and Ctrl+D's quit effect.
- Ran the built executable in an actual 120×24 PTY with `TERM=xterm-256color` and `COLORTERM=truecolor`. Confirmed initial title/input rendering, Ctrl+C keeps the process running, Ctrl+D exits with code 0, alternate screen entry/exit, and exact restoration of terminal attributes.
- Read `prd/init.png` and matched its dark gray (#303030), gray input (#484848), blue prompt, yellow model and pink usage text, top spacing and bottom placement in the renderer. This environment has no graphical terminal for human visual comparison; the final manual visual acceptance item remains unchecked. To finish it, run `go run .` in a graphical terminal, compare spacing/colors with the reference, resize the window, and exit with Ctrl+D.
