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

---

## 六、简历描述具体改进建议

### 6.1 项目简介部分

**【当前描述】**
```
轻量级终端 Coding Agent，基于 ReAct 与 Plan Mode 双模式驱动 LLM 自主完成编程任务，
支持 MCP 工具扩展、Skill 技能包、跨会话记忆、多 Agent 并行协作。
```

**【问题】**
- "轻量级"定位模糊，未体现与商业工具的差异化
- 技术点罗列但缺乏串联

**【建议改为】**
```
基于终端的 AI 编程助手，通过 TUI/浏览器双界面与 LLM 交互完成编程任务；
统一封装 Anthropic/OpenAI 流式协议，支持 ReAct 与 Plan Mode 双模式、
MCP 工具扩展、跨会话记忆、多 Agent 协作。
```

**【优化点】**
- 强调"TUI/浏览器双界面"是差异化特性
- 突出"统一封装多 LLM 协议"的技术价值


---

### 6.2 架构设计部分

**【当前描述】**
```
架构设计：按 TUI/Remote、Agent/Conversation、Tools/MCP/Skills、Memory/Session、
Permissions/Sandbox 等子系统组织代码；统一 Anthropic、OpenAI 流式响应协议为统一事件接口。
```

**【问题】**
- 根据第2.1节分析，这个描述准确但不够简洁
- "统一事件接口"放在架构设计中略显突兀

**【建议改为】**
```
架构设计：按交互层（TUI/Remote）、引擎层（Agent/Conversation）、工具层（Tools/MCP/Skills）、
持久化层（Memory/Session）、安全层（Permissions/Sandbox）组织代码；
统一 Anthropic、OpenAI 流式响应为 StreamEvent 接口。
```

**【优化点】**
- 明确标注各层职责，便于面试时展开
- 简化"统一事件接口"的表述


---

### 6.3 安全控制部分 ⚠️ 关键修正

**【当前描述】**
```
安全控制：设计多层权限拦截模型，覆盖 Plan Mode 写入例外、安全只读放行、
危险命令检测、路径沙箱、YAML 规则引擎、会话级放行与 HITL 人工确认。
```

**【问题】**
- 根据第2.3节分析，实际是 6 层（含子层达 8 层），不是 5 层
- 当前描述未提及层数，这是好事，保持即可

**【建议】**
```
安全控制：设计 6 层权限拦截模型，按优先级短路执行：Plan Mode 写入例外、
安全只读放行、危险命令检测（13 个 regex）、路径沙箱、YAML 规则引擎
（支持 user/project/local 三级覆盖）、会话级放行与 HITL 人工确认。
```

**【优化点】**
- 明确标注"6 层"，与代码实际一致
- 增加"13 个 regex"和"三级覆盖"等可量化细节
- "按优先级短路执行"体现设计思想


---

### 6.4 性能优化部分 ⚠️ 关键修正

**【当前描述】**
```
性能优化：实现 MCP 工具延迟加载与 ToolSearch 按需发现，Benchmark 实测 
Token 占用减少约 84%；两层渐进式上下文压缩策略支持数小时连续编程会话。
```

**【问题】**
- 根据第2.4节分析，实测是 84.2%，当前描述"约 84%"准确
- 这部分描述已经很好，建议保持

**【建议】保持不变，或微调为：
```
性能优化：实现 MCP 工具延迟加载与 ToolSearch 按需发现，Benchmark 实测 
Token 占用减少 84%（58 工具 10 轮会话）；两层渐进式上下文压缩策略
（双阈值触发 + 边界对齐）支持数小时连续编程会话。
```

**【优化点】**
- 增加"58 工具 10 轮会话"测试场景说明
- 增加"双阈值触发 + 边界对齐"技术细节


---

### 6.5 记忆系统部分 ⚠️ 关键修正

**【当前描述】**
```
记忆系统：实现 Markdown+YAML 持久化，异步调用 LLM 自动提取四类记忆
（用户偏好、纠正反馈、项目知识、参考信息），支持跨会话知识持续累积。
```

**【问题】**
- 根据第2.6节分析，当前描述已经准确（Markdown+YAML）
- 这部分描述已经很好

**【建议】保持不变，或微调增加技术细节：
```
记忆系统：实现 Markdown+YAML frontmatter 持久化，异步调用 LLM 自动提取
四类记忆（用户偏好、纠正反馈、项目知识、参考信息），支持 in-progress 
coalescing 防重入，跨会话知识持续累积。
```

**【优化点】**
- 明确"YAML frontmatter"格式
- 增加"in-progress coalescing 防重入"技术亮点


---

### 6.6 多 Agent 协作部分 ⚠️ 关键修正

**【当前描述】**
```
多 Agent 协作：基于 Git Worktree 为子 Agent 创建独立工作区；Coordinator Agent 
通过工具约束引导 LLM 完成任务拆分，多个 Agent 并行处理复杂任务。
```

**【问题】**
- 根据第2.7节分析，当前描述已经准确
- "通过工具约束引导 LLM"的表述准确反映了被动约束而非主动调度

**【建议】保持不变，这是准确的描述

**【面试准备】**
如果被问"Coordinator 怎么拆分任务"，回答要点：
- Coordinator 通过工具过滤器限制只能使用 Agent/SendMessage/TaskXxx/只读工具
- 任务拆分逻辑由 LLM 根据 prompt 自行决定，而非代码层面的自动分解算法
- 设计思路：约束工具集 → 引导 LLM 行为模式


---

## 七、简历修改总结（对照表）

