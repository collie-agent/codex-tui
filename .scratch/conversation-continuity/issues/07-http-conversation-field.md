# 修复 HTTP 会话字段不匹配导致后续任务无法提交

Status: ready-for-human

## Problem

TUI 使用 `ConvId` 提交会话 ID，但当前 codex-server 的 POST /tasks 读取 `conv_id`。服务端忽略旧字段并新建会话，返回的 task_info 触发 TUI 会话冲突保护，阻止后续 Enter 提交。指定会话启动时第一轮即可触发；普通启动时第二轮触发。

## Resolution

- HTTP 创建请求改用 `conv_id`，首次无会话时继续省略该字段；`ConvId` 属于服务端内部队列协议。
- 保留逐任务归属校验、冲突保护和串行提交。
- 回归测试使用与实际服务端相同的 JSON 请求解码及小写 SSE 事件，验证普通启动和指定会话启动均可连续执行三轮。
- 同步现有会话、中断后继续和重试测试的请求字段断言；早期票据中的 `ConvId` HTTP 字段说明由本票及更新后的规格取代。

## Comments

- 修复前 `go test ./internal/tui -run TestContinuousTasksUseServerConversationProtocol -count=1` 在普通启动第二轮、指定会话启动第一轮均复现 `Protocol error: conversation conflict; submission blocked`。
- 修复后全量测试、`go test -race ./...`、`go vet ./...` 和 `git diff --check` 均通过；已重新构建根目录 `main` 可执行文件。
