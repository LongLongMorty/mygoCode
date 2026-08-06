## 精简版**Terminal-Bench (Terminal Agent Benchmark)** 

设计一个精简版的 **Terminal-Bench (Terminal Agent Benchmark)** 是非常明智的思路。标准 Terminal-Bench 2.0 包含近 90 个复杂任务，运行成本高、评测耗时长。对于自研 Coding Agent 的开发与迭代，更需要一个评测快速、成本可控、覆盖核心链路（Fast Feedback Loop）的“基准小工具集”。

## 一、 精简版设计方案：推荐规模与架构

### 1. 任务数量：推荐 **20 ~ 25 条**

- **理由**：
  - **快速迭代**：20~25 个任务，在 4~8 并发的 Docker 沙箱中运行，可在 **5~10 分钟内完成全量评测**，非常适合接入 CI/CD 或作为开发时的日常 Regression Test。
  - **统计有效性**：单次测试中，每对一个任务增加大约 4% 的准确率，能够拉开不同版本 Agent 之间的差距。
  - **覆盖率**：足以覆盖 Agent 在终端交互中最核心的 5 大能力维度。

### 2. 核心架构设计（与 Terminal-Bench 一致）

结构上保持简单的三要素，不要搞复杂：

``` markdown
benchmark/
├── task_01_git_merge/
│   ├── instruction.md   # 给 Agent 的自然语言任务描述
│   ├── docker/          # 初始化好的 Dockerfile / 环境
│   └── verify.sh        # 隐藏在沙箱外部/外部挂载的断言脚本 (返回 0 表示 PASS)
```

## 二、 应该保留哪些任务类别？（5 大核心分类）

根据 Coding Agent 在真实终端中最高频、最容易断裂的链路，建议精简版的 **25 条任务** 按以下比例划分：

### 1. 环境搭建与依赖编译（Setup & Build）—— **6 条**

- **重要性**：Agent 能否自主解决“环境配置失败”、“缺失 C/C++ 动态库”、“Python/Go 版本冲突”是其能否独立工作的基石。
- **推荐保留的具体任务**：
  - **Python 依赖冲突**：在一个已经损坏或包含不兼容 pip 包的环境中修复依赖并安装指定的包。
  - **C/C++ 或 Go 项目编译**：编译一个缺少某些系统库（如 `libssl-dev` 或 `cmake`）的项目，要求 Agent 自行用 `apt`/`yum` 补充依赖并编译成功。
  - **多语言环境切换**：在一个 Node.js/Python 混合项目中使用 pyenv/nvm 切换到特定版本运行脚本。
  - **Docker-in-Docker / Compose**：通过命令行执行 `docker compose up` 启动一个多容器服务并确保可用。

### 2. 深入调试与 Bug 修复（Debugging & Diagnosis）—— **6 条**

- **重要性**：Terminal 相对纯代码补全的核心优势在于“看日志、定位报错、动态修复”。
- **推荐保留的具体任务**：
  - **端口占用与进程诊断**：某个服务启动报错“Port in use”，Agent 需用 `lsof`/`netstat`/`kill` 定位并清理进程，然后启动服务。
  - **日志分析与崩溃定位**：给出一个崩溃的后端服务日志（如 OOM 或空指针），Agent 需搜寻 `/var/log` 或日志文件，定位 Bug 并修改代码。
  - **环境变量缺失**：服务缺失 `DATABASE_URL` 等关键环境变量，Agent 需读取配置模板，正确配置 `.env` 或 export 变量。
  - **网络连通性/防火墙调试**：本地服务无法访问，需检查 DNS 配置、`curl` 诊断、或配置代理/hosts 文件。

### 3. Git 高级版本控制（Git & VCS Ops）—— **4 条**

