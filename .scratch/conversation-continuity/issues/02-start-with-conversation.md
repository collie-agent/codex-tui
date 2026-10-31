# 02: 以 --conv-id 启动已有会话

**What to build:** 用户通过 --conv-id 指定已有会话，在空白输入界面开始工作，第一次提交即延续该会话，并对服务端返回归属进行验证。

**Blocked by:** 01 — 实例内连续任务共享会话。

**Status:** ready-for-agent

- [x] 移除 --task-id 启动参数，不保留兼容入口；增加 --conv-id，更新帮助与使用说明。内部任务 ID 仍用于新任务订阅。
- [x] 未提供 --conv-id 或值为空时沿用普通启动行为。
- [x] 指定会话启动后显示空白输入界面与当前会话 ID；不自动创建任务、订阅旧任务或加载历史输出。
- [x] 第一次及后续 POST /tasks 均携带指定的 ConvId 和用户输入。
- [x] 指定的 ID 不等同于服务端确认；首个任务及后续任务均要求 task_info 返回一致的非空 conv_id。
- [x] 缺失确认时完成后阻止继续提交；返回不同会话时保留指定 ID，显示异常并阻止后续提交。
- [x] 指定不存在的会话时沿用现有等待和连接错误处理；入队成功不表示会话有效，不自动改建会话，不增加失败查询或超时推断。
- [x] 在命令行外部边界验证新参数、帮助文本和旧参数拒绝行为；通过 TUI 与本地 HTTP/SSE 验证启动无请求、首次携带 ID、归属验证与异常展示。
- [x] 运行相关测试和静态检查，确保普通启动及内部任务订阅继续可用。

## Notes

依据“实例内会话延续”规格。服务端延续前序运行的结果和错误，不承诺恢复完整聊天历史。

## Comments

- 2026-10-30：已完成。用户确认命令行进程入口与 TUI 消息驱动 + 本地 HTTP/SSE 两个测试边界后，以 TDD 实现。保留原 triage 标签供交付审阅。
- Red → green：`TestConversationCommandLine` 首次运行报 `flag provided but not defined: -conv-id`，替换启动参数后通过；`TestSpecifiedConversationStartsEmptyAndContinues` 首次运行因缺少 `TaskOptions.ConversationID` 编译失败，增加启动身份配置、初始化和 CLI 传递后通过。
- `main_test.go` 在真实二进制边界验证帮助、新参数（含空值）及旧参数拒绝；`internal/tui/start_conversation_test.go` 通过请求、渲染和再次提交行为验证启动无网络动作、首次及后续携带 ConvId、空值普通启动、缺失/无效/冲突确认保护、只有入队响应时继续等待。已有 ticket 01 的逐任务确认与冲突保护直接满足异常用例，无需重复实现。
- README 已更新会话启动、服务连接、Enter 提交、逐任务确认与排队语义。内部 TaskID 订阅支持保留。
- 验证通过：`go test ./...`、`go vet ./...`。
