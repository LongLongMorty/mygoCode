# 从 Codex 借鉴 MygoCode 的轻量改进建议

> 目的：服务日常实习项目展示，不复刻 Codex 的完整产品。本文基于 2026-07-22 对 [OpenAI Codex CLI](https://github.com/openai/codex) 公开 README、CLI 定位和开源仓库结构的观察，并结合 MygoCode 当前源码提出建议。

## 结论

Codex 的工程特点是把“模型能做什么”明确拆成三件事：在哪个沙箱里运行、什么时候需要人批准、如何在交互式终端之外被脚本调用。它还用分层的 `AGENTS.md` 约束不同目录下的工作方式。

MygoCode 已经有对应的底座：权限模式、路径沙箱、OS 沙箱、Hooks、Plan Mode，以及 `AGENTS.md` / `MYGOCODE.md` 的分层加载和 `@include` 边界检查。因此建议做的是统一入口和可观测性，而不是重新实现安全系统。

## 建议一：把权限与沙箱组合成“运行预设”

**优先级：P0；预计 0.5～1 天。**

### 借鉴 Codex

Codex 将审批策略和沙箱策略作为明确的运行参数：用户可以选择只读、工作区写入或完全访问，也可以选择遇到风险时询问、失败时询问或不再询问。重要的是，运行状态会在界面和命令行中清楚显示。

### 当前基础

MygoCode 已分别实现：

- `internal/permissions`：`default`、`acceptEdits`、`plan`、`bypassPermissions`；
- `internal/commands/sandbox.go`：自动放行、普通确认、关闭沙箱；
- 路径沙箱和 OS 级 sandbox；
- `/permission`、`/sandbox`、`/status` 命令。

目前用户需要理解多个开关，状态栏也没有将最终有效策略合并成一句话。

### 最小实现

增加三个面向用户的预设，内部仍调用现有逻辑：

| 预设 | 有效行为 | 场景 |
| --- | --- | --- |
| `plan` | 只读工具，命令与写入均询问 | 阅读陌生项目、制定方案 |
| `safe` | 工作区内编辑可确认，危险命令始终拒绝 | 日常开发默认模式 |
| `auto` | 已配置的安全命令自动执行，显式危险规则仍拒绝 | 熟悉项目后的快速迭代 |

建议支持 `/profile plan|safe|auto` 和启动参数 `--profile safe`，在 `/status` 显示类似：`Profile: safe (permission=default, sandbox=regular)`。预设只是配置映射，不要新增一套 Checker。

### 面试可讲的点

“把安全选项从底层开关收敛成用户能理解的运行预设，同时保留底层规则覆盖，减少误配置。”

## 建议二：增加 `exec` 非交互入口和 JSONL 输出

**优先级：P1；预计 1～2 天。**

### 借鉴 Codex

Codex CLI 不只面向交互式 TUI，也适合被脚本或 CI 调用。非交互模式的关键不是再做一个 Agent，而是稳定的退出码、纯文本输出和机器可读事件。

### 最小实现

增加：

```bash
mygocode exec --profile plan "分析当前项目的登录流程"
mygocode exec --format jsonl "运行测试并总结失败原因"
```

实现边界：

1. 复用现有 `config.LoadConfig`、Agent 主循环、权限检查和会话记录，不复制执行逻辑。
2. 默认输出最终文本；`--format jsonl` 按行输出 `text_delta`、`tool_result`、`permission_request`、`error`、`done` 事件。
3. 非交互模式遇到需要确认的操作时立即返回明确错误和非零退出码，不等待一个不存在的 TUI 输入框。
4. 支持 `--max-turns` 和 `--timeout`，避免脚本永远运行。
5. 测试成功、权限拒绝、超时和模型错误四种退出路径。

这项功能可以直接演示“Agent 能进入自动化流程”，比开发完整 App Server 的投入小很多。

## 建议三：增加 `/instructions`，让分层规则可检查

**优先级：P2；预计 0.5 天。**

### 借鉴 Codex

Codex 将项目指令文件作为正式配置入口，并根据工作目录加载不同层级的规则。用户能够知道当前目录继承了哪些约束，减少“模型为什么这样做”的疑问。

### 当前基础

`internal/memory/instructions.go` 已经支持：

- 全局 `~/.mygocode/MYGOCODE.md` 和 `AGENTS.md`；
- 从 Git 根目录到当前目录的逐级发现；
- `.mygocode/INSTRUCTIONS.md` 与本地覆盖文件；
- `@include`、循环检测、最大深度和路径边界检查。

这些实现本身已经值得保留，但目前用户只能看到最终拼接后的效果，无法快速检查来源和优先级。

### 最小实现

增加只读命令 `/instructions`：

```text
Loaded instructions (low -> high priority)
1. ~/.mygocode/AGENTS.md
2. project/AGENTS.md
3. project/backend/MYGOCODE.md
4. project/.mygocode/INSTRUCTIONS.md
```

可选地提供 `/instructions show <index>` 查看单个文件路径和内容摘要。不要在命令中输出完整密钥或本地配置值；只显示文件来源、层级和字符数即可。

## 现有能力，应该展示而非重做

| Codex 方向 | MygoCode 现状 | 建议 |
| --- | --- | --- |
| 分层 `AGENTS.md` | 已有完整发现、合并和 include 安全检查 | 用 `/instructions` 暴露来源，不重写加载器。 |
| Plan / 只读工作流 | 已有 Plan Mode 和权限矩阵 | 纳入 `plan` 运行预设。 |
| 沙箱与审批 | 已有路径沙箱、OS sandbox、人机确认和 Hooks | 统一为 profile，保留细粒度规则。 |
| 会话恢复与压缩 | 已有 JSONL 会话和 compact boundary | 非交互模式继续复用，不新增存储格式。 |
| MCP / 子 Agent | 已有 MCP 延迟加载和子 Agent | 不为追求“像 Codex”再扩展协议。 |

## 不建议现在做

| Codex 的方向 | 不做的原因 |
| --- | --- |
| App Server、JSON-RPC 全量协议和 IDE 生态 | 这是平台级工程，远超日常实习项目的展示需要。 |
| ChatGPT 登录、账号、额度和计费 | 会引入认证、隐私和服务端依赖，与本地 Go Agent 的主线无关。 |
| 原生桌面 App | 当前 TUI + 浏览器远程 UI 已足够证明交互能力。 |
| 完全绕过审批的危险模式 | 不利于项目安全叙事；应保留显式配置和风险边界。 |
| 复制 Codex 的所有事件协议 | 只实现脚本真正需要的少量 JSONL 事件即可。 |

## 推荐实施顺序

1. 先做 `plan / safe / auto` 运行预设和 `/status` 展示。
2. 再做 `mygocode exec` 的纯文本输出，确认退出码与超时行为。
3. 增加 JSONL 事件输出和对应测试。
4. 最后补 `/instructions`，并在 README 中放一段实际命令输出。

## 简历表述参考

功能完成后可表述为：

> 使用 Go 构建本地 AI 编程助手，将权限审批、路径沙箱和 OS 沙箱封装为可切换运行预设；支持交互式 TUI 与 JSONL 非交互执行，并实现分层项目指令的来源追踪和安全 include 校验。

只有在相应功能和测试完成后再使用这段表述；当前项目仍应按源码中已经完成的功能描述。