- **重要性**：真实开发中，Agent 需要处理复杂的 Git 协同，而不是仅做 `git commit`。
- **推荐保留的具体任务**：
  - **Git 冲突解决 (Merge Conflict)**：在给定分支上 `git merge` 并解决复杂的代码冲突，保证测试通过。
  - **历史历史记录重构 (Rebase/Cherry-pick)**：将特定的 Commit 提取到新分支，或回滚引发 Bug 的 Commit（Revert）。
  - **Git 子模块/Submodule 修复**：解决未递归克隆（Submodule missing）导致的编译报错。

### 4. 系统配置与文件处理（CLI Data Processing）—— **5 条**

- **重要性**：考察 Agent 的 Bash 原生工具链使用能力（`awk`, `sed`, `jq`, `find`, `grep` 等）。
- **推荐保留的具体任务**：
  - **JSON/YAML 配置文件批量改写**：要求 Agent 使用 `jq` 或 Python 脚本更新一个嵌套极深的数据配置文件。
  - **大规模文件查找与清洗**：在复杂的目录树下，查找所有带有特定编码错误的文件并批量替换字符串。
  - **数据库 Migration / Seed 数据**：运行数据库迁移脚本，并使用 CLI 工具注入测试种子数据。

### 5. 跨工具与部署自动化（Deploy & Automation）—— **4 条**

- **重要性**：验证 Agent 能否把多个命令串联成流水线。
- **推荐保留的具体任务**：
  - **编写简易 Makefile / Shell 部署脚本**：根据需求撰写一个自动打包、构建、校验的一键脚本。
  - **定时任务与后台进程**：配置一个 `systemd` 服务或 `nohup` / `tmux` 后台任务，要求重启后仍可自动运行。
  - **HTTP 服务部署**：用 Nginx 反向代理一个本地 Python/Go 服务，要求配置完成且 `curl http://localhost` 能够正确返回。

## 三、 剪裁（剔除）哪些内容？

为了保持精简，以下 Terminal-Bench 2.0 中涉及的类别**建议直接砍掉**：

1. **CTF / 网络安全破解类 (Cybersecurity Challenge)**：
   - *理由*：除非你在做专注于 Security 的 Agent，否则这些任务指令极度弯弯绕绕（如逆向工程、缓冲区溢出），非通用 Coding/DevOps Agent 的核心场景。
2. **极端的算法与数学计算任务**：
   - *理由*：应该交给代码生成的基准（如 HumanEval）去测，在 Terminal 测纯算法会导致沙箱运行时间变长且意义不大。
3. **主观性强/无确定性断言的任务**：
   - *理由*：避免“优化这个 Bash 脚本使其更美观”这类任务，只留“最终 `verify.sh` 返回 0/1”的硬指标。

## 四、 极简基准集的构建建议（经验规则）

1. **验证脚本（`verify.sh`）一定要黑盒化**：
   - 不要把测试用例和校验逻辑写入给 Agent 的 `instruction.md` 中。
   - 校验脚本应直接检查**系统状态**（例如：`nc -z localhost 8080`，或者 `pytest` 的返回码），而不是仅看 Agent 输出了什么。
2. **防止 Agent 破坏沙箱环境**：
   - 限制 Agent 的交互最大轮数（建议单任务 **30~50 轮**）与单次命令超时时间（如 60s），防止死循环卡死整个评测流程。
3. **保持基准版本稳定（Frozen Set）**：
   - 选定这 25 个任务后，**不要频繁改动它们**，这样才能客观衡量你的 Coding Agent 在 Weekly/Daily 迭代中的 Prompt、Tool Call 和 Base Model 的能力变化。

## 五、重新设计的精简任务集（20 条，已实现）

基于以上设计原则与本机环境约束，任务集落地为 **20 条、5 大分类**，在 `internal/eval/` 中实现（2026-08-05 实测通过）。

### 环境约束与剪裁说明

本机工具链现状：Docker Desktop 已安装且 **daemon 运行中**（node:18-alpine 基础镜像约 10s 可拉取）；无 apt；node / git / go 工具链可用。`bash` 已修复（见下）。因此：

