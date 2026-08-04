# 辅助模型（aux model）改进实施规格

> 状态：Implemented（已完成）
>
> 来源：会话讨论「主任务用主力模型，压缩/记忆提取等后台任务用辅助模型」，对标 Claude Code 的 `ANTHROPIC_SMALL_FAST_MODEL`（Haiku 跑摘要/标题等后台任务）与社区呼声最高的 `compactModel` 配置（anthropics/claude-code#12660）。
>
> 目标：新增 provider 级 `aux_model` 配置，让上下文压缩摘要、记忆提取、记忆召回选择器等后台 LLM 任务走更便宜/更快的辅助模型，主对话仍用主力模型。未配置时行为与现状完全一致（全部走主模型）。

当前实现：
- `internal/config/config.go`：`ProviderConfig.AuxModel` 字段（`yaml:"aux_model"`）。
- `internal/llm/model_resolver.go`：`ResolveAuxModel`（别名解析）与 `AuxClientConfig`（复制配置换模型；未配置返回 nil）。
- `internal/agent/agent.go`：`Agent.AuxClient` 字段 + `compactClient()`（含 typed-nil 防护）；自动压缩与 `handleStreamError` 强制压缩均走辅助 client。
- `internal/memory/extractor/extractor.go`：`Deps.AuxClient` + `Deps.client()`；后台提取子代理走辅助 client。
- `internal/tui/tui.go`：`Model.auxClient` + `initAuxClient()`；接线主 agent、记忆提取、记忆召回选择器、`/compact`。
- `internal/remote/server.go`：`Server.auxClient`；接线主 agent、记忆提取、`/compact`。

验证命令：

```bash
go test ./internal/config ./internal/llm ./internal/agent ./internal/compact ./internal/memory/extractor ./internal/tui ./cmd/mygocode
```

配置示例：

```yaml
providers:
  - name: claude
    protocol: anthropic
    base_url: https://api.anthropic.com
    model: claude-sonnet-4-6-20250514
    aux_model: haiku   # 压缩摘要/记忆提取/记忆召回走 Haiku，主对话仍用 Sonnet
```

`aux_model` 支持别名（`haiku`/`sonnet`/`opus` → 对应 Claude 模型 ID），也接受任意 provider 自定义模型名。

## 1. 范围与原则

只做「辅助模型」这一个增量：配置字段 + 三条后台任务链路的 client 切换。不重写压缩、记忆或 LLM 客户端。

- 不实现模型路由/难度分类（如按任务自动选择模型）。
- 不实现视觉模型（vision description）——另立功能另行评估。
- 不修改 `model_resolver.go` 的现有子代理别名语义；只把别名解析复用到 `aux_model` 上（`aux_model: haiku` 可用）。
- 不改变主对话的模型选择逻辑。
- 辅助 client 构建失败必须**静默降级**回主 client，不得阻止启动。

## 2. 优先级与状态

| ID | 功能 | 状态 | 主要代码边界 |
| --- | --- | --- | --- |
| P0 | `aux_model` 配置字段 + 解析辅助 | 已实现 | `internal/config/config.go`、`internal/llm/model_resolver.go` |
| P1 | 上下文压缩（自动 + `/compact`）走辅助模型 | 已实现 | `internal/agent/agent.go`、`internal/compact/`、`internal/tui/tui.go`、`internal/remote/server.go` |
| P2 | 记忆提取与记忆召回选择器走辅助模型 | 已实现 | `internal/memory/extractor/extractor.go`、`internal/tui/tui.go` |

执行顺序为 P0 → P1 → P2。每一阶段均保持未配置 `aux_model` 时行为不变（向后兼容）。

## 3. P0：配置字段与解析

### 行为要求

