# MyGo Coding Agent 项目评估报告

> 基于源码实际分析（非简历自述），逐条核查，如实评估。

---

## 一、项目整体评价

**结论：是一个有较高完成度的真实项目，核心机制均有代码支撑，适合写入简历。**

项目模块名为 `mewcode`（非 `mygocode`），共约 50 个测试文件，覆盖核心子系统，代码结构清晰，分包合理。相比许多"简历项目"，这个项目的技术深度和实现细节均属真实，但简历中有若干表述与代码事实存在偏差，需要修正，否则面试时容易被问倒。

---

## 二、简历各项声明逐条核查

### 2.1 「五层分层架构（交互、引擎、工具、记忆、安全）」

**结论：描述失真，建议修正。**

代码中不存在以"交互/引擎/工具/记忆/安全"命名的五层架构。实际结构是若干独立子系统：

| 简历描述 | 代码实际 |
| --- | --- |
| 交互层 | `internal/tui/`（Bubbletea TUI）+ `internal/remote/`（HTTP/WS） |
| 引擎层 | `internal/agent/`（ReAct 循环）+ `internal/conversation/` |
| 工具层 | `internal/tools/` + `internal/mcp/` + `internal/skills/` |
| 记忆层 | `internal/memory/` + `internal/history/` + `internal/session/` |
| 安全层 | `internal/permissions/` + `internal/sandbox/` |

这是客观存在的分层，但代码里没有这个命名体系。简历若写"五层"，面试官要求你展示对应代码时会尴尬。建议改为描述实际的包结构。

---

### 2.2 「统一 Anthropic、OpenAI 流式响应协议为统一接口」

**结论：准确，实现完整。**

`internal/llm/client.go` 定义了统一 `Client` 接口：

```go
type Client interface {
    Stream(ctx context.Context, conv *conversation.Manager, tools []map[string]any) (<-chan StreamEvent, <-chan error)
    SetSystemPrompt(prompt string)
}
```

Anthropic 和 OpenAI（含兼容协议）各自实现，向上暴露相同的 `StreamEvent` 类型（TextDelta/ToolCallStart/ToolCallDelta/ToolCallComplete 等）。这条亮点真实可信。

---

### 2.3 「五层权限拦截模型」

**结论：层数有误，实际是 6 层（含子层达 8 层），但机制真实。**

`internal/permissions/permissions.go` 中 `Checker.Check()` 的实际层次：

| 层号 | 名称 | 内容 |
| --- | --- | --- |
| Layer 0 | Plan Mode 写例外 | 仅允许写入 planfile |
| Layer 1 | 安全只读自动放行 | 白名单命令自动 allow |
| Layer 1b | OS 沙箱自动放行 | sandbox 内命令自动 allow |
| Layer 2 | 危险命令检测 | 13 个 regex：`rm -rf /`、fork bomb、`git push --force` 等 |
| Layer 3 | 路径沙箱检查 | 限制写入在项目根目录 + tmpdir |
| Layer 4 | YAML 规则引擎 | 加载 user/project/local 三级 YAML 规则 |
| Layer 4b | Session allow-always | 会话级记忆化放行 |
| Layer 5 | HITL 人工确认 | 弹出终端交互询问 |

简历写"五层"实际是 6~8 层，建议改为"多层权限拦截"或直接说"6 层"。这个设计本身是亮点，不用缩水描述。

---

### 2.4 「MCP 工具延迟加载机制使 Token 占用减少 85%」

**结论：机制真实，有 benchmark 测试支撑，实测 84.2%，简历写的 85% 略有高估。**

延迟加载机制确实存在：`mcp/mcp.go` 中所有 MCP 工具实现 `ShouldDefer() bool { return true }`，初始不向 LLM 提交工具 schema，由 `ToolSearch` 按需加载。

`internal/tools/deferred_benchmark_test.go` 有完整的模拟测试（58 个 MCP 工具、10 轮对话），实测全会话 token 节省 **84.2%**（全量 158685 estimated tokens → 延迟加载 25102）。测试断言阈值 `>= 80%`，实际数字略低于简历所写的 85%。

建议简历改为"减少约 84%"或"减少 80%+"，面试时可以直接说"有 benchmark 测试验证"，可信度高。

---

### 2.5 「两层渐进式上下文压缩策略」

**结论：准确，是项目中技术深度最高的部分。**

