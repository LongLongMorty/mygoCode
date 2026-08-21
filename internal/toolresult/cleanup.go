package toolresult

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// Spill retention and cleanup constants. The 30-day window matches the
// session retention period (internal/session maxSessionAgeDays), so spilled
// results never outlive the sessions that reference them.
const (
	// SpillMaxAge is how long a spilled tool result stays on disk before the
	// cleanup loop removes it.
	SpillMaxAge = 30 * 24 * time.Hour
	// SpillCleanupInterval is how often the background cleanup loop runs.
	SpillCleanupInterval = 1 * time.Hour
)

// CleanupSpillDir removes spilled tool-result files under
// <workDir>/.mygocode/tool_results/ whose mtime is older than maxAge.
// Returns the number of files removed. A missing directory is a no-op.
func CleanupSpillDir(workDir string, maxAge time.Duration) int {
	dir := filepath.Join(workDir, SpillSubdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0 // missing dir (or unreadable) — nothing to clean
	}

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue // never recurse into unexpected subdirs
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed
}

// StartCleanupLoop runs periodic spill cleanup in a background goroutine.
// Returns immediately; the goroutine exits when ctx is cancelled. Mirrors the
// worktree stale-cleanup loop pattern.
func StartCleanupLoop(ctx context.Context, workDir string, interval, maxAge time.Duration) {
	if interval <= 0 || maxAge <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				CleanupSpillDir(workDir, maxAge)
			}
		}
	}()
}