### 核心修改点一览

| 项目 | 当前简历 | 建议修改 | 优先级 |
|------|---------|---------|--------|
| 项目简介 | "轻量级终端 Coding Agent" | "基于终端的 AI 编程助手，通过 TUI/浏览器双界面..." | P2 |
| 架构设计 | 子系统罗列 | 明确标注各层职责（交互层、引擎层...） | P2 |
| 安全控制 | "多层权限拦截模型" | "6 层权限拦截模型，按优先级短路执行..." | P0 |
| 性能优化 | "Token 占用减少约 84%" | 保持不变，或增加测试场景说明 | P2 |
| 记忆系统 | "Markdown+YAML 持久化" | 保持不变，或增加 "in-progress coalescing" | P2 |
| 多 Agent | "通过工具约束引导 LLM" | 保持不变 | - |

**优先级说明：**
- P0：必须修改，否则面试时容易被问倒
- P1：建议修改，增强准确性
- P2：可选修改，增强细节


---

## 八、修改后的完整简历文本建议

### 项目简介（修改后）

**MyGo Coding Agent** — 终端 AI 编程助手

**项目简介**：基于终端的 AI 编程助手，通过 TUI/浏览器双界面与 LLM 交互完成编程任务；统一封装 Anthropic/OpenAI 流式协议，支持 ReAct 与 Plan Mode 双模式、MCP 工具扩展、跨会话记忆、多 Agent 协作。

**技术栈**：Go、MCP、ReAct、Multi-Agent、Anthropic API、OpenAI API


**技术亮点**：
- **架构设计**：按交互层（TUI/Remote）、引擎层（Agent/Conversation）、工具层（Tools/MCP/Skills）、持久化层（Memory/Session）、安全层（Permissions/Sandbox）组织代码；统一 Anthropic、OpenAI 流式响应为 StreamEvent 接口。
- **安全控制**：设计 6 层权限拦截模型，按优先级短路执行：Plan Mode 写入例外、安全只读放行、危险命令检测（13 个 regex）、路径沙箱、YAML 规则引擎（支持 user/project/local 三级覆盖）、会话级放行与 HITL 人工确认。


- **性能优化**：实现 MCP 工具延迟加载与 ToolSearch 按需发现，Benchmark 实测 Token 占用减少 84%（58 工具 10 轮会话）；两层渐进式上下文压缩策略（双阈值触发 + 边界对齐）支持数小时连续编程会话。
- **记忆系统**：实现 Markdown+YAML frontmatter 持久化，异步调用 LLM 自动提取四类记忆（用户偏好、纠正反馈、项目知识、参考信息），支持 in-progress coalescing 防重入，跨会话知识持续累积。
- **多 Agent 协作**：基于 Git Worktree 为子 Agent 创建独立工作区；Coordinator Agent 通过工具约束引导 LLM 完成任务拆分，多个 Agent 并行处理复杂任务。


---

## 九、最终检查清单

在更新简历前，请逐项核对：

### ✅ 必须修改项（P0）
- [ ] 安全控制：确认改为"6 层权限拦截模型"
- [ ] 安全控制：增加"按优先级短路执行"描述
- [ ] 安全控制：增加"13 个 regex"、"三级覆盖"等量化细节

### 🔄 建议修改项（P1-P2）
- [ ] 项目简介：考虑强调"TUI/浏览器双界面"差异化特性
- [ ] 架构设计：考虑明确标注各层职责
- [ ] 性能优化：考虑增加"58 工具 10 轮会话"测试场景
- [ ] 性能优化：考虑增加"双阈值触发 + 边界对齐"细节
- [ ] 记忆系统：考虑增加"in-progress coalescing 防重入"亮点

### 📝 面试准备项
- [ ] 准备展示 `TestDeferredBenchmarkFullSession` 测试代码
- [ ] 准备讲解"两层上下文压缩"的设计细节
- [ ] 准备回答"Coordinator 怎么拆分任务"（工具约束引导）
- [ ] 准备回答"记忆持久化格式"（Markdown+YAML frontmatter）


---

## 十、结论与行动建议

### 总体评价
MyGo Coding Agent 项目**技术实力扎实，完全值得写入简历**。代码实现与简历描述的匹配度约 **90%**，主要问题集中在表述细节而非技术能力。

### 核心优势（面试时重点讲）
1. **两层上下文压缩**：工程深度高，设计完整（双阈值、边界对齐、断点续传）
2. **MCP 延迟加载**：有 Benchmark 验证，84% Token 节省真实可信
3. **6 层权限模型**：覆盖面广，YAML 规则引擎设计合理
4. **统一 LLM 接口**：同时支持 Anthropic/OpenAI/兼容协议，抽象得当

### 立即行动项
1. **修改简历**：按第六节建议，优先修改安全控制部分（P0 级）
2. **准备演示**：准备展示 `internal/tools/deferred_benchmark_test.go` 中的测试代码
3. **熟悉代码**：重点熟悉 `internal/compact/` 和 `internal/permissions/` 两个子系统
4. **模拟面试**：按第四节"需要在面试前准备好的问题"逐条准备答案

### 风险提示
- ⚠️ 不要在面试中提及"五层架构"或其他简历中未出现的不准确描述
- ⚠️ 量化数据（84%）务必能指出验证代码的具体位置
- ⚠️ 如被问及实现细节，优先讲有测试覆盖的部分

---

**报告完成时间**：2026-07-26  
**建议复查周期**：面试前 24 小时再次核对简历与代码的一致性

