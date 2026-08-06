package tools

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestResolveBashPath verifies the Bash tool never ends up running inside the
// WSL launcher on Windows: Git for Windows bash is preferred when installed,
// and the WSL launcher is refused with a clear error when it's the only
// option.
func TestResolveBashPath(t *testing.T) {
	path, err := resolveBashPath()
	if err != nil {
		t.Fatalf("resolveBashPath: %v", err)
	}
	if runtime.GOOS != "windows" {
		if path != "bash" {
			t.Errorf("non-Windows path = %q, want \"bash\"", path)
		}
		return
	}
	if strings.Contains(strings.ToLower(path), "system32") {
		t.Errorf("resolved bash must not be the WSL launcher (System32), got %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("resolved bash %q does not exist: %v", path, err)
	}
}

// TestBashToolRunsNode is an end-to-end check that the Bash tool can execute
// a real Windows command through its resolved bash. Skips when node is not
// available. Guards against regressions to the WSL-launcher empty-PATH
// failure mode ("node: command not found").
func TestBashToolRunsNode(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	tool := &BashTool{WorkDir: t.TempDir()}
	res := tool.Execute(context.Background(), map[string]any{
		"command": "node --version",
	})
	if res.IsError {
		t.Fatalf("Bash tool failed: %s", res.Output)
	}
	if !strings.Contains(res.Output, "v") {
		t.Errorf("unexpected node output: %q", res.Output)
	}
}
