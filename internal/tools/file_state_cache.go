package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileStateCache tracks which files have been read and their modification
// times, enforcing a "read-before-edit" discipline identical to Claude Code.
// It also serialises concurrent edits to the same file: BeginEdit claims the
// file for the duration of the write, so a second parallel tool call editing
// the same file is rejected instead of silently lost-updating the first.
type FileStateCache struct {
	mu      sync.Mutex
	entries map[string]*fileStateEntry
	editing map[string]bool // files with an in-flight edit
}

type fileStateEntry struct {
	Content string
	Mtime   int64 // UnixMilli
}

func NewFileStateCache() *FileStateCache {
	return &FileStateCache{
		entries: make(map[string]*fileStateEntry),
		editing: make(map[string]bool),
	}
}

// Record stores the file content and mtime after a successful read.
func (c *FileStateCache) Record(filePath string, content string, mtime int64) {
	abs := normalizePath(filePath)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[abs] = &fileStateEntry{Content: content, Mtime: mtime}
}

// Check verifies that a file has been read and hasn't been modified since.
// Returns (true, "") if OK, or (false, errorMessage) if the edit should be
// blocked.
func (c *FileStateCache) Check(filePath string) (bool, string) {
	abs := normalizePath(filePath)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.checkLocked(abs)
}

// BeginEdit claims the file for an in-flight edit and verifies the
// read-before-edit + mtime discipline. Concurrent edits to the same file are
// rejected with a clear message. Callers must call EndEdit when the write
// finishes (success or failure) to release the claim.
func (c *FileStateCache) BeginEdit(filePath string) (bool, string) {
	abs := normalizePath(filePath)
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.editing[abs] {
		return false, fmt.Sprintf("Error: file is already being edited by a concurrent tool call. Wait for it to finish, then re-read the file before editing.")
	}

	// New files skip the read-before-edit requirement but still get the
	// edit claim, so two parallel writes to the same new file can't clobber
	// each other.
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		c.editing[abs] = true
		return true, ""
	}

	if ok, msg := c.checkLocked(abs); !ok {
		return false, msg
	}
	c.editing[abs] = true
	return true, ""
}

// EndEdit releases the edit claim taken by BeginEdit. Safe to call even when
// no claim exists.
func (c *FileStateCache) EndEdit(filePath string) {
	abs := normalizePath(filePath)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.editing, abs)
}

// checkLocked runs the read-before-edit + mtime checks. Caller must hold mu.
func (c *FileStateCache) checkLocked(abs string) (bool, string) {
	entry, exists := c.entries[abs]
	if !exists {
		return false, fmt.Sprintf("Error: file has not been read yet. Read it first before editing.")
	}

	info, err := os.Stat(abs)
	if err != nil {
		// File might have been deleted — let the caller handle that.
		return true, ""
	}
	currentMtime := info.ModTime().UnixMilli()
	if currentMtime > entry.Mtime {
		return false, fmt.Sprintf("Error: file has been modified since last read. Read it again before editing.")
	}

	return true, ""
}

// Update refreshes the cache entry after a successful edit or write.
func (c *FileStateCache) Update(filePath string, newContent string) {
	abs := normalizePath(filePath)
	info, err := os.Stat(abs)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[abs] = &fileStateEntry{
		Content: newContent,
		Mtime:   info.ModTime().UnixMilli(),
	}
}

func normalizePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}