- **剪掉**：系统库安装（`apt`/`yum`）、systemd 服务、Nginx 部署、`lsof`/`netstat` 进程诊断类任务——依赖 apt 或系统级命令，本机不可复现（对应第三节"剪裁"精神）。
- **保留**：围绕 node / git / go / docker 四种工具链的任务，验证全部**黑盒化**——harness 直接检查系统状态（运行脚本断言、读文件断言、HTTP 请求断言、`docker build`/`compose` 真实构建与请求），不依赖 Agent 的任何自我陈述。Docker 任务的构建/运行全部由 harness 执行。

**Bash 工具环境问题（已修复，2026-08-05）**：Windows 上 `bash` 在 PATH 中解析为 `C:\Windows\System32\bash.exe`（WSL 启动器），启动的是 PATH 为空、文件系统隔离的 Linux 子系统，导致 Agent 的 Bash 工具执行任何 Windows 命令都 `command not found`。修复：`internal/tools/bash.go` 的 `resolveBashPath()` 在 Windows 上优先使用 Git for Windows 的 bash（继承 Windows PATH，node/git 可用），并显式拒绝 WSL 启动器（返回清晰错误提示安装 Git for Windows）。有单测 `TestResolveBashPath` / `TestBashToolRunsNode` 守护。此前任务集为绕开此问题采用 outcome-driven 设计（验证由 harness 独立执行），该设计保留（更稳健），不因修复而回退。
- **保留**：围绕 node / git / go 三种可用工具链的任务，验证全部**黑盒化**——harness 直接检查系统状态（运行脚本断言、读文件断言、HTTP 请求断言），不依赖 Agent 的任何自我陈述。

### 分类一：环境搭建与依赖编译（Setup & Build）—— 4 条

| # | 任务 | 介绍 | 验证方式（黑盒） |
| --- | --- | --- | --- |
| 1 | `hello-node` | **项目文件创建**：在空目录中创建 hello.js 并写入指定输出语句。考察最小文件创建链路。 | `node hello.js` 输出包含指定内容 |
| 2 | `go-build-fix` | **Go 项目编译修复**：main.go 导入了不存在的 `evalproj/util` 包，Agent 需补全缺失包并确保整体可编译。对应"缺失依赖导致编译失败"的经典场景。 | `go build ./...` 成功且 `go run .` 输出正确 |
| 3 | `node-project-fix` | **Node 项目结构修复**：main.js 依赖缺失的 `lib/helper.js`，Agent 需按调用方约定补全模块。考察"读懂调用方、补齐被依赖模块"的能力。 | `node main.js` 输出指定值 |
| 4 | `docker-build-fix` | **Docker 构建修复**：Dockerfile 复制了不存在的文件导致 `docker build` 失败，Agent 需读构建错误并修复。对应"容器构建失败"场景。 | `docker build` 成功 + `docker run` 输出正确 |

### 分类二：深入调试与 Bug 修复（Debugging & Diagnosis）—— 5 条

| # | 任务 | 介绍 | 验证方式（黑盒） |
| --- | --- | --- | --- |
| 4 | `fix-failing-test` | **逻辑 Bug 修复**：add 函数实现为减法，测试失败。Agent 读代码、定位、修复，外部跑测试判定。SWE-bench 风格（测试通过即成功）。 | `node test.js` 输出 PASS |
| 5 | `fix-syntax-error` | **语法错误定位**：`functoin` 拼写错误导致模块无法加载。考察解析期错误的诊断能力。 | `node main.js` 输出正确问候语 |
| 6 | `log-crash-diagnosis` | **日志驱动调试**：crash.log 含堆栈（对 null 调 toUpperCase），Agent 需读日志 → 定位代码行 → 修复（过滤 null）。考察"看日志、定位、修复"的终端调试闭环。 | `node main.js` 输出 a,b,d |
| 7 | `edit-precision` | **精确编辑**：两个结构相似的函数，只改 second（`return 2`→`return 20`），first 必须原样。考察 EditFile 的上下文精确性，防止误伤。 | 读文件断言：second 已改、first 未动、旧值不存在 |
| 8 | `refactor-export` | **跨文件重构一致性**：重命名导出函数并同步更新所有引用方。考察跨文件修改的引用一致性。 | `node main.js` 输出重构后的正确值 |

