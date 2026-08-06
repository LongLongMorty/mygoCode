package eval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubRunner records the prompts it received and returns scripted results.
type stubRunner struct {
	prompts []string
	err     error
}

func (s *stubRunner) run(ctx context.Context, workDir, prompt string) (string, error) {
	s.prompts = append(s.prompts, prompt)
	return "done", s.err
}

// TestHarnessIsolation verifies each task runs in its own directory: the
// runner sees a per-task path, and one task's files don't leak into another.
func TestHarnessIsolation(t *testing.T) {
	tasks := []Task{
		{
			Name:   "one",
			Prompt: "task one",
			Setup:  func(dir string) error { return mustWrite(dir, "marker.txt", "one") },
		},
		{
			Name:   "two",
			Prompt: "task two",
		},
	}
	stub := &stubRunner{}
	h := New(t.TempDir(), tasks)
	results := h.Run(context.Background(), stub.run)

	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if len(stub.prompts) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(stub.prompts))
	}
	// Isolation: task "two" must not see task "one"'s marker.
	if _, err := os.Stat(filepath.Join(h.BaseDir, "two", "marker.txt")); !os.IsNotExist(err) {
		t.Errorf("task isolation broken: marker.txt leaked into task 'two'")
	}
	if _, err := os.Stat(filepath.Join(h.BaseDir, "one", "marker.txt")); err != nil {
		t.Errorf("task 'one' marker missing: %v", err)
	}
}

// TestHarnessVerifyAndReport covers the verify → pass/fail flow and the
// report's success-rate math.
func TestHarnessVerifyAndReport(t *testing.T) {
	tasks := []Task{
		{
			Name:   "passing",
			Prompt: "p",
			Setup:  func(dir string) error { return mustWrite(dir, "ok.txt", "yes") },
			Verify: func(dir string) (bool, string) {
				data, err := os.ReadFile(filepath.Join(dir, "ok.txt"))
				return err == nil && string(data) == "yes", "read ok.txt"
			},
		},
		{
			Name:   "failing-verify",
			Prompt: "f",
			Verify: func(dir string) (bool, string) {
				return false, "assertion did not hold"
			},
		},
		{
			Name:   "no-verify",
			Prompt: "n",
		},
		{
			Name:   "agent-error",
			Prompt: "e",
		},
	}
	var prompts []string
	run := func(ctx context.Context, workDir, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		if strings.HasSuffix(prompt, "e") {
			return "", errors.New("boom")
		}
		return "done", nil
	}
	h := New(t.TempDir(), tasks)
	results := h.Run(context.Background(), run)

	if !results[0].Passed {
		t.Errorf("passing task failed: %s", results[0].Reason)
	}
	if results[1].Passed || !strings.Contains(results[1].Reason, "assertion did not hold") {
		t.Errorf("failing-verify = %+v", results[1])
	}
	if !results[2].Passed {
		t.Errorf("no-verify task should pass when the agent succeeds: %+v", results[2])
	}
	if results[3].Passed || !strings.Contains(results[3].Reason, "boom") {
		t.Errorf("agent-error task should fail with the runner error: %+v", results[3])
	}
	if len(prompts) != 4 {
		t.Errorf("runner calls = %d, want 4", len(prompts))
	}

	report := Report(results)
	if !strings.Contains(report, "2/4 passed (50.0%)") {
		t.Errorf("report math wrong:\n%s", report)
	}
	if !strings.Contains(report, "[PASS] passing") || !strings.Contains(report, "[FAIL] agent-error") {
		t.Errorf("report marks wrong:\n%s", report)
	}
}

// TestHarnessAgentErrorPropagates verifies a runner error (e.g. context
// timeout) surfaces as a task failure with the error text, and does not stop
// the rest of the suite.
func TestHarnessAgentErrorPropagates(t *testing.T) {
	tasks := []Task{
		{Name: "boom", Prompt: "x"},
		{Name: "after", Prompt: "y"},
	}
	stub := &stubRunner{err: errors.New("context deadline exceeded")}
	h := New(t.TempDir(), tasks)
	results := h.Run(context.Background(), stub.run)

	if results[0].Passed || !strings.Contains(results[0].Reason, "context deadline exceeded") {
		t.Errorf("error not propagated: %+v", results[0])
	}
	if len(results) != 2 {
		t.Errorf("suite aborted after agent error: %d results", len(results))
	}
	// The second task still ran (runner called twice).
	if len(stub.prompts) != 2 {
		t.Errorf("runner calls = %d, want 2", len(stub.prompts))
	}
}

// TestHarnessTimeoutCutsOff verifies the per-task deadline is enforced when
// the runner ignores the context.
func TestHarnessTimeoutCutsOff(t *testing.T) {
	tasks := []Task{{
		Name:    "slow",
		Prompt:  "s",
		Timeout: 100 * time.Millisecond,
		Verify:  func(dir string) (bool, string) { return true, "" },
	}}
	run := func(ctx context.Context, workDir, prompt string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "done", nil
		}
	}
	h := New(t.TempDir(), tasks)
	start := time.Now()
	results := h.Run(context.Background(), run)
	elapsed := time.Since(start)

	if results[0].Passed {
		t.Errorf("slow task should fail on timeout")
	}
	if !strings.Contains(results[0].Reason, "deadline exceeded") {
		t.Errorf("reason = %q, want deadline exceeded", results[0].Reason)
	}
	if elapsed > 2*time.Second {
		t.Errorf("harness did not cut the task off: took %s", elapsed)
	}
}

// TestDefaultTasksAreOfflineAndVerifiable sanity-checks the built-in task set
// against a stub runner: every task must have a Verify step and a name.
func TestDefaultTasksAreOfflineAndVerifiable(t *testing.T) {
	tasks := DefaultTasks()
	if len(tasks) < 5 {
		t.Fatalf("expected at least 5 built-in tasks, got %d", len(tasks))
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		if task.Name == "" {
			t.Error("task with empty name")
		}
		if seen[task.Name] {
			t.Errorf("duplicate task name %q", task.Name)
		}
		seen[task.Name] = true
		if task.Verify == nil {
			t.Errorf("task %s has no Verify step", task.Name)
		}
		if task.Timeout <= 0 {
			t.Errorf("task %s has no timeout", task.Name)
		}
	}
}
