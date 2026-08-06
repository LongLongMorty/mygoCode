package eval

// Live end-to-end run of the mini eval harness against the real agent.
// Run with: MYGOCODE_LIVE_TESTS=1 go test ./internal/eval -run TestEvalLive -v -count=1
// Requires config.yaml or .mygocode/config.yaml (with an API key) somewhere
// up the directory tree; skips cleanly otherwise. Each task costs a few LLM
// calls — the full 6-task suite typically takes 5–15 minutes.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mygocode/internal/agent"
	"mygocode/internal/config"
	"mygocode/internal/conversation"
	"mygocode/internal/llm"
	"mygocode/internal/permissions"
	"mygocode/internal/tools"
)

// evalConfig mirrors agent_live_test.loadRealConfig: walk up the tree looking
// for config.yaml / .mygocode/config.yaml with a usable API key.
func evalConfig(t *testing.T) *config.ProviderConfig {
	t.Helper()
	wd, _ := os.Getwd()
	for {
		for _, name := range []string{"config.yaml", filepath.Join(".mygocode", "config.yaml")} {
			cfg, err := config.LoadConfig(filepath.Join(wd, name))
			if err == nil && len(cfg.Providers) > 0 && cfg.Providers[0].ResolveAPIKey() != "" {
				return &cfg.Providers[0]
			}
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	t.Skip("No usable config.yaml / .mygocode/config.yaml with an API key found")
	return nil
}

func TestEvalLiveMiniSuite(t *testing.T) {
	if os.Getenv("MYGOCODE_LIVE_TESTS") != "1" {
		t.Skip("live eval: set MYGOCODE_LIVE_TESTS=1 to run (spends LLM tokens)")
	}
	p := evalConfig(t)
	client, err := llm.NewClient(p, "You are the agent under evaluation. Complete the user's task inside the working directory. Follow instructions exactly and verify your work before finishing.")
	if err != nil {
		t.Fatalf("build client: %v", err)
	}

	run := func(ctx context.Context, workDir, prompt string) (string, error) {
		// All file/bash tools are anchored to the task directory, so the
		// agent's relative paths can never leak into the repo (tools resolve
		// relative paths against the process cwd otherwise).
		reg := tools.CreateDefaultToolsWithWorkDir(workDir).Registry
		ag := agent.New(client, reg, p.Protocol)
		ag.WorkDir = workDir
		ag.Checker = permissions.NewChecker(
			permissions.NewPathSandbox(workDir),
			&permissions.RuleEngine{},
			permissions.ModeBypass, // headless eval: no TUI to answer permission dialogs
		)
		ag.MaxIterations = 20
		conv := conversation.NewManager()
		conv.AddUserMessage(prompt)
		ch := ag.Run(ctx, conv)
		var sb strings.Builder
		for ev := range ch {
			switch e := ev.(type) {
			case agent.StreamText:
				sb.WriteString(e.Text)
			case agent.PermissionRequestEvent:
				e.ResponseCh <- agent.PermAllow
			case agent.ToolResultEvent:
				// Keep a compact trace of what the agent actually did, so a
				// failed task shows the exact command instead of a bare
				// assertion message.
				fmt.Fprintf(&sb, "\n[tool %s] %s", e.ToolName, truncate(e.Output, 200))
			}
		}
		return sb.String(), nil
	}

	h := New(t.TempDir(), DefaultTasks())
	traces := map[string]string{}
	results := h.Run(context.Background(), func(ctx context.Context, workDir, prompt string) (string, error) {
		out, err := run(ctx, workDir, prompt)
		traces[filepath.Base(workDir)] = out
		return out, err
	})

	fmt.Println("\n" + Report(results))
	for _, r := range results {
		if !r.Passed {
			t.Logf("--- trace for %s ---\n%s", r.TaskName, truncate(traces[r.TaskName], 4000))
			t.Errorf("task %s failed: %s", r.TaskName, r.Reason)
		}
	}
}

// TestEvalLiveSingleTask is a quick smoke variant for iterating on one task:
// go test ./internal/eval -run TestEvalLiveSingleTask -v -count=1 -args hello-node
func TestEvalLiveSingleTask(t *testing.T) {
	if os.Getenv("MYGOCODE_LIVE_TESTS") != "1" {
		t.Skip("live eval: set MYGOCODE_LIVE_TESTS=1 to run (spends LLM tokens)")
	}
	args := os.Args
	name := "hello-node"
	if len(args) > 2 && !strings.HasPrefix(args[len(args)-1], "-test.") {
		name = args[len(args)-1]
	}
	p := evalConfig(t)
	client, err := llm.NewClient(p, "You are the agent under evaluation. Complete the user's task inside the working directory. Follow instructions exactly and verify your work before finishing.")
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	var task *Task
	for _, tk := range DefaultTasks() {
		if tk.Name == name {
			task = &tk
			break
		}
	}
	if task == nil {
		t.Fatalf("unknown task %q", name)
	}

	run := func(ctx context.Context, workDir, prompt string) (string, error) {
		// All file/bash tools are anchored to the task directory, so the
		// agent's relative paths can never leak into the repo (tools resolve
		// relative paths against the process cwd otherwise).
		reg := tools.CreateDefaultToolsWithWorkDir(workDir).Registry
		ag := agent.New(client, reg, p.Protocol)
		ag.WorkDir = workDir
		ag.Checker = permissions.NewChecker(
			permissions.NewPathSandbox(workDir),
			&permissions.RuleEngine{},
			permissions.ModeBypass,
		)
		ag.MaxIterations = 20
		conv := conversation.NewManager()
		conv.AddUserMessage(prompt)
		ch := ag.Run(ctx, conv)
		var sb strings.Builder
		for ev := range ch {
			switch e := ev.(type) {
			case agent.StreamText:
				sb.WriteString(e.Text)
			case agent.PermissionRequestEvent:
				e.ResponseCh <- agent.PermAllow
			case agent.ToolResultEvent:
				fmt.Fprintf(&sb, "\n[tool %s] %s", e.ToolName, truncate(e.Output, 200))
			}
		}
		return sb.String(), nil
	}

	var trace string
	h := New(t.TempDir(), []Task{*task})
	results := h.Run(context.Background(), func(ctx context.Context, workDir, prompt string) (string, error) {
		out, err := run(ctx, workDir, prompt)
		trace = out
		return out, err
	})
	fmt.Println("\n" + Report(results))
	if !results[0].Passed {
		t.Logf("agent trace:\n%s", truncate(trace, 4000))
		t.Errorf("task %s failed: %s", results[0].TaskName, results[0].Reason)
	}
}