### 分类三：Git 高级版本控制（Git & VCS Ops）—— 3 条

| # | 任务 | 介绍 | 验证方式（黑盒） |
| --- | --- | --- | --- |
| 9 | `git-commit` | **基础提交**：初始化仓库（Setup 完成 init/config），Agent 提交文件且 commit message 含关键字。考察 git 基础链路。 | `git log` 含关键字 + 工作区干净 |
| 10 | `git-merge-conflict` | **冲突解决**：两个分支修改同一行产生真实冲突，Agent 执行 merge 并解决冲突、完成提交。考察真实协同场景。 | 无 unmerged 状态 + `node test.js` PASS |
| 11 | `git-revert-bug` | **历史回滚**：历史中某个提交引入 bug，Agent 需用 git 定位（而非手动改代码）并 revert，使测试恢复通过。考察历史诊断与回滚操作。 | `node test.js` 输出 PASS |

### 分类四：系统配置与文件处理（CLI Data Processing）—— 3 条

| # | 任务 | 介绍 | 验证方式（黑盒） |
| --- | --- | --- | --- |
| 12 | `config-update` | **JSON 配置精确改写**：修改嵌套字段（server.timeout 30→60），必须保持 JSON 合法且不动其他字段。 | `node` 解析 JSON 断言字段值 + 其他字段原样 |
| 13 | `search-and-fix` | **搜索定位修改**：magic 函数藏于 src 多文件之一，Agent 用搜索定位并修复。考察检索能力。 | `node main.js` 输出正确值 |
| 14 | `batch-string-replace` | **批量替换防误伤**：目录树中批量替换 OLD_TOKEN→NEW_TOKEN，但 `OLD_TOKEN_SAFE` 是不同标识符，绝不能改。考察精确批量操作。 | 逐文件断言：目标文件已换、保护文件原样 |

### 分类五：跨工具与部署自动化（Deploy & Automation）—— 5 条

| # | 任务 | 介绍 | 验证方式（黑盒） |
| --- | --- | --- | --- |
| 15 | `cli-countdown` | **CLI 工具实现**：从零实现带参数处理的命令行工具（倒计时输出）。考察参数解析 + 输出格式。 | `node countdown.js 3` 输出精确三行 |
| 16 | `write-http-server` | **HTTP 服务实现**：编写监听 8123 端口的 HTTP 服务，harness 启动进程并发起真实 HTTP 请求验证。考察"实现一个可运行的服务"。 | harness 启动 → `GET /` 返回 200 + 指定 JSON |
| 17 | `write-tests` | **测试编写**：为已有 calculator 编写测试，断言覆盖三函数。考察测试能力与断言正确性。 | `node test.js` 输出 PASS |
| 18 | `docker-run-node` | **容器化应用**：从零写 Dockerfile（node:18-alpine）+ 应用文件，harness 真实 `docker build` + `docker run` 验证。考察"容器化一个应用"的完整链路。 | `docker build` 成功 + `docker run` 输出正确 |
| 19 | `docker-compose-up` | **Compose 编排**：从零写 Dockerfile + 应用 + docker-compose.yml（端口映射），harness `docker compose up -d --build` 并 HTTP 请求映射端口验证，随后自动 `down -v` 清理。考察"服务编排"能力。 | compose 启动 → `GET :8124/` 返回 200 + 指定 JSON |

## 六、与既有 eval harness 的整合

