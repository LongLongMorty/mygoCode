package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mygocode/internal/sandbox"
)

// resolveBashPath returns the bash executable to use for the Bash tool.
//
// On Windows, `bash` in PATH usually resolves to C:\Windows\System32\bash.exe
// — the WSL launcher — which starts a Linux subsystem with an empty PATH and
// a separate filesystem, so agent commands like `node` or `git` fail with
// "command not found" even though they exist on Windows. Git for Windows bash
// runs natively with the Windows PATH inherited, so it is preferred whenever
// installed. A plain PATH lookup is used on non-Windows platforms.
func resolveBashPath() (string, error) {
	if runtime.GOOS == "windows" {
		candidates := []string{
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Program Files\Git\usr\bin\bash.exe`,
			`C:\Program Files (x86)\Git\bin\bash.exe`,
			`C:\Program Files (x86)\Git\usr\bin\bash.exe`,
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		// Fall back to a PATH lookup, but refuse the WSL launcher explicitly
		// so the agent gets a clear message instead of mysterious
		// "command not found" failures inside an empty-PATH Linux shell.
		p, err := exec.LookPath("bash")
		if err != nil {
			return "", fmt.Errorf("bash not found in PATH (install Git for Windows to enable the Bash tool)")
		}
		if strings.EqualFold(p, `C:\Windows\System32\bash.exe`) {
			return "", errors.New("bash resolves to the WSL launcher (C:\\Windows\\System32\\bash.exe), which has an empty PATH and cannot run Windows commands; install Git for Windows to enable the Bash tool")
		}
		return p, nil
	}
	return "bash", nil
}

// commandErrorThresholds 定义特殊命令的退出码阈值，
// 退出码 >= 阈值才视为真正的错误。
// 例如 grep 退出码 1 表示"没有匹配"，不算错误；退出码 2 才是语法/IO 错误。
var commandErrorThresholds = map[string]int{
	"grep": 2, // exit 1 = 无匹配
	"rg":   2, // ripgrep，同 grep
	"diff": 2, // exit 1 = 文件有差异
	"find": 2, // exit 1 = 部分成功（权限不足等）
	"test": 2, // exit 1 = 条件为假
	"[":    2, // test 的别名写法
}

// interpretExitCode 根据命令语义判断退出码是否代表错误。
// 管道场景取最后一个命令（bash 默认以最后一条的退出码作为管道退出码）。
func interpretExitCode(command string, exitCode int) bool {
	if exitCode == 0 {
		return false
	}
	base := extractLastCommand(command)
	if threshold, ok := commandErrorThresholds[base]; ok {
		return exitCode >= threshold
	}
	// 默认：非零即错误
	return true
}

// extractLastCommand 从完整命令字符串中提取管道最后一段的基础命令名。
// 例如 "cat file | grep foo" → "grep"，"timeout 5 rg pattern" → "rg"。
func extractLastCommand(command string) string {
	// 取管道最后一段
	parts := strings.Split(command, "|")
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return ""
	}
	// 取第一个 token 作为命令，再去掉路径前缀
	fields := strings.Fields(last)
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

// exitCodeHint 为特殊命令的非错误退出码提供语义提示。
func exitCodeHint(command string, exitCode int) string {
	base := extractLastCommand(command)
	switch base {
	case "grep", "rg":
		if exitCode == 1 {
			return "no matches found"
		}
	case "diff":
		if exitCode == 1 {
			return "files differ"
		}
	case "find":
		if exitCode == 1 {
			return "some directories were inaccessible"
		}
	case "test", "[":
		if exitCode == 1 {
			return "condition is false"
		}
	}
	return fmt.Sprintf("command failed with exit code %d", exitCode)
}

const maxTimeout = 600

type BashTool struct {
	WorkDir       string
	Sandbox       sandbox.Sandbox // OS 级沙箱实例，nil 表示不启用
	SandboxConfig sandbox.Config  // 沙箱的路径和网络权限配置
}

func (t *BashTool) Name() string { return "Bash" }

func (t *BashTool) Description() string { return BashDescription }

func (t *BashTool) Category() ToolCategory { return CategoryCommand }

func (t *BashTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "Shell command to execute"},
				"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (max 600)", "default": 120},
			},
			"required": []string{"command"},
		},
	}
}

func (t *BashTool) Execute(ctx context.Context, args map[string]any) ToolResult {
	command, _ := args["command"].(string)
	if command == "" {
		return ToolResult{Output: "Error: command is required", IsError: true}
	}

	bashPath, err := resolveBashPath()
	if err != nil {
		return ToolResult{Output: "Error: " + err.Error(), IsError: true}
	}

	timeout := intArg(args, "timeout", 120)
	if timeout > maxTimeout {
		timeout = maxTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// 如果沙箱可用，将命令包装到沙箱内执行
	actualCommand := command
	if t.Sandbox != nil && t.Sandbox.Available() {
		wrapped, err := t.Sandbox.Wrap(command, t.SandboxConfig)
		if err == nil {
			actualCommand = wrapped
		}
	}

	cmd := exec.CommandContext(ctx, bashPath, "-c", actualCommand)
	// stdout 和 stderr 合并到同一个流，与 Claude Code 的 merged fd 对齐
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if t.WorkDir != "" {
		cmd.Dir = t.WorkDir
	}

	err = cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return ToolResult{Output: fmt.Sprintf("Error: command timed out after %ds", timeout), IsError: true}
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == nil {
			return ToolResult{Output: fmt.Sprintf("Error executing command: %s", err), IsError: true}
		}
	}

	var sb bytes.Buffer
	fmt.Fprintf(&sb, "$ %s\n", command)
	if combined.Len() > 0 {
		sb.Write(combined.Bytes())
		if combined.Bytes()[combined.Len()-1] != '\n' {
			sb.WriteByte('\n')
		}
	}
	if exitCode != 0 {
		fmt.Fprintf(&sb, "Exit code %d", exitCode)
		if !interpretExitCode(command, exitCode) {
			fmt.Fprintf(&sb, " (%s)", exitCodeHint(command, exitCode))
		}
	}

	// is_error 只在超时/中断时为 true，正常非零退出码不标 error
	return ToolResult{Output: sb.String(), IsError: false}
}
