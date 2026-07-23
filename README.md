# MygoCode

> 一款基于终端的 AI 编程助手，使用 Go 语言编写

## 📖 简介

MygoCode 是一款可直接在生产环境中使用的 AI 编程助手，运行在您的终端中。它能读写、搜索、编辑文件，执行命令，并通过 TUI 和基于浏览器的远程 UI 与 LLM 交互。该项目支持多种 LLM 提供商（OpenAI、Anthropic 以及兼容 OpenAI 协议的 API），并可通过 MCP（模型上下文协议）服务器进行扩展。

### 核心亮点

- **统一 LLM 接口**：将 Anthropic 和 OpenAI 的流式传输协议抽象为统一接口
- **MCP 工具懒加载**：通过延迟工具发现机制，减少约 84% 的 token 消耗（经基准测试验证）
- **双层上下文压缩**：双阈值触发机制，结合边界对齐与恢复附件系统
- **多层权限系统**：6 层以上拦截模型，包括计划模式、安全命令白名单、危险命令检测、路径沙箱、YAML 规则引擎和人机确认
- **异步记忆提取**：自动提取 4 种类型的记忆（用户、反馈、项目、参考），支持进行中合并
- **智能体协作**：基于 Git 工作树的隔离机制，实现智能体并行执行

## ✨ 功能特性

- **交互式 TUI**：基于 Bubbletea 构建，在代码库中实现无缝终端交互
- **远程 UI 模式**：通过本地 HTTP + WebSocket 服务器在浏览器中访问
- **多提供商支持**：兼容 `anthropic`、`openai` 和 `openai-compat` 协议
- **内置代码工具**：文件读写、精确编辑、命令执行、Glob 搜索、Grep 搜索
- **MCP 服务器集成**：通过模型上下文协议连接外部工具，支持按需发现
- **技能系统**：项目级技能定义在 `.mygocode/skills/<skill-name>/SKILL.md` 中
- **会话管理**：对话恢复、上下文压缩、计划模式、权限模式、代码审查
- **钩子系统**：在会话/消息/工具事件上执行命令、HTTP 请求、提示或智能体操作
- **高级功能**：自动记忆、待办事项追踪、子智能体、团队协作、工作树辅助

## 📋 环境要求

- **Go**：1.25.0 或更高版本
- **LLM 提供商**：至少配置一个 LLM 提供商（OpenAI、Anthropic 或兼容 API）
- **API 密钥**：通过环境变量或配置文件设置，用于远程模型访问

## 🚀 快速入门

### 安装

克隆仓库并下载依赖：

```bash
git clone https://github.com/yourusername/mygocode.git
cd mygocode
go mod download
```

### 编译

编译可执行文件：

```bash
go build -o mygocode ./cmd/mygocode
```

### 运行终端 TUI

```bash
./mygocode
```

### 运行浏览器远程 UI

启动远程 UI 服务器（默认端口：18888）：

```bash
./mygocode --remote
```

然后在浏览器中打开 `http://localhost:18888`

自定义端口：

```bash
./mygocode --remote :3000
```

## ⚙️ 配置

MygoCode 按以下顺序加载和合并配置文件：

```text
~/.mygocode/config.yaml           # 全局用户配置
<project>/.mygocode/config.yaml   # 项目配置
<project>/.mygocode/config.local.yaml  # 本地覆盖配置（已加入 gitignore）
```

### 最小配置

**OpenAI 示例：**

```yaml
providers:
  - name: openai
    protocol: openai
    base_url: https://api.openai.com/v1
    model: gpt-4.1
```

**Anthropic 示例：**

```yaml
providers:
  - name: claude
    protocol: anthropic
    base_url: https://api.anthropic.com
    model: claude-sonnet-4-20250514
    thinking: true
    context_window: 200000
    max_output_tokens: 64000
```

### API 密钥

通过环境变量设置 API 密钥（推荐）：

```bash
export OPENAI_API_KEY="your-api-key"
export ANTHROPIC_API_KEY="your-api-key"
```

或直接在配置文件中设置（不建议纳入版本控制）：

```yaml
providers:
  - name: claude
    protocol: anthropic
    api_key: your-api-key
```

### 兼容 OpenAI 的服务

```yaml
providers:
  - name: local-compatible
    protocol: openai-compat
    base_url: http://localhost:11434/v1
    model: your-model-name
    api_key: dummy
```

## 🔌 MCP 配置

通过 `mcp_servers` 配置连接 MCP 服务器。支持 stdio、Streamable HTTP 和 SSE 传输协议。

**Stdio 示例：**

```yaml
mcp_servers:
  - name: filesystem
    command: npx
    args:
      - -y
      - "@modelcontextprotocol/server-filesystem"
      - .
```

**HTTP 示例：**

```yaml
mcp_servers:
  - name: remote-tools
    url: http://localhost:3001/mcp
    transport: http
    headers:
      Authorization: "Bearer ${MCP_TOKEN}"
```

MCP 工具注册格式为 `mcp__<server>__<tool>`，通过 `ToolSearch` 按需加载。

## 💻 命令

TUI 或远程 UI 中可用的斜杠命令：