上述任务集在 `internal/eval/` 中实现，与设计文档的结构一一对应：

| 设计文档 | 实现 |
| --- | --- |
| `task_N_name/instruction.md` | `Task.Prompt`（自然语言指令） |
| `docker/`（初始化环境） | `Task.Setup`（Go 代码构造隔离目录，每任务独立子目录） |
| `verify.sh`（黑盒断言，返回 0/1） | `Task.Verify`（Go 状态断言，返回 ok+reason） |

- **Runner 抽象**：harness 与 LLM 解耦——单测用 stub Runner，live 用真实 agent（`eval_live_test.go`）。
- **门控**：live 测试需 `MYGOCODE_LIVE_TESTS=1`（消耗 token）；harness 单测无需 API key。
- **Docker 任务前置条件**：`docker-build-fix` / `docker-run-node` / `docker-compose-up` 需要 Docker daemon 运行（Docker Desktop 启动）；docker 命令均带超时（build 180s / compose up 240s），镜像构建失败不会挂起整套评测。
- **用法**：

  ```bash
  # 全套 20 条（约 9-10 分钟，真实 LLM + Docker daemon）
  MYGOCODE_LIVE_TESTS=1 go test ./internal/eval -run TestEvalLiveMiniSuite -v -count=1

  # 单任务迭代
  MYGOCODE_LIVE_TESTS=1 go test ./internal/eval -run TestEvalLiveSingleTask -v -count=1 -args git-merge-conflict

  # 仅 harness 单测（不需要 API key）
  go test ./internal/eval
  ```

## 七、实测结果（2026-08-05，claude-sonnet-4-6 经代理）

**20/20 通过（100%）**，总耗时 9m08s（含 Docker 真实构建）：

```
[PASS] hello-node (8.4s)           [PASS] git-commit (20.0s)
[PASS] go-build-fix (17.9s)        [PASS] git-merge-conflict (64.1s)
[PASS] node-project-fix (17.4s)    [PASS] git-revert-bug (42.9s)
[PASS] docker-build-fix (36.5s)    [PASS] config-update (13.7s)
[PASS] fix-failing-test (19.8s)    [PASS] search-and-fix (17.4s)
[PASS] fix-syntax-error (120.9s)   [PASS] batch-string-replace (38.1s)
[PASS] log-crash-diagnosis (20.9s) [PASS] cli-countdown (8.6s)
[PASS] edit-precision (16.7s)      [PASS] write-http-server (18.8s)
[PASS] refactor-export (16.8s)     [PASS] docker-run-node (15.4s)
                                   [PASS] docker-compose-up (17.6s)
                                   [PASS] write-tests (16.7s)
```

Docker 任务均通过：Agent 写的 Dockerfile 一次构建成功（docker-build-fix 修复了缺失 COPY；docker-run-node 完整容器化；docker-compose-up 编排 + 端口映射经真实 HTTP 请求验证，`down -v` 自动清理）。

**历史过程记录**（17 任务版时发现并修复的两个任务设计问题，均非 Agent 能力问题）：

1. **git 任务分支名**：本机 `git init` 默认分支为 `master`，任务 Setup 中 `git checkout main` 失败导致环境构建失败。修复：`gitSetup` 统一 `git branch -m main`。
2. **日志任务断言歧义**：`log-crash-diagnosis` 的 Prompt 未指定期望输出，Agent 选择"null 转空串"方案（输出 `A,B,,D`）而断言期望"过滤 null"（`a,b,d`）——两种方案都合理。修复：Prompt 明确期望输出与约束（跳过 null、保持小写）。

> 备注：任务集已满足"Frozen Set"要求，后续迭代不建议频繁改动。Docker 任务对 daemon 与镜像拉取网络有依赖（node:18-alpine 本机约 10s）；daemon 未运行时这三个任务会失败，属环境问题而非 Agent 能力问题。