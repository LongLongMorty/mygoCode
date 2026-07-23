# OpenCode 借鉴改进实施规格（轻量版）

> 状态：In Progress（P0/P1 首版已实现；P2 经评估后不实施）
>
> 来源：[OPENCODE_借鉴与改进建议.md](./OPENCODE_借鉴与改进建议.md)
>
> 目标：将现有 Agent 和任务进度能力整理为清晰、可演示的工作流。

当前实现：`internal/agent/profile.go` 定义 `build`、`plan`、`review` 档案；`internal/agents/summary.go` 提供并发安全的任务快照与只读汇总。当前命令行验证：`go test ./cmd/mygocode -run TestHandleProjectTrustFlag`。P0/P1 的完整模块回归完成前，不将其标记为正式完成。

## 1. 范围与原则

本规格只包含两个增量功能：主 Agent 档案和只读任务汇总。复用现有 Agent 引擎、权限检查、团队调度、指令加载和会话机制，不重写它们。

- 不实现桌面端、云端分享、账号登录、HTTP SDK 或完整 OpenCode 协议。
- 不增加递归任务树、新调度算法或新模型目录。
- 不开放 `.mygocode/agents/*.md` 作为自定义 Agent 配置，先保证三个内置档案稳定。
- 不实施 `mygocode run` 非交互执行入口；未来有明确脚本或 CI 需求时另行评估。

## 2. 优先级与状态

| ID | 功能 | 状态 | 主要代码边界 |
| --- | --- | --- | --- |
| P0 | `build / plan / review` 主 Agent 档案与 `/agent` 切换 | 首版已实现，待完整回归 | `internal/agent/`、`internal/permissions/`、`internal/tui/`、`internal/commands/` |
| P1 | 只读 `/tasks` 子任务汇总 | 首版已实现，待完整回归 | `internal/agents/`、`internal/teams/`、`internal/tui/`、`internal/commands/` |
| P2 | `mygocode run` 非交互执行 | 不实施 | 已移除参数解析脚手架和测试 |

执行顺序为 P0 → P1 → 文档收尾。每一阶段均保持既有 `/plan`、权限模式、团队工具和 TUI 行为可用。

## 3. P0：主 Agent 档案

### 行为要求

- 内置 `build`、`plan`、`review` 三个档案。
- `/agent` 无参数列出档案；`/agent <name>` 仅在没有运行中的请求时切换。
- 切换只更新 system prompt、可见工具和权限模式，不清空会话、不新建会话。
- `plan` 和 `review` 不向模型暴露 `WriteFile`、`EditFile` 或危险 `Bash` 操作；限制同时由工具过滤和现有权限检查落实。
- 未知档案或流式执行中切换返回明确错误，并保持原状态。

### 验收与测试

- 新启动默认 `build`，普通编辑流程保持不变。
- 每个档案的权限模式、提示词和工具过滤均有单元测试。
- 覆盖 `/agent` 列表、合法切换、未知档案和执行中拒绝切换。
- 切换后 conversation、session ID 和 token 统计保持不变。

## 4. P1：只读 `/tasks` 汇总

### 行为要求

- `/tasks` 只读显示 `TaskManager` 和 `TeamManager` 的任务及进度，不创建、更新或取消任务。
- 汇总至少显示名称、来源、状态、最近活动、工具调用数和耗时；缺失字段显示 `-`，不猜测数据。
- 无任务时显示清晰空状态；运行、完成、失败和取消会在下次查看时体现。
- 长名称和活动描述会被截断，避免破坏 TUI 布局。
- 不展示任务输出、完整工具参数或模型回复，避免泄露不必要上下文。

### 验收与测试

- 空列表、运行中、完成、失败、取消和多个团队成员均产生稳定文本输出。
- TaskManager 的并发快照不会 panic、产生数据竞争或读取半条记录。
- 调用 `/tasks` 前后任务数量、状态、取消语句和持久化 todo 数据均不变。
- 既有 TUI 实时进度块继续工作。

## 5. P2 取消记录

`mygocode run` 非交互执行的收益不足以抵消权限边界、超时控制、输出协议和会话隔离带来的额外维护成本。因此 P2 不在当前项目范围内，已删除仅用于参数解析的实现与测试。README 和本规格不得将该能力描述为已实现或待交付。

## 6. 测试矩阵

| 用例 ID | 类型 | 场景 | 预期结果 |
| --- | --- | --- | --- |
| PROFILE-UT-01 | 单元 | 默认、build、plan、review 映射 | 档案、提示词、权限和工具过滤一致 |
| PROFILE-UT-02 | 单元 | 未知档案、运行中切换 | 返回错误且原状态不变 |
| PROFILE-IT-01 | 集成 | 切换档案后查看 status/继续对话 | 会话与统计保留，状态显示新档案 |
| TASKS-UT-01 | 单元 | 空任务、各状态、长活动文本 | 输出稳定、截断安全、空状态可读 |
| TASKS-UT-02 | 单元 | 并发更新 TaskManager/TeammateProgress | 快照自洽且无数据竞争 |
| TASKS-IT-01 | 集成 | `/tasks` 前后创建、更新、取消任务 | 汇总只读，任务和 todo 数据不变 |

推荐命令：

```bash
go test ./internal/agent ./internal/agents ./internal/teams ./internal/commands
go test ./cmd/mygocode
go test ./...
```

基线记录（2026-07-23）：`internal/agents`、`internal/teams`、`internal/commands` 测试通过；`internal/memory` 当前有既有 Windows 路径断言和项目内存目录残留文件问题。既有编译错误、超时或平台运行时错误须单独登记，不能归因于本组功能。

## 7. 文档规则

- 未通过对应测试前，README 只标记为进行中并链接本规格。
- 功能完成后才将本文件状态更新为 `Implemented`，并补充实际实现文件与测试命令。
- 建议文档保留借鉴理由；本文件只描述当前范围、验收和测试。
