package toolresult

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSpillFile writes a spill file and returns its path, advancing mtime
// via Chtimes so age can be controlled deterministically.
func writeSpillFile(t *testing.T, dir, name string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-age)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCleanupSpillDirRemovesOnlyOld verifies the age-based filter: old files
// are removed, recent files survive, subdirectories are never touched.
func TestCleanupSpillDirRemovesOnlyOld(t *testing.T) {
	workDir := t.TempDir()
	dir := filepath.Join(workDir, SpillSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	old := writeSpillFile(t, dir, "old1", 40*24*time.Hour)
	old2 := writeSpillFile(t, dir, "old2", 31*24*time.Hour)
	fresh := writeSpillFile(t, dir, "fresh", 24*time.Hour)

	// A nested directory must be left alone entirely.
	nested := filepath.Join(dir, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedOld := writeSpillFile(t, nested, "inner-old", 40*24*time.Hour)

	removed := CleanupSpillDir(workDir, SpillMaxAge)

	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	for _, p := range []string{old, old2} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("old spill %s should have been removed", p)
		}
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh spill should survive: %v", err)
	}
	if _, err := os.Stat(nestedOld); err != nil {
		t.Errorf("nested file must not be touched: %v", err)
	}
}

// TestCleanupSpillDirMissingDirIsNoop verifies a missing spill directory
// doesn't error and removes nothing.
func TestCleanupSpillDirMissingDirIsNoop(t *testing.T) {
	workDir := t.TempDir()
	if removed := CleanupSpillDir(workDir, SpillMaxAge); removed != 0 {
		t.Errorf("missing dir removed = %d, want 0", removed)
	}
}

// TestCleanupSpillDirBoundary verifies files just inside the retention
// window survive: mtime is compared strictly before the cutoff, so a file
// aged maxAge minus a margin (still "under 30 days") is kept. Exact-equality
// tests are meaningless here — wall-clock drift between Chtimes and the
// cleanup call always pushes "exactly 30 days" past the cutoff.
func TestCleanupSpillDirBoundary(t *testing.T) {
	workDir := t.TempDir()
	dir := filepath.Join(workDir, SpillSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	recent := writeSpillFile(t, dir, "recent", SpillMaxAge-1*time.Hour)
	justOld := writeSpillFile(t, dir, "just-old", SpillMaxAge+1*time.Hour)

	removed := CleanupSpillDir(workDir, SpillMaxAge)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1 (only the >30d file)", removed)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("file aged 30d-1h should survive: %v", err)
	}
	if _, err := os.Stat(justOld); !os.IsNotExist(err) {
		t.Errorf("file aged 30d+1h should be removed")
	}
}
