package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Runner executes one task: it receives the task's isolated working directory
// and the prompt, and returns the agent's final text (or an error on
// timeout / agent failure). Live mode wires a real agent here; unit tests use
// a stub.
type Runner func(ctx context.Context, workDir, prompt string) (string, error)

// Result is the outcome of a single task run.
type Result struct {
	TaskName string
	Passed   bool
	Reason   string
	Duration time.Duration
}

// Harness runs a task set, each task in its own subdirectory under BaseDir
// (isolation: a failing task cannot contaminate the next one, and no task
// touches the real project).
type Harness struct {
	Tasks   []Task
	BaseDir string
}

// New creates a harness rooted at a caller-owned base directory (typically
// t.TempDir()).
func New(baseDir string, tasks []Task) *Harness {
	return &Harness{Tasks: tasks, BaseDir: baseDir}
}

// Run executes every task sequentially and returns per-task results. Task
// setup errors and agent errors are recorded as failures with reasons, so a
// broken task never aborts the suite.
func (h *Harness) Run(ctx context.Context, run Runner) []Result {
	results := make([]Result, 0, len(h.Tasks))
	for _, task := range h.Tasks {
		dir := filepath.Join(h.BaseDir, task.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			results = append(results, Result{TaskName: task.Name, Passed: false, Reason: "setup: " + err.Error()})
			continue
		}

		if task.Setup != nil {
			if err := task.Setup(dir); err != nil {
				results = append(results, Result{TaskName: task.Name, Passed: false, Reason: "setup: " + err.Error()})
				continue
			}
		}

		timeout := task.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		start := time.Now()
		tctx, cancel := context.WithTimeout(ctx, timeout)
		// Anchor the agent to its task directory: the file tools resolve
		// relative paths against the process cwd (the repo root when tests
		// run), not the task dir, so the prompt must carry the absolute path
		// and the tools' own docs recommend absolute file_path values.
		fullPrompt := fmt.Sprintf("工作目录：%s（涉及文件操作时请使用该目录下的绝对路径）\n\n%s", dir, task.Prompt)
		_, agentErr := run(tctx, dir, fullPrompt)
		cancel()

		passed := false
		reason := ""
		switch {
		case agentErr != nil:
			reason = "agent error: " + agentErr.Error()
		case task.Verify != nil:
			passed, reason = task.Verify(dir)
			if !passed && reason == "" {
				reason = "verification failed"
			}
		default:
			passed = true
		}

		results = append(results, Result{
			TaskName: task.Name,
			Passed:   passed,
			Reason:   reason,
			Duration: time.Since(start),
		})
	}
	return results
}

// Report renders the suite summary: per-task PASS/FAIL lines and the overall
// success rate.
func Report(results []Result) string {
	var sb strings.Builder
	sb.WriteString("=== Mini Eval Harness ===\n")
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
		mark := "PASS"
		if !r.Passed {
			mark = "FAIL"
		}
		fmt.Fprintf(&sb, "[%s] %s (%.1fs)", mark, r.TaskName, r.Duration.Seconds())
		if r.Reason != "" {
			sb.WriteString(" — " + r.Reason)
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "Result: %d/%d passed (%.1f%%), total %s\n",
		passed, len(results), pct(passed, len(results)), formatTotal(results))
	return sb.String()
}

func pct(passed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(passed) * 100 / float64(total)
}

func formatTotal(results []Result) time.Duration {
	var total time.Duration
	for _, r := range results {
		total += r.Duration
	}
	return total
}
