# 从 OpenCode 借鉴 MygoCode 的轻量改进建议

> 目的：把 MygoCode 打磨成适合日常实习展示的项目，而不是复刻 OpenCode。本文基于 2026-07-22 对 [OpenCode](https://github.com/anomalyco/opencode) 的 README、Agents、Permissions、Commands、Share 与 Config 公开文档，以及本仓库源码的对照。

## 结论

OpenCode 的优势不只是 Agent 功能本身，而是把能力组织成容易理解的使用路径：用户能一眼区分 Build / Plan Agent，能看到子任务的进展，也能在命令行中把 Agent 接进脚本。

MygoCode 已经具备不少底座：Plan Mode、多层权限、可配置子 Agent、团队协作进度、文件快照和 `/rewind`、TUI 与远程 UI。因此不要再加一套多 Agent 或回滚系统；优先把已有能力“产品化”成下列三个小功能。

## 建议一：提供可切换的主 Agent 档案

**优先级：P0；预计 1 天。**

### 借鉴 OpenCode

OpenCode 将默认主 Agent 区分为 **build**（可修改代码）和 **plan**（只读分析），并允许用户切换。子 Agent 也有 General、Explore、Scout 等清晰的职责说明。

### 当前基础

MygoCode 已有 Plan Mode，以及 `internal/agents/definition.go` 中的 Agent 定义、模型、工具白名单、权限模式和 System Prompt 字段；但这些主要服务于子 Agent。主对话只有 Provider 选择和 `/plan`，用户很难理解“当前 Agent 在做什么”。

### 最小实现

只内置三个主 Agent 档案，并通过 `/agent` 切换：

| 档案 | 权限与提示词 | 使用场景 |
| --- | --- | --- |
| `build` | 默认工具与默认权限 | 正常修改、测试、修复代码 |
| `plan` | 复用现有 Plan Mode，只读 | 阅读陌生仓库、制定方案 |
| `review` | 只读工具 + 代码审查提示词 | 审查当前 `git diff` |

实现时不要新建 Agent 引擎：切换时替换 System Prompt、工具过滤器和 Permission Mode，并在状态栏与 `/status` 显示当前档案即可。后续可允许 `.mygocode/agents/*.md` 增加自定义档案。

### 面试可讲的点

“同一个模型在不同工作阶段需要不同最小权限。将提示词、工具可见性和权限模式封装为档案，能降低误操作，也让工作流更可解释。”

## 建议二：增加只读的 `/tasks` 子任务面板

**优先级：P1；预计 0.5～1 天。**

### 借鉴 OpenCode

OpenCode 允许从主会话导航到子 Agent 会话，并展示子任务的层级和执行状态。它的重点是让用户知道“谁在做什么”，而不是把并发隐藏在后台。

### 当前基础

MygoCode 已有 `TaskManager`、TeamManager、子 Agent 进度通道，以及 `TeammateProgress` 中的状态、工具次数、Token 数、最近操作和耗时。TUI 也会显示当前子 Agent 的活动块，但缺少一个可随时查看的汇总入口。

### 最小实现

增加 `/tasks`，只读展示，不先做会话跳转或复杂调度：

```text
Tasks
ID        Agent       Status      Last activity              Elapsed
task-01   explore     running     Searching internal/llm     18s
task-02   reviewer    completed   Reviewed git diff          42s
```

要求：

1. 汇总主 Agent 创建的任务和团队成员；没有任务时明确显示空状态。
2. 每项仅显示任务 ID、名称/类型、状态、最近一次工具活动、耗时、工具调用数。
3. 状态变化和取消场景增加单元测试；不要在这一阶段实现递归子任务树。

这会把已有的多 Agent 实现从“源码中存在”变成“用户能看到并讲清楚”。

## 建议三：增加非交互式单次执行命令

**优先级：P2；预计 1～2 天。**

### 借鉴 OpenCode

OpenCode 除 TUI 外提供面向脚本的运行方式，便于在终端、CI 或演示中复用 Agent 能力。

### 最小实现

增加一个严格受限的入口，例如：

```bash
mygocode run --agent review "审查当前 git diff"
mygocode run --agent plan --format json "分析这个仓库的认证流程"
```

范围控制如下：

1. 复用现有配置、Agent 主循环和权限检查，不复制一套执行逻辑。
2. 默认使用 `plan` 档案；指定 `build` 时仍走现有人机确认，不添加绕过权限选项。
3. 默认输出纯文本；`--format json` 仅输出最终文本、token 使用量、耗时与退出状态，方便脚本处理。
4. 增加成功、模型错误、权限拒绝三类测试。

这比开发桌面端或公开云服务小得多，但能展示 CLI 参数设计、流式事件收敛和自动化边界。

## 已有能力，应该展示而非重做

| 能力 | 本项目现状 | 建议 |
| --- | --- | --- |
| 只读规划 | 已有 `/plan` 和权限模式 | 作为 `plan` Agent 档案复用。 |
| 文件撤销 | 已有文件快照和 `/rewind` | 在 README 增加一段实际演示，不再开发 `/undo`。 |
| 子 Agent | 已有定义、任务管理、团队与 Worktree | 先做 `/tasks` 可视化，不做新的调度器。 |
| 自定义命令 | 已支持 Markdown commands 与 Skills | 提供一个 review 命令样例即可，无需插件市场。 |

## 不建议现在做

| OpenCode 的方向 | 不做的原因 |
| --- | --- |
| 桌面应用、多端同步、公开分享链接 | 需要账户、存储、脱敏和访问控制，超出实习项目的合理范围。 |
| 完整 Web Server / SDK / HTTP API 生态 | 已有远程 UI；继续扩展会把重点从 Agent 工程转为平台维护。 |
| 多层级子 Agent 会话导航 | 现有任务与 Worktree 已足够复杂，先把状态摘要做好。 |
| 大规模模型目录与登录体系 | 现有 Anthropic、OpenAI、OpenAI-compatible 已满足演示需求。 |
| 成本自动计费 | 依赖持续维护各模型价格，容易导致数据不准确。 |

## 推荐实施顺序

1. 实现 `build / plan / review` 三个主 Agent 档案并补切换测试。
2. 实现 `/tasks`，用已有进度数据提供汇总视图。
3. 实现 `mygocode run` 的纯文本路径；JSON 输出放在最后。
4. 更新 README，放入一次 `/agent plan`、一次 `/tasks`、一次 `mygocode run --agent review` 的真实输出截图或文本。

## 简历表述参考

功能完成后可表述为：

> 使用 Go 构建终端 AI 编程助手，基于 Agent 档案统一编排提示词、工具白名单与权限模式；实现子任务实时状态汇总和可脚本化的单次执行入口，支持交互式开发、只读规划与自动化代码审查。

仅在相应功能和测试完成后使用这段表述；现阶段应以仓库中的实际实现为准。
