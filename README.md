# MygoCode

> A Terminal-based AI Coding Assistant written in Go

## 📖 About

MygoCode is a production-ready AI coding assistant that runs in your terminal. It can read, search, edit files, execute commands, and interact with LLMs through both TUI and browser-based Remote UI. The project supports multiple LLM providers (OpenAI, Anthropic, and OpenAI-compatible APIs) and can be extended with MCP (Model Context Protocol) servers.

### Key Highlights

- **Unified LLM Interface**: Abstracts Anthropic and OpenAI streaming protocols into a single interface
- **MCP Tool Lazy Loading**: Reduces token usage by ~84% through deferred tool discovery (verified by benchmark tests)
- **Two-layer Context Compression**: Dual-threshold triggering with boundary snapping and recovery attachment system
- **Multi-layer Permission System**: 6+ layer interception model including plan mode, safe command allowlist, dangerous command detection, path sandbox, YAML rule engine, and HITL confirmation
- **Async Memory Extraction**: Automatically extracts 4 types of memories (user, feedback, project, reference) with in-progress coalescing
- **Agent Collaboration**: Git worktree-based isolation for parallel agent execution

## ✨ Features

- **Interactive TUI**: Built with Bubbletea for seamless terminal interaction within your codebase
- **Remote UI Mode**: Access through browser via local HTTP + WebSocket server
- **Multi-provider Support**: Compatible with `anthropic`, `openai`, and `openai-compat` protocols
- **Built-in Code Tools**: File read/write, precise editing, command execution, Glob search, Grep search
- **MCP Server Integration**: Connect external tools via Model Context Protocol with on-demand discovery
- **Skills System**: Project-specific skills in `.mygocode/skills/<skill-name>/SKILL.md`
- **Session Management**: Resume conversations, context compression, plan mode, permission modes, code review
- **Hooks System**: Execute commands, HTTP requests, prompts, or agent actions on session/message/tool events
- **Advanced Capabilities**: Auto-memory, todo tracking, sub-agents, team collaboration, worktree assistance

## 📋 Requirements

- **Go**: Version 1.25.0 or higher
- **LLM Provider**: At least one configured LLM provider (OpenAI, Anthropic, or compatible API)
- **API Key**: Set via environment variable or configuration file for remote model access

## 🚀 Quick Start

### Installation

Clone the repository and download dependencies:

```bash
git clone https://github.com/yourusername/mygocode.git
cd mygocode
go mod download
```

### Build

Build the executable:

```bash
go build -o mygocode ./cmd/mygocode
```

### Run Terminal TUI

```bash
./mygocode
```

### Run Browser Remote UI

Start the Remote UI server (default port: 18888):

```bash
./mygocode --remote
```

Then open your browser at `http://localhost:18888`

Custom port:

```bash
./mygocode --remote :3000
```

## ⚙️ Configuration

MygoCode loads and merges configuration files in the following order:

```text
~/.mygocode/config.yaml           # Global user config
<project>/.mygocode/config.yaml   # Project config
<project>/.mygocode/config.local.yaml  # Local overrides (gitignored)
```

### Minimal Configuration

**OpenAI Example:**

```yaml
providers:
  - name: openai
    protocol: openai
    base_url: https://api.openai.com/v1
    model: gpt-4.1
```

**Anthropic Example:**

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

### API Keys

Set API keys via environment variables (recommended):

```bash
export OPENAI_API_KEY="your-api-key"
export ANTHROPIC_API_KEY="your-api-key"
```

Or directly in config (not recommended for version control):

```yaml
providers:
  - name: claude
    protocol: anthropic
    api_key: your-api-key
```

### OpenAI-Compatible Services

```yaml
providers:
  - name: local-compatible
    protocol: openai-compat
    base_url: http://localhost:11434/v1
    model: your-model-name
    api_key: dummy
```

## 🔌 MCP Configuration

Connect MCP servers via `mcp_servers` configuration. Supports stdio, Streamable HTTP, and SSE transports.

**Stdio Example:**

```yaml
mcp_servers:
  - name: filesystem
    command: npx
    args:
      - -y
      - "@modelcontextprotocol/server-filesystem"
      - .
```

**HTTP Example:**

```yaml
mcp_servers:
  - name: remote-tools
    url: http://localhost:3001/mcp
    transport: http
    headers:
      Authorization: "Bearer ${MCP_TOKEN}"
```

MCP tools are registered as `mcp__<server>__<tool>` and loaded on-demand via `ToolSearch`.

## 💻 Commands

Slash commands available in TUI or Remote UI:

```text
/help              Show available commands
/status            Show current model, directory, tokens, tools
/clear             Clear current conversation
/compact           Compress current context
/plan              Enter read-only plan mode
/review            Review current code changes
/resume            Resume a previous session
/session           Show session information
/memory list       View auto-memory
/memory clear      Clear auto-memory
/skills            List available skills
/skills reload     Reload skills
/permission info   Show permission mode
/permission mode   Set mode: default|acceptEdits|plan|bypassPermissions
/mcp               Show MCP server status
/sandbox           Configure command execution sandbox
```

## 🪝 Hooks

Declare hooks in configuration to trigger actions on specific events:

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

**Supported Events:**

```text
session_start, session_end, turn_start, turn_end,
pre_send, post_receive, pre_tool_use, post_tool_use, shutdown
```

**Supported Action Types:**

```text
command, prompt, http, agent
```

## 🎯 Skills System

Project-specific skills are defined in `.mygocode/skills/<skill-name>/SKILL.md`.

**Example Skill File:**

```markdown
---
name: api-review
description: Review API design and compatibility risks.
when_to_use: Use when reviewing HTTP API changes.
tags:
  - review
---

Please check if API changes have compatibility, security, and error handling risks.
User request: $ARGUMENTS
```

**Usage:**

```text
/api-review Check this API change
```

## 📁 Project Structure

```text
cmd/mygocode/            CLI entry, Remote mode, teammate worker mode
internal/agent/         Agent main loop and event stream
internal/llm/           OpenAI, Anthropic, OpenAI-compatible clients
internal/tools/         Built-in tools (file, command, search, media input)
internal/tui/           Terminal interactive interface
internal/remote/        Browser Remote UI service
internal/config/        YAML config loading, merging, validation
internal/mcp/           MCP Server connection and tool wrapping
internal/commands/      Slash command registration and handling
internal/skills/        Skill loading, parsing, execution
internal/hooks/         Hooks configuration, validation, execution
internal/memory/        Auto-memory and memory extraction
internal/session/       Session save and restore
internal/teams/         Multi-agent team collaboration
internal/worktree/      Git worktree assistance
internal/permissions/   Permission checking and path sandbox
```

## 🔧 Development

**Run Tests:**

```bash
go test ./...
```

**Format Code:**

```bash
gofmt -w .
```

**Pre-commit Check:**

```bash
go test ./...
```

## 💡 Best Practices

- Store global config in `~/.mygocode/config.yaml`, project-specific config in `.mygocode/config.local.yaml`
- Never commit real API keys to the repository; use environment variables or local config files
- Use `/plan` mode before making large-scale changes to align on approach
- Use `/review` to quickly check logic, security, performance, and style issues in current git diff

