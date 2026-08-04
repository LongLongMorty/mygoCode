package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateUserHome redirects both the Unix and Windows home-dir lookups to a
// fresh temp dir. On Windows os.UserHomeDir reads USERPROFILE, not HOME, so
// setting only HOME leaves tests reading the real user memory directory —
// which turns environment-dependent (leftover files → flaky failures).
func isolateUserHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
}

func TestGetAutoMemPath(t *testing.T) {
	t.Setenv("MYGOCODE_REMOTE_MEMORY_DIR", "")
	root := filepath.Join(t.TempDir(), "project")
	want := filepath.Join(root, ".mygocode", "memory") + string(filepath.Separator)
	path := GetAutoMemPath(root)
	if path != want {
		t.Errorf("GetAutoMemPath(%q) = %q, want %q", root, path, want)
	}
	if !strings.HasPrefix(path, root) {
		t.Errorf("expected path under project root %s, got: %s", root, path)
	}
}

func TestGetAutoMemPathRespectsOverride(t *testing.T) {
	t.Setenv("MYGOCODE_REMOTE_MEMORY_DIR", "/custom/memdir")
	override := "/custom/memdir"
	want := filepath.Clean(override) + string(filepath.Separator)
	path := GetAutoMemPath("/tmp/anything")
	if path != want {
		t.Errorf("override not honored: got %q, want %q", path, want)
	}
}

// TestGetAutoMemPathLegacyEnvVar verifies the pre-rename env var name still
// works as a fallback, and the new name wins when both are set.
func TestGetAutoMemPathLegacyEnvVar(t *testing.T) {
	t.Setenv("MYGOCODE_REMOTE_MEMORY_DIR", "")
	t.Setenv("MEWCODE_REMOTE_MEMORY_DIR", "/legacy/memdir")
	want := filepath.Clean("/legacy/memdir") + string(filepath.Separator)
	if got := GetAutoMemPath("/tmp/x"); got != want {
		t.Errorf("legacy env var not honoured: got %q, want %q", got, want)
	}

	t.Setenv("MYGOCODE_REMOTE_MEMORY_DIR", "/new/memdir")
	want = filepath.Clean("/new/memdir") + string(filepath.Separator)
	if got := GetAutoMemPath("/tmp/x"); got != want {
		t.Errorf("new env var should win: got %q, want %q", got, want)
	}
}