- `ProviderConfig` 新增 `AuxModel string \`yaml:"aux_model"\``；空值表示「不使用辅助模型」。
- `llm.ResolveAuxModel(name)` 对 `aux_model` 值应用现有别名表（`haiku`/`sonnet`/`opus` → 对应 Claude 模型 ID），未命中原样返回。
- `llm.AuxClientConfig(cfg)` 返回辅助 client 配置：复制 provider 配置并将 `Model` 替换为解析后的辅助模型；未配置 `aux_model` 时返回 `nil`（调用方回退主配置）。
- 辅助 client 的 `context_window`、`max_output_tokens`、`thinking` 等其余字段继承主 provider 配置。

### 验收与测试

- YAML 解析 `aux_model` 字段成功，缺省为空字符串。
- `aux_model: haiku` 解析为 `claude-haiku-4-5-20251001`；`aux_model: glm-5.1` 原样返回。
- `AuxClientConfig` 返回的配置与原配置除 `Model` 外字段一致；未配置时返回 nil。
- 现有不含 `aux_model` 的配置加载、合并、校验行为不变。

## 4. P1：压缩走辅助模型

### 行为要求

- `agent.Agent` 新增 `AuxClient llm.Client` 字段；提供内部 `compactClient()`：`AuxClient` 非空时返回它，否则返回主 `Client`。
- 自动压缩（`agent.go` 主循环 `compact.ManageContext`）与手动压缩（`/compact` 的 `compact.ForceCompact`）均改用 `compactClient()`。
- TUI 启动（单 provider 模式）构建辅助 client 并挂到主 agent；构建失败时告警并回退主 client。
- remote server 的压缩路径同样改用辅助 client（若其构建链路可用）。
- 未配置 `aux_model` 时，所有路径与现状逐字节一致（仍走主 client）。

### 验收与测试

- Agent 设置 `AuxClient` 后，压缩调用打到辅助 client（用记录型 fake client 验证）。
- `AuxClient` 为 nil 时压缩仍走主 client。
- 构建辅助 client 失败（如非法模型名）不影响启动，主流程可用。

## 5. P2：记忆提取与召回选择器走辅助模型

### 行为要求

- `extractor.Deps` 新增可选 `AuxClient llm.Client`；非空时，后台提取子代理（`extractor.go` 的 `agent.New`）改用辅助 client。
- TUI 的 `prefetchRelevantMemories` 侧查询 client 改由 `AuxClientConfig` 构建（未配置时仍用主 provider 配置，行为不变）。
- 记忆提取的沙箱、工具白名单、游标等逻辑一律不动。

### 验收与测试

- Deps 设置 `AuxClient` 后提取子代理使用辅助 client（fake client 验证）。
- Deps 未设置时行为不变（既有 extractor 测试全部通过）。

## 6. 测试矩阵

| 用例 ID | 类型 | 场景 | 预期结果 |
| --- | --- | --- | --- |
| AUX-CFG-01 | 单元 | YAML 解析 `aux_model` | 字段正确，缺省为空串 |
| AUX-CFG-02 | 单元 | `aux_model: haiku` / 自定义名 | 别名解析为 Claude 模型 ID；自定义名原样 |
| AUX-CFG-03 | 单元 | `AuxClientConfig` 复制语义 | 仅 Model 替换，其余字段一致；未配置返回 nil |
| AUX-AGENT-01 | 单元 | Agent 带/不带 AuxClient 压缩 | 压缩分别打到辅助/主 client |
| AUX-EXTR-01 | 单元 | Deps 带/不带 AuxClient 提取 | 子代理分别用辅助/主 client |
| AUX-IT-01 | 集成 | 完整 `go test ./...` | 全量通过（既有失败项另登记，不归因本规格） |

推荐命令：

```bash
go test ./internal/config ./internal/llm ./internal/agent ./internal/compact ./internal/memory/extractor
go test ./...
```

基线记录（2026-08-05）：`internal/config` 存在既有 `MediaProviderConfig` 编译问题（`media_providers_test.go` 引用未定义类型），属历史遗留，另登记。

## 7. 文档规则

- 未通过对应测试前，README 只标记为进行中并链接本规格。
- 功能完成后将本文件状态更新为 `Implemented`，并补充实际实现文件与测试命令。
- README 配置示例补充 `aux_model` 字段说明。
