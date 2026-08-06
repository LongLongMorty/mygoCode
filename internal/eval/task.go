// Package eval implements a lightweight, Terminal-Bench-style evaluation
// harness for the agent: a set of real tasks, each with an isolated working
// directory, an execution-based verification step, and a success-rate report.
//
// The harness itself is LLM-agnostic — callers inject a Runner (a real agent
// in live mode, a stub in unit tests). Task verifications are state-based
// assertions (run a script, check its output), never LLM judges.
//
// The task set follows "精简版Terminal-Bench" design: 17 tasks across the
// 5 core capability categories (Setup & Build, Debugging & Diagnosis,
// Git & VCS Ops, CLI Data Processing, Deploy & Automation), each with an
// outcome-driven state assertion — the agent is free to accomplish the task
// however it likes; only the final state is judged.
package eval

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Task is one eval task: a prompt handed to the agent, optional environment
// setup, and a state-based verifier that decides pass/fail.
type Task struct {
	Name    string
	Prompt  string
	Setup   func(dir string) error
	Verify  func(dir string) (bool, string)
	Timeout time.Duration
}

// DefaultTimeout caps tasks that don't specify one.
const DefaultTimeout = 5 * time.Minute

// runInDir executes cmd with args in dir, returning trimmed combined output.
func runInDir(dir, cmd string, args ...string) (string, error) {
	c := exec.Command(cmd, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// runInDirTimeout is runInDir with a deadline — docker build/pull/compose can
// take minutes and must not hang the whole suite.
func runInDirTimeout(dir string, timeout time.Duration, cmd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// mustWrite writes a file (creating parents) into dir; returns the task setup
// error. Used by task Setup functions.
func mustWrite(dir, rel, content string) error {
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// outputContains verifies that running `cmd args...` in dir produces output
// containing want. Returns (ok, human-readable reason).
func outputContains(dir, want, cmd string, args ...string) (bool, string) {
	out, err := runInDir(dir, cmd, args...)
	if err != nil {
		return false, cmd + " failed: " + err.Error() + " (output: " + truncate(out, 120) + ")"
	}
	if !strings.Contains(out, want) {
		return false, cmd + " output missing " + strconvQuote(want) + ": " + truncate(out, 120)
	}
	return true, ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func strconvQuote(s string) string {
	return `"` + s + `"`
}

// gitSetup initialises a repo with a local identity and a main branch (git
// init's default branch name varies by configuration — master on many
// machines — so we normalise to main).
func gitSetup(dir string) error {
	steps := [][]string{
		{"git", "init", "-q"},
		{"git", "config", "user.email", "eval@mygocode.local"},
		{"git", "config", "user.name", "eval"},
		{"git", "branch", "-m", "main"},
	}
	for _, s := range steps {
		if _, err := runInDir(dir, s[0], s[1:]...); err != nil {
			return err
		}
	}
	return nil
}

// DefaultTasks returns the built-in task set, organised by the 5 capability
// categories of the 精简版 Terminal-Bench design. All tasks are offline
// (node / git / go only) and execution-verifiable.
func DefaultTasks() []Task {
	return []Task{
		// ---------------------------------------------------------------
		// Category 1: Setup & Build — 环境搭建与依赖编译
		// ---------------------------------------------------------------

		{
			Name:   "hello-node",
			Prompt: "创建一个 hello.js 文件，内容是 console.log('Hello from mygocode eval')。只创建文件即可；运行验证会由外部完成（如果你的运行环境不可用，不要反复尝试运行命令）。",
			Verify: func(dir string) (bool, string) {
				return outputContains(dir, "Hello from mygocode eval", "node", "hello.js")
			},
			Timeout: 3 * time.Minute,
		},
		{
			Name: "go-build-fix",
			Setup: func(dir string) error {
				if err := mustWrite(dir, "go.mod", "module evalproj\n\ngo 1.21\n"); err != nil {
					return err
				}
				return mustWrite(dir, "main.go", "package main\n\nimport (\n\t\"fmt\"\n\n\t\"evalproj/util\"\n)\n\nfunc main() {\n\tfmt.Println(util.Greet(\"eval\"))\n}\n")
			},
			Prompt: "这是一个 Go 项目：main.go 导入了 evalproj/util 包，但 util 包不存在，所以 go build 会失败。创建 util/util.go，提供 Greet(name string) string 函数，返回 'Hello, ' + name + '!'。只创建文件即可；编译验证会由外部完成。",
			Verify: func(dir string) (bool, string) {
				out, err := runInDir(dir, "go", "build", "./...")
				if err != nil {
					return false, "go build failed: " + err.Error() + " (output: " + truncate(out, 200) + ")"
				}
				out, err = runInDir(dir, "go", "run", ".")
				if err != nil {
					return false, "go run failed: " + err.Error() + " (output: " + truncate(out, 200) + ")"
				}
				if !strings.Contains(out, "Hello, eval!") {
					return false, "go run output missing 'Hello, eval!': " + truncate(out, 200)
				}
				return true, ""
			},
			Timeout: 5 * time.Minute,
		},
		{
			Name: "node-project-fix",
			Setup: func(dir string) error {
				if err := mustWrite(dir, "package.json", "{\n  \"name\": \"eval-demo\",\n  \"main\": \"main.js\"\n}\n"); err != nil {
					return err
				}
				return mustWrite(dir, "main.js", "const { helper } = require('./lib/helper.js');\nconsole.log(helper());\n")
			},
			Prompt: "项目跑不起来：main.js 依赖 lib/helper.js，但该文件不存在。创建 lib/helper.js，导出 helper 函数，返回字符串 'ready'。只创建文件即可；运行验证会由外部完成。",
			Verify: func(dir string) (bool, string) {
				return outputContains(dir, "ready", "node", "main.js")
			},
			Timeout: 4 * time.Minute,
		},
		{
			Name: "docker-build-fix",
			Setup: func(dir string) error {
				if err := mustWrite(dir, "Dockerfile", "FROM node:18-alpine\nWORKDIR /app\nCOPY app.js /app/\nCOPY missing.js /app/\nCMD [\"node\", \"app.js\"]\n"); err != nil {
					return err
				}
				return mustWrite(dir, "app.js", "console.log('hello build');\n")
			},
			Prompt: "docker build 会失败：Dockerfile 复制了一个不存在的文件 missing.js。修复 Dockerfile（把有问题的行删掉或改正），使 docker build 成功。构建和运行会由外部执行（你不需要也无法运行 docker，仔细读文件即可）。",
			Verify: func(dir string) (bool, string) {
				if _, err := runInDirTimeout(dir, 180*time.Second, "docker", "build", "-q", "-t", "eval-docker-build-fix", "."); err != nil {
					return false, "docker build failed: " + err.Error()
				}
				out, err := runInDirTimeout(dir, 60*time.Second, "docker", "run", "--rm", "eval-docker-build-fix")
				if err != nil {
					return false, "docker run failed: " + err.Error() + " (output: " + truncate(out, 120) + ")"
				}
				if !strings.Contains(out, "hello build") {
					return false, "docker run output missing 'hello build': " + truncate(out, 120)
				}
				return true, ""
			},
			Timeout: 6 * time.Minute,
		},

		// ---------------------------------------------------------------
		// Category 2: Debugging & Diagnosis — 深入调试与 Bug 修复
		// ---------------------------------------------------------------

		{
			Name: "fix-failing-test",
			Setup: func(dir string) error {
				if err := mustWrite(dir, "add.js", "function add(a, b) {\n  return a - b;\n}\nmodule.exports = { add };\n"); err != nil {
					return err
				}
				return mustWrite(dir, "test.js", "const { add } = require('./add.js');\n"+
					"if (add(2, 3) === 5 && add(10, 4) === 14) { console.log('PASS'); } else { console.log('FAIL'); process.exit(1); }\n")
			},
			Prompt: "add.js 中有一个 bug：add 函数应该返回两数之和，当前实现却是相减。修复它。测试会由外部运行（如果你的运行环境不可用，请仔细阅读代码确保修复正确，不要反复尝试运行命令）。",
			Verify: func(dir string) (bool, string) {
				return outputContains(dir, "PASS", "node", "test.js")
			},
			Timeout: 5 * time.Minute,
		},
		{
			Name: "fix-syntax-error",
			Setup: func(dir string) error {
				if err := mustWrite(dir, "broken.js", `functoin greet(name) {
  return 'Hello, ' + name + '!';
}
module.exports = { greet };
`); err != nil {
					return err
				}
				return mustWrite(dir, "main.js", "const { greet } = require('./broken.js');\nconsole.log(greet('World'));\n")
			},
			Prompt: "broken.js 里有语法错误（函数声明写错了），导致 main.js 无法运行。修复 broken.js（只改这一个文件），使 node main.js 能输出 Hello, World!。运行验证会由外部完成。",
			Verify: func(dir string) (bool, string) {
				return outputContains(dir, "Hello, World!", "node", "main.js")
			},
			Timeout: 4 * time.Minute,
		},
		{
			Name: "log-crash-diagnosis",
			Setup: func(dir string) error {
				if err := mustWrite(dir, "app.js", `function processItem(item) {
  return item.toUpperCase();
}

function run() {
  const items = ['a', 'b', null, 'd'];
  return items.map(processItem).join(',');
}

module.exports = { run };
`); err != nil {
					return err
				}
				if err := mustWrite(dir, "main.js", "const { run } = require('./app.js');\nconsole.log(run());\n"); err != nil {
					return err
				}
				return mustWrite(dir, "crash.log", `[ERROR] 2026-08-05T10:00:00Z app crashed
TypeError: Cannot read properties of null (reading 'toUpperCase')
    at processItem (app.js:2:16)
    at Array.map (<anonymous>)
    at run (app.js:8:16)
`)
			},
			Prompt: "程序崩溃了。crash.log 里有堆栈信息：processItem 在 app.js 第 2 行对 null 调用 toUpperCase 导致崩溃。分析日志，修复 app.js，使 node main.js 输出 a,b,d（即跳过/过滤掉数组中的 null 项，保持其他项原样小写，不做大小写转换）。运行验证会由外部完成。",
			Verify: func(dir string) (bool, string) {
				return outputContains(dir, "a,b,d", "node", "main.js")
			},
			Timeout: 5 * time.Minute,
		},
		{
			Name: "edit-precision",
			Setup: func(dir string) error {
				return mustWrite(dir, "module.js", `function first() {
  // TODO: implement
  return 1;
}

function second() {
  // TODO: implement
  return 2;
}

module.exports = { first, second };
`)
			},
			Prompt: "module.js 中有两个结构相似的函数（first 和 second），各自带一个 TODO 注释。只修改 second 函数：把它的 return 2 改为 return 20。编辑时必须带足够上下文以精确定位，绝不能改动 first 函数（它的 return 1 必须保持原样）。",
			Verify: func(dir string) (bool, string) {
				data, err := os.ReadFile(filepath.Join(dir, "module.js"))
				if err != nil {
					return false, "read module.js: " + err.Error()
				}
				content := string(data)
				// Use ;-terminated exact matches: "return 2" is a substring of
				// the correct "return 20;", so naive Contains would false-fail.
				if !strings.Contains(content, "return 20;") {
					return false, "second function not updated: missing 'return 20;'"
				}
				if strings.Contains(content, "return 2;") {
					return false, "old 'return 2;' still present"
				}
				if !strings.Contains(content, "return 1;") {
					return false, "first function was modified: 'return 1;' missing"
				}
				return true, ""
			},
			Timeout: 4 * time.Minute,
		},
		{
			Name: "refactor-export",
			Setup: func(dir string) error {
				if err := mustWrite(dir, "utils.js", "function double(x) { return x * 2; }\nmodule.exports = { double };\n"); err != nil {
					return err
				}
				return mustWrite(dir, "main.js", "const { double } = require('./utils.js');\nconsole.log(double(6));\n")
			},
			Prompt: "把 utils.js 中的 double 函数改名为 triple 并改为乘以 3，同时更新 main.js 中的引用（它 require 了 double）。只修改文件即可；运行验证会由外部完成。",
			Verify: func(dir string) (bool, string) {
				return outputContains(dir, "18", "node", "main.js")
			},
			Timeout: 5 * time.Minute,
		},

		// ---------------------------------------------------------------
		// Category 3: Git & VCS Ops — Git 高级版本控制
		// ---------------------------------------------------------------

		{
			Name: "git-commit",
			Setup: func(dir string) error {
				if err := gitSetup(dir); err != nil {
					return err
				}
				return mustWrite(dir, "README.md", "# Eval task\n")
			},
			Prompt: "这是一个 git 仓库（已初始化）。把当前目录下的文件（README.md）提交到 git，commit message 中必须包含 init 字样。提交后无需其他操作。",
			Verify: func(dir string) (bool, string) {
				log, err := runInDir(dir, "git", "log", "-1", "--format=%s")
				if err != nil {
					return false, "git log failed: " + err.Error()
				}
				if !strings.Contains(log, "init") {
					return false, "commit message missing 'init': " + log
				}
				status, _ := runInDir(dir, "git", "status", "--porcelain")
				if status != "" {
					return false, "working tree not clean: " + status
				}
				return true, ""
			},
			Timeout: 3 * time.Minute,
		},
		{
			Name: "git-merge-conflict",
			Setup: func(dir string) error {
				if err := gitSetup(dir); err != nil {
					return err
				}
				base := "function add(a, b) {\n  return a + b;\n}\nfunction subtract(a, b) {\n  return a - b;\n}\nmodule.exports = { add, subtract };\n"
				test := "const { add, subtract } = require('./math.js');\n" +
					"if (add(2, 3) === 5 && subtract(10, 4) === 6) { console.log('PASS'); } else { console.log('FAIL'); process.exit(1); }\n"
				if err := mustWrite(dir, "math.js", base); err != nil {
					return err
				}
				if err := mustWrite(dir, "test.js", test); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "add", "-A"); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "commit", "-qm", "base"); err != nil {
					return err
				}
				// feature branch edits the same add() line as main below → conflict.
				if _, err := runInDir(dir, "git", "checkout", "-qb", "feature"); err != nil {
					return err
				}
				feature := strings.Replace(base, "  return a + b;", "  return b + a; // feature", 1)
				if err := mustWrite(dir, "math.js", feature); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "add", "-A"); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "commit", "-qm", "feature change"); err != nil {
					return err
				}
				// main branch edits the same line differently → conflict on merge.
				if _, err := runInDir(dir, "git", "checkout", "-q", "main"); err != nil {
					return err
				}
				main := strings.Replace(base, "  return a + b;", "  return (a + b); // main", 1)
				if err := mustWrite(dir, "math.js", main); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "add", "-A"); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "commit", "-qm", "main change"); err != nil {
					return err
				}
				return nil
			},
			Prompt: "当前在 main 分支。把 feature 分支合并到 main（git merge feature）。两个分支修改了 math.js 的同一行，会产生冲突：解决冲突（保留正确实现：add 返回两数之和），然后完成合并提交。注意 test.js 会由外部运行验证。",
			Verify: func(dir string) (bool, string) {
				status, _ := runInDir(dir, "git", "status", "--porcelain")
				for _, line := range strings.Split(status, "\n") {
					if strings.HasPrefix(line, "UU") || strings.Contains(line, "unmerged") {
						return false, "merge conflict unresolved: " + status
					}
				}
				return outputContains(dir, "PASS", "node", "test.js")
			},
			Timeout: 5 * time.Minute,
		},
		{
			Name: "git-revert-bug",
			Setup: func(dir string) error {
				if err := gitSetup(dir); err != nil {
					return err
				}
				good := "function add(a, b) {\n  return a + b;\n}\nfunction subtract(a, b) {\n  return a - b;\n}\nmodule.exports = { add, subtract };\n"
				test := "const { add, subtract } = require('./math.js');\n" +
					"if (add(2, 3) === 5 && subtract(10, 4) === 6) { console.log('PASS'); } else { console.log('FAIL'); process.exit(1); }\n"
				if err := mustWrite(dir, "math.js", good); err != nil {
					return err
				}
				if err := mustWrite(dir, "test.js", test); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "add", "-A"); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "commit", "-qm", "add math module"); err != nil {
					return err
				}
				// commit 2 introduces the bug
				buggy := strings.Replace(good, "return a - b;", "return a + b;", 1)
				if err := mustWrite(dir, "math.js", buggy); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "add", "-A"); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "commit", "-qm", "add subtract (buggy)"); err != nil {
					return err
				}
				// commit 3 unrelated
				if err := mustWrite(dir, "README.md", "# math\n"); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "add", "-A"); err != nil {
					return err
				}
				if _, err := runInDir(dir, "git", "commit", "-qm", "add readme"); err != nil {
					return err
				}
				return nil
			},
			Prompt: "这个 git 仓库的历史中有一个提交引入了 bug：subtract 函数实现错误（减法变成了加法），导致 test.js 失败。用 git 定位引入 bug 的提交并 revert 它（不要手动直接改代码，用 git 操作）。test.js 会由外部运行验证。",
			Verify: func(dir string) (bool, string) {
				return outputContains(dir, "PASS", "node", "test.js")
			},
			Timeout: 5 * time.Minute,
		},

		// ---------------------------------------------------------------
		// Category 4: CLI Data Processing — 系统配置与文件处理
		// ---------------------------------------------------------------

		{
			Name: "config-update",
			Setup: func(dir string) error {
				return mustWrite(dir, "config.json", `{
  "name": "demo",
  "server": {
    "host": "localhost",
    "port": 8080,
    "timeout": 30
  }
}
`)
			},
			Prompt: "修改 config.json：把 server.timeout 从 30 改为 60。修改后文件必须仍然是合法 JSON，且不能改动其他字段（name、host、port 等保持原样）。",
			Verify: func(dir string) (bool, string) {
				// Parse as JSON (require validates syntax) and check the field.
				return outputContains(dir, "timeout=60", "node", "-e",
					"const c = require('./config.json'); if (c.server && c.server.timeout === 60 && c.server.port === 8080) { console.log('timeout=60'); } else { console.log('BAD'); process.exit(1); }")
			},
			Timeout: 4 * time.Minute,
		},
		{
			Name: "search-and-fix",
			Setup: func(dir string) error {
				files := map[string]string{
					"src/a.js": "function alpha() { return 'a'; }\nmodule.exports = { alpha };\n",
					"src/b.js": "// TODO: fix magic to return 'correct'\nfunction magic() { return 'wrong'; }\nmodule.exports = { magic };\n",
					"src/c.js": "function gamma() { return 'c'; }\nmodule.exports = { gamma };\n",
					"main.js":  "const { magic } = require('./src/b.js');\nconsole.log(magic());\n",
				}
				for rel, content := range files {
					if err := mustWrite(dir, rel, content); err != nil {
						return err
					}
				}
				return nil
			},
			Prompt: "main.js 的输出不对。magic 函数在 src 目录的某个文件里，找到它并修复，使其返回 'correct'。只修改文件即可；运行验证会由外部完成。",
			Verify: func(dir string) (bool, string) {
				return outputContains(dir, "correct", "node", "main.js")
			},
			Timeout: 5 * time.Minute,
		},
		{
			Name: "batch-string-replace",
			Setup: func(dir string) error {
				files := map[string]string{
					"data/a.txt":     "first OLD_TOKEN here\nand OLD_TOKEN again\n",
					"data/b.txt":     "only OLD_TOKEN\n",
					"data/sub/c.txt": "nested OLD_TOKEN file\n",
					"data/keep.txt":  "do not touch OLD_TOKEN_SAFE\n",
				}
				for rel, content := range files {
					if err := mustWrite(dir, rel, content); err != nil {
						return err
					}
				}
				return nil
			},
			Prompt: "data 目录（含子目录）下的多个文本文件里出现了 OLD_TOKEN，需要批量替换为 NEW_TOKEN。注意：data/keep.txt 中的 OLD_TOKEN_SAFE 是一个不同的标识符，绝不能改动它。",
			Verify: func(dir string) (bool, string) {
				checks := []struct {
					rel    string
					has    string
					absent string
				}{
					{"data/a.txt", "NEW_TOKEN", "OLD_TOKEN"},
					{"data/b.txt", "NEW_TOKEN", "OLD_TOKEN"},
					{"data/sub/c.txt", "NEW_TOKEN", "OLD_TOKEN"},
					{"data/keep.txt", "OLD_TOKEN_SAFE", ""},
				}
				for _, c := range checks {
					data, err := os.ReadFile(filepath.Join(dir, c.rel))
					if err != nil {
						return false, "read " + c.rel + ": " + err.Error()
					}
					content := string(data)
					if !strings.Contains(content, c.has) {
						return false, c.rel + " missing " + strconvQuote(c.has) + ": " + truncate(content, 120)
					}
					if c.absent != "" && strings.Contains(content, c.absent) {
						return false, c.rel + " still contains " + strconvQuote(c.absent) + ": " + truncate(content, 120)
					}
				}
				return true, ""
			},
			Timeout: 4 * time.Minute,
		},

		// ---------------------------------------------------------------
		// Category 5: Deploy & Automation — 跨工具与部署自动化
		// ---------------------------------------------------------------

		{
			Name:   "cli-countdown",
			Prompt: "创建 countdown.js：接受一个命令行数字参数 n，输出从 n 到 1 每行一个数字（例如参数 3 输出 3、2、1 三行）。只创建文件即可；运行验证会由外部完成。",
			Verify: func(dir string) (bool, string) {
				out, err := runInDir(dir, "node", "countdown.js", "3")
				if err != nil {
					return false, "node countdown.js failed: " + err.Error()
				}
				lines := strings.Split(out, "\n")
				expected := []string{"3", "2", "1"}
				if len(lines) < 3 || lines[0] != expected[0] || lines[1] != expected[1] || lines[2] != expected[2] {
					return false, "unexpected output: " + truncate(out, 120)
				}
				return true, ""
			},
			Timeout: 4 * time.Minute,
		},
		{
			Name: "write-http-server",
			Setup: func(dir string) error {
				// Pre-write a stub the agent must not break? No — empty dir,
				// the agent writes the whole server from scratch.
				return nil
			},
			Prompt: "创建 server.js：一个 HTTP 服务器，监听 8123 端口，对 GET / 请求返回 JSON：{\"status\": \"ok\"}。只创建文件即可；服务会由外部启动并验证。",
			Verify: func(dir string) (bool, string) {
				cmd := exec.Command("node", "server.js")
				cmd.Dir = dir
				if err := cmd.Start(); err != nil {
					return false, "start server: " + err.Error()
				}
				defer func() {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}()
				client := &http.Client{Timeout: 500 * time.Millisecond}
				for i := 0; i < 12; i++ {
					resp, err := client.Get("http://127.0.0.1:8123/")
					if err == nil {
						body, _ := io.ReadAll(resp.Body)
						resp.Body.Close()
						if resp.StatusCode == http.StatusOK && strings.Contains(string(body), "ok") {
							return true, ""
						}
						return false, "unexpected response: HTTP " + resp.Status + " " + truncate(string(body), 120)
					}
					time.Sleep(500 * time.Millisecond)
				}
				return false, "server did not respond on :8123 within 6s"
			},
			Timeout: 4 * time.Minute,
		},
		{
			Name:   "docker-run-node",
			Prompt: "创建两个文件：Dockerfile（基础镜像 node:18-alpine，把 hello.js 复制进容器，用 CMD 运行 node hello.js）和 hello.js（运行时输出 'Hello from docker'）。构建和运行会由外部执行（你不需要也无法运行 docker，仔细写文件即可）。",
			Verify: func(dir string) (bool, string) {
				if _, err := runInDirTimeout(dir, 180*time.Second, "docker", "build", "-q", "-t", "eval-docker-run-node", "."); err != nil {
					return false, "docker build failed: " + err.Error()
				}
				out, err := runInDirTimeout(dir, 60*time.Second, "docker", "run", "--rm", "eval-docker-run-node")
				if err != nil {
					return false, "docker run failed: " + err.Error() + " (output: " + truncate(out, 120) + ")"
				}
				if !strings.Contains(out, "Hello from docker") {
					return false, "docker run output missing 'Hello from docker': " + truncate(out, 120)
				}
				return true, ""
			},
			Timeout: 6 * time.Minute,
		},
		{
			Name:   "docker-compose-up",
			Prompt: "创建三个文件：Dockerfile（基础镜像 node:18-alpine，复制 app.js 进容器，用 CMD 运行 node app.js，容器内监听 3000 端口）；app.js（HTTP 服务器，监听 3000，对 GET / 返回 JSON {\"ok\": true}）；docker-compose.yml（单个服务 web：构建当前目录，端口映射 8124:3000）。compose 启动与验证会由外部执行（你不需要也无法运行 docker，仔细写文件即可）。",
			Verify: func(dir string) (bool, string) {
				// Bring up the stack; then check the mapped port; always tear down.
				upOut, err := runInDirTimeout(dir, 240*time.Second, "docker", "compose", "-p", "evalcompose", "up", "-d", "--build")
				if err != nil {
					_, _ = runInDirTimeout(dir, 60*time.Second, "docker", "compose", "-p", "evalcompose", "down", "-v")
					return false, "docker compose up failed: " + err.Error() + " (output: " + truncate(upOut, 300) + ")"
				}
				defer func() {
					_, _ = runInDirTimeout(dir, 60*time.Second, "docker", "compose", "-p", "evalcompose", "down", "-v")
				}()
				client := &http.Client{Timeout: 500 * time.Millisecond}
				for i := 0; i < 15; i++ {
					resp, err := client.Get("http://127.0.0.1:8124/")
					if err == nil {
						body, _ := io.ReadAll(resp.Body)
						resp.Body.Close()
						if resp.StatusCode == http.StatusOK && strings.Contains(string(body), "ok") {
							return true, ""
						}
						return false, "unexpected response: HTTP " + resp.Status + " " + truncate(string(body), 120)
					}
					time.Sleep(1 * time.Second)
				}
				return false, "compose service did not respond on :8124 within 15s"
			},
			Timeout: 8 * time.Minute,
		},
		{
			Name: "write-tests",
			Setup: func(dir string) error {
				return mustWrite(dir, "calculator.js", `function add(a, b) { return a + b; }
function subtract(a, b) { return a - b; }
function multiply(a, b) { return a * b; }
module.exports = { add, subtract, multiply };
`)
			},
			Prompt: "为 calculator.js 编写 test.js：测试 add(2, 3) 等于 5、subtract(10, 4) 等于 6、multiply(3, 4) 等于 12；全部通过时输出 PASS 并以退出码 0 结束，任一失败输出 FAIL 并以非零码退出。只创建 test.js；运行验证会由外部完成。",
			Verify: func(dir string) (bool, string) {
				return outputContains(dir, "PASS", "node", "test.js")
			},
			Timeout: 4 * time.Minute,
		},
	}
}