```text
/help              显示可用命令
/status            显示当前模型、目录、token、工具信息
/clear             清除当前对话
/compact           压缩当前上下文
/plan              进入只读计划模式
/review            审查当前代码变更
/resume            恢复之前的会话
/session           显示会话信息
/fork              从当前会话创建独立分支
/export [path]     导出脱敏的离线 HTML 会话记录
/trust [action]    管理项目配置可信状态：status|trust|deny|revoke
/memory list       查看自动记忆
/memory clear      清除自动记忆
/skills            列出可用技能
/skills reload     重新加载技能
/permission info   查看权限模式
/permission mode   设置模式：default|acceptEdits|plan|bypassPermissions
/mcp               显示 MCP 服务器状态
/sandbox           配置命令执行沙箱
```

## 🪝 钩子

在配置中声明钩子，在特定事件上触发操作：

```yaml
hooks:
  - id: block-dangerous-rm
    event: pre_tool_use
    if: 'tool == "Bash" && args.command =~ /rm\s+-rf/'
    reject: true
    action:
      type: prompt
      message: "Dangerous delete command detected and blocked."
```

**支持的事件：**

```text
session_start, session_end, turn_start, turn_end,
pre_send, post_receive, pre_tool_use, post_tool_use, shutdown
```

**支持的操作类型：**

```text
command, prompt, http, agent
```

## 🎯 技能系统

项目级技能定义在 `.mygocode/skills/<skill-name>/SKILL.md` 中。

**技能文件示例：**

```markdown
---
name: api-review
description: 审查 API 设计和兼容性风险。
when_to_use: 在审查 HTTP API 变更时使用。
tags:
  - review
---

请检查 API 变更是否存在兼容性、安全性和错误处理方面的风险。
用户请求：$ARGUMENTS
```

**使用方法：**

```text
/api-review 检查此 API 变更
```

## 📁 项目结构

```text
cmd/mygocode/            CLI 入口、远程模式、协作者工作模式
internal/agent/         智能体主循环和事件流
internal/llm/           OpenAI、Anthropic、兼容 OpenAI 的客户端
internal/tools/         内置工具（文件、命令、搜索、媒体输入）
internal/tui/           终端交互界面
internal/remote/        浏览器远程 UI 服务
internal/config/        YAML 配置加载、合并、校验
internal/mcp/           MCP 服务器连接和工具封装
internal/commands/      斜杠命令注册和处理
internal/skills/        技能加载、解析、执行
internal/hooks/         钩子配置、校验、执行
internal/memory/        自动记忆和记忆提取
internal/session/       会话保存和恢复
internal/teams/         多智能体团队协作
internal/worktree/      Git 工作树辅助
internal/permissions/   权限检查和路径沙箱
```

## 🔧 开发

**运行测试：**

```bash
go test ./...
```

**格式化代码：**

```bash
gofmt -w .
```

**提交前检查：**

```bash
go test ./...
```

## 📌 Pi 借鉴改进计划

当前状态：**已实现并通过针对性测试**。本项目根据 Pi Agent 的实践，已按 P0 → P1 → P2 完成三项轻量改进：

| 优先级 | 计划 | 目的 |
| --- | --- | --- |
| P0 | 项目本地配置“信任门槛” | 陌生仓库默认不执行项目 Hook、命令、技能和 MCP 子进程 |
| P1 | `/fork` 会话分支 | 保留原方案，独立尝试新的实现路径 |
| P2 | `/export` 脱敏 HTML | 导出可离线查看的调试和面试展示记录 |

完整的需求、阶段计划、验收标准和测试矩阵见：[PI_改进实施规格.md](./PI_改进实施规格.md)。项目级配置默认不受信任，仅加载全局配置；通过 `/trust trust` 明确授权后，重启程序才会加载项目的 Hook、命令、技能和 MCP 配置。

如果项目仅有本地配置、尚未配置全局 provider，先执行 `./mygocode --trust-project` 完成显式首次授权，再重新启动程序。

## 📌 OpenCode 借鉴改进计划

当前状态：**进行中**。P0/P1 已完成首版实现并有针对性单元测试；P2 非交互执行经评估后不在当前范围内实施。

| 优先级 | 计划 | 目的 |
| --- | --- | --- |
| P0 | `build / plan / review` 主 Agent 档案 | 让工作阶段、提示词和权限边界一眼可见 |
| P1 | 只读 `/tasks` 汇总 | 查看子 Agent 与团队成员的状态和最近活动 |
| P2 | `mygocode run` 非交互执行（不实施） | 经评估后不纳入当前项目 |

已实现：`/agent` 的 `build`、`plan`、`review` 档案映射与只读工具限制，以及只读 `/tasks` 汇总。完整需求、验收标准和测试矩阵见：[OPENCODE_改进实施规格.md](./OPENCODE_改进实施规格.md)。

## 💡 最佳实践

- 全局配置存放在 `~/.mygocode/config.yaml`，项目级配置存放在 `.mygocode/config.local.yaml`
- 切勿将真实的 API 密钥提交到仓库中；请使用环境变量或本地配置文件
- 在进行大规模更改前使用 `/plan` 模式，统一方案思路
- 使用 `/review` 快速检查当前 git diff 中的逻辑、安全、性能和样式问题