func TestIsAutoMemPath(t *testing.T) {
	t.Setenv("MYGOCODE_REMOTE_MEMORY_DIR", "")
	isolateUserHome(t)
	root := filepath.Join(t.TempDir(), "p")
	dir := GetAutoMemPath(root)
	sep := string(filepath.Separator)
	cases := map[string]bool{
		dir + sep + "MEMORY.md":                  true,
		dir + sep + "foo.md":                     true,
		filepath.Join(dir, "sub", "foo.md"):      true,
		filepath.Join(root, ".mygocode", "memoryx"): false,
		filepath.Join(t.TempDir(), "other", "foo.md"): false,
	}
	for path, want := range cases {
		if got := IsAutoMemPath(path, root); got != want {
			t.Errorf("IsAutoMemPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestParseMemoryType(t *testing.T) {
	cases := map[string]MemoryType{
		"user":      TypeUser,
		"feedback":  TypeFeedback,
		"project":   TypeProject,
		"reference": TypeReference,
	}
	for in, want := range cases {
		got, ok := ParseMemoryType(in)
		if !ok || got != want {
			t.Errorf("ParseMemoryType(%q) = (%q, %v); want (%q, true)", in, got, ok, want)
		}
	}
	if _, ok := ParseMemoryType("unknown"); ok {
		t.Errorf("ParseMemoryType(unknown) should return false")
	}
	if _, ok := ParseMemoryType(""); ok {
		t.Errorf("ParseMemoryType empty should return false")
	}
}

func TestTruncateEntrypointContent(t *testing.T) {
	t.Run("under limits", func(t *testing.T) {
		raw := "- one\n- two\n- three"
		got := TruncateEntrypointContent(raw)
		if got.WasLineTruncated || got.WasByteTruncated {
			t.Errorf("should not truncate small content; got %+v", got)
		}
		if got.Content != raw {
			t.Errorf("content modified: %q", got.Content)
		}
	})

	t.Run("line cap", func(t *testing.T) {
		var lines []string
		for i := 0; i < MaxEntrypointLines+10; i++ {
			lines = append(lines, "x")
		}
		raw := strings.Join(lines, "\n")
		got := TruncateEntrypointContent(raw)
		if !got.WasLineTruncated {
			t.Errorf("expected line truncation")
		}
		if !strings.Contains(got.Content, "WARNING") {
			t.Errorf("truncation warning missing")
		}
	})

	t.Run("byte cap", func(t *testing.T) {
		raw := strings.Repeat("xxxxxxxxxx", MaxEntrypointBytes/5) + "\nextra"
		got := TruncateEntrypointContent(raw)
		if !got.WasByteTruncated {
			t.Errorf("expected byte truncation")
		}
	})
}

func TestLoadAutoMemoryPrompt(t *testing.T) {
	t.Setenv("MYGOCODE_REMOTE_MEMORY_DIR", "")
	isolateUserHome(t)
	root := t.TempDir()
	prompt := LoadAutoMemoryPrompt(root)
	for _, want := range []string{
		"# auto memory",
		"## Types of memory",
		"## What NOT to save in memory",
		"## How to save memories",
		"## When to access memories",
		"## Before recommending from memory",
		"User-level " + AutoMemEntrypointName,
		"Project-level " + AutoMemEntrypointName,
		"is currently empty",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q in auto memory prompt", want)
		}
	}
}

func TestManagerLoadAll(t *testing.T) {
	t.Setenv("MYGOCODE_REMOTE_MEMORY_DIR", "")
	isolateUserHome(t)
	root := t.TempDir()
	mgr := NewManager(root)
	dir := mgr.Dir()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, "user_role.md"), `---
name: user-role
description: user is a Go engineer
type: user
---

Body content.
`)
	mustWriteFile(t, filepath.Join(dir, "MEMORY.md"), "- [user-role](user_role.md) — user is a Go engineer\n")
	mustWriteFile(t, filepath.Join(dir, "skip.txt"), "not a memory")

	files := mgr.LoadAll()
	if len(files) != 1 {
		t.Fatalf("expected 1 memory file (MEMORY.md and skip.txt excluded), got %d", len(files))
	}
	f := files[0]
	if f.Name != "user-role" || f.Type != TypeUser {
		t.Errorf("frontmatter parsed wrong: %+v", f)
	}

	got := mgr.GetMemories()
	if len(got) != 1 || !strings.Contains(got[0], "[user]") {
		t.Errorf("GetMemories returned %v", got)
	}
}

func TestManagerClear(t *testing.T) {
	t.Setenv("MYGOCODE_REMOTE_MEMORY_DIR", "")
	isolateUserHome(t)
	root := t.TempDir()
	mgr := NewManager(root)
	dir := mgr.Dir()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, "a.md"), "---\nname: a\ntype: user\n---\n")
	mustWriteFile(t, filepath.Join(dir, "MEMORY.md"), "- [a](a.md)\n")

	mgr.Clear()

	if files := mgr.LoadAll(); len(files) != 0 {
		t.Errorf("expected 0 files after Clear, got %d", len(files))
	}
	if _, err := os.Stat(mgr.EntrypointPath()); !os.IsNotExist(err) {
		t.Errorf("MEMORY.md should be removed; stat err = %v", err)
	}
}

func TestBuildSystemReminderIncludesExistingIndex(t *testing.T) {
	t.Setenv("MYGOCODE_REMOTE_MEMORY_DIR", "")
	isolateUserHome(t)
	root := t.TempDir()
	mgr := NewManager(root)

	if err := os.MkdirAll(mgr.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	indexLine := "- [previous-memory](prev.md) — saved earlier"
	mustWriteFile(t, mgr.EntrypointPath(), indexLine+"\n")

	prompt := mgr.BuildSystemReminder()
	if !strings.Contains(prompt, indexLine) {
		t.Errorf("system reminder did not include MEMORY.md content:\n%s", prompt)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