- **Layer 1**（`internal/toolresult/`）：单条工具结果预算控制，按 token 上限 spill/snip，防止单个结果撑爆上下文。
- **Layer 2**（`internal/compact/compact.go`）：LLM 驱动全会话摘要，包含：
  - 软触发（13000 token 余量）和硬触发（3000 token 余量）双阈值
  - 保留末尾 10K token / 最少 5 条消息的 verbatim 窗口
  - 防止切割 tool_use/tool_result 对的 boundary snapping
  - 压缩后 `RecoveryAttachment` 重注入最近文件读取和 skill SOP
  - 断点续传：compact 边界写入 JSONL session log

这条完全属实，建议在面试中重点讲。

---

### 2.6 「JSONL 持久化，异步调用 LLM 自动提取四类记忆」

**结论：部分准确，"JSONL 持久化"描述对象有误。**

四类记忆和异步提取是真实的：

- 四类记忆（TypeUser/TypeFeedback/TypeProject/TypeReference）存在于 `internal/memory/memory_types.go`
- 异步提取器（`internal/memory/extractor/extractor.go`）在每次 `LoopComplete` 后 fire-and-forget，有 in-progress coalescing 防重入，shutdown 时 `Drain()` 等待最多 60s

但**记忆存储格式是带 YAML frontmatter 的 Markdown 文件，不是 JSONL**。JSONL 用于 `prompt_history`（`internal/history/`）和 session log（`internal/session/`）。

建议简历改为："实现 Markdown+YAML 持久化，异步…"，或不提格式细节。

---

### 2.7 「基于 Git Worktree 实现文件级隔离；Coordinator Agent 负责任务拆分，多 Agent 并行」

**结论：worktree 隔离真实；coordinator 是工具过滤器，非主动拆分。**

**Worktree 隔离**（`internal/worktree/agent.go`）：`CreateAgentWorktree()` 为每个子 Agent 创建独立 worktree，不 chdir、不耦合 TUI，有 10 个测试文件，实现完整。

**Coordinator**（`internal/teams/coordinator.go`）：实际是一个工具过滤器——当 Team 存在时，限制 Coordinator Agent 只能使用 Agent/SendMessage/TaskXxx/只读工具。这是**被动约束**，不是主动分解任务的调度器。任务拆分逻辑由 LLM 根据 prompt 自行决定，代码层面没有自动分解算法。

建议简历改为："Coordinator Agent 通过工具约束引导 LLM 完成任务拆分，多 Agent 并行…"，更准确。

---

## 三、项目真正的技术亮点（建议在面试中主动讲）

1. **两层上下文压缩**：双阈值触发、boundary snapping、RecoveryAttachment、断点续传，是真正有工程深度的设计，很多同类项目没有做到。

2. **MCP 延迟加载 + ToolSearch 按需发现**：设计思路清晰，`ShouldDefer` 接口 + registry 层的 discover/promote 流程，可以展开讲 10 分钟。

3. **异步记忆提取器**：in-progress coalescing、bypass 模式子 Agent、60s drain 优雅退出，细节扎实。

4. **统一 LLM 流式接口**：同时支持 Anthropic SDK、OpenAI Responses API、任意兼容协议，还处理了 extended thinking 的 ThinkingDelta 事件，接口抽象得当。

5. **权限分层模型**：8 层按优先级短路执行，YAML 规则 DSL 支持三级覆盖（user/project/local），OS 级 sandbox（Linux/Darwin），设计合理。

---

## 四、需要在面试前准备好的问题

| 可能被问到 | 准备方向 |
| --- | --- |
| "84%/85% 是怎么测量的？" | `TestDeferredBenchmarkFullSession`，58 工具 10 轮，实测 84.2%，可直接演示 |
| "五层架构对应代码哪里？" | 改为描述实际包结构 |
| "Coordinator 怎么拆分任务？" | 明确说是 prompt 引导 + 工具约束，不是算法 |
| "记忆用 JSONL 还是别的格式？" | 修正为 Markdown+YAML frontmatter |
| "Plan Mode 和 ReAct 有什么区别？" | Plan Mode = 限权 + 5 阶段 prompt，只能写 planfile；ReAct = 标准 think-act-observe |

---

## 五、总体建议

这个项目**完全值得写进简历**，技术实现的广度和深度都超出了大多数课程项目。主要风险不在于项目本身，而在于简历措辞与代码实际之间的几处偏差。逐条修正后，面试时展开讲任何一个子系统都能撑住追问。
