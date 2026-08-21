package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBeginEditConcurrentSameFile verifies the core of the concurrent-edit
// guard: of N parallel BeginEdit calls on the same file, exactly one wins.
func TestBeginEditConcurrentSameFile(t *testing.T) {
	cache := NewFileStateCache()
	f := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(f, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Must read it first for read-before-edit to pass.
	info, _ := os.Stat(f)
	cache.Record(f, "v1", info.ModTime().UnixMilli())

	const n = 20
	var wg sync.WaitGroup
	okCount := 0
	var mu sync.Mutex
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _ := cache.BeginEdit(f)
			if ok {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if okCount != 1 {
		t.Fatalf("BeginEdit winners = %d, want exactly 1 (lost-update window must be closed)", okCount)
	}
}

// TestBeginEditLifecycle covers claim → reject → release → reclaim.
func TestBeginEditLifecycle(t *testing.T) {
	cache := NewFileStateCache()
	f := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(f, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(f)
	cache.Record(f, "v1", info.ModTime().UnixMilli())

	if ok, msg := cache.BeginEdit(f); !ok {
		t.Fatalf("first BeginEdit failed: %s", msg)
	}
	if ok, msg := cache.BeginEdit(f); ok {
		t.Fatal("second BeginEdit should be rejected while first is in flight")
	} else if !strings.Contains(msg, "already being edited") {
		t.Errorf("rejection message = %q, want 'already being edited'", msg)
	}
	cache.EndEdit(f)
	if ok, msg := cache.BeginEdit(f); !ok {
		t.Fatalf("BeginEdit after EndEdit failed: %s", msg)
	}
	cache.EndEdit(f)
}

// TestBeginEditNewFile skips read-before-edit for new files but still claims.
func TestBeginEditNewFile(t *testing.T) {
	cache := NewFileStateCache()
	f := filepath.Join(t.TempDir(), "new.txt")

	if ok, msg := cache.BeginEdit(f); !ok {
		t.Fatalf("BeginEdit on new file should succeed without a prior read: %s", msg)
	}
	if ok, _ := cache.BeginEdit(f); ok {
		t.Fatal("second BeginEdit on the same new file should be rejected")
	}
	cache.EndEdit(f)
	if ok, _ := cache.BeginEdit(f); !ok {
		t.Fatal("BeginEdit after EndEdit on new file should succeed")
	}
	cache.EndEdit(f)
}

// TestBeginEditRequiresRead verifies read-before-edit still applies to
// existing files.
func TestBeginEditRequiresRead(t *testing.T) {
	cache := NewFileStateCache()
	f := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, msg := cache.BeginEdit(f); ok {
		t.Fatal("BeginEdit on unread existing file should be rejected")
	} else if !strings.Contains(msg, "not been read yet") {
		t.Errorf("message = %q, want 'not been read yet'", msg)
	}
}

// TestBeginEditDetectsExternalModification verifies the mtime check still
// fires after the file changed since the recorded read.
func TestBeginEditDetectsExternalModification(t *testing.T) {
	cache := NewFileStateCache()
	f := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(f, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(f)
	cache.Record(f, "v1", info.ModTime().UnixMilli())

	if err := os.WriteFile(f, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Push the mtime explicitly into the future: two back-to-back writes can
	// land on the same mtime tick (NTFS resolution), which would make the
	// strict `>` comparison fail and turn this into a flaky test.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(f, future, future); err != nil {
		t.Fatal(err)
	}
	if ok, msg := cache.BeginEdit(f); ok {
		t.Fatal("BeginEdit should reject a file modified since the recorded read")
	} else if !strings.Contains(msg, "modified since last read") {
		t.Errorf("message = %q, want 'modified since last read'", msg)
	}
}

// TestConcurrentEditToolNoLostUpdate is the end-to-end version: two parallel
// EditFileTool calls on the same file — exactly one wins and the surviving
// content is one of the two intended edits, never a torn mix.
func TestConcurrentEditToolNoLostUpdate(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsc := NewFileStateCache()
	data, _ := os.ReadFile(f)
	info, _ := os.Stat(f)
	fsc.Record(f, string(data), info.ModTime().UnixMilli())

	tool := &EditFileTool{FileStateCache: fsc}
	edit := func(old, new string) bool {
		res := tool.Execute(context.Background(), map[string]any{
			"file_path":  f,
			"old_string": old,
			"new_string": new,
		})
		return !res.IsError
	}

	var wg sync.WaitGroup
	var winsMu sync.Mutex
	wins := 0
	wg.Add(2)
	go func() {
		defer wg.Done()
		if edit("line1", "line1-A") {
			winsMu.Lock()
			wins++
			winsMu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		if edit("line2", "line2-B") {
			winsMu.Lock()
			wins++
			winsMu.Unlock()
		}
	}()
	wg.Wait()

	final, _ := os.ReadFile(f)
	content := string(final)
	if wins != 1 {
		t.Errorf("successful edits = %d, want exactly 1", wins)
	}
	// The losing edit must not have partially clobbered the winner: the file
	// holds exactly one intended result, never a torn mix.
	hasA := strings.Contains(content, "line1-A")
	hasB := strings.Contains(content, "line2-B")
	if hasA == hasB {
		t.Errorf("final content should contain exactly one edit's result, got %q", content)
	}
}
