package memory

import (
	"os"
	"path/filepath"
	"strings"
)

// AutoMemEntrypointName is the filename of the per-project memory index.
const AutoMemEntrypointName = "MEMORY.md"

// memoryDirOverride returns the remote-memory override. The current name is
// MYGOCODE_REMOTE_MEMORY_DIR; the legacy MEWCODE_REMOTE_MEMORY_DIR (from
// before the project rename) is still honoured so existing setups keep
// working. New name wins when both are set.
func memoryDirOverride() string {
	if v := os.Getenv("MYGOCODE_REMOTE_MEMORY_DIR"); v != "" {
		return v
	}
	return os.Getenv("MEWCODE_REMOTE_MEMORY_DIR")
}

// GetAutoMemPath returns the auto-memory directory path for the given
// project root. Shape: <projectRoot>/.mygocode/memory/
//
// The trailing separator is preserved so prefix-based path matching (e.g.,
// sandbox `HasPrefix` checks) work correctly without falsely matching
// `…/memoryxyz`.
//
// MygoCode colocates memory with other project-local state under .mygocode/
// so records show up in the IDE and editors can open them directly.
//
// Resolution order:
//  1. MYGOCODE_REMOTE_MEMORY_DIR env var (legacy MEWCODE_REMOTE_MEMORY_DIR
//     still accepted) — escape hatch for CI/container scenarios where memory
//     should live elsewhere; the value is cleaned to the platform's native
//     path form.
//  2. <projectRoot>/.mygocode/memory
func GetAutoMemPath(projectRoot string) string {
	if override := memoryDirOverride(); override != "" {
		return filepath.Clean(override) + string(filepath.Separator)
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		abs = projectRoot
	}
	return filepath.Join(abs, ".mygocode", "memory") + string(filepath.Separator)
}

// GetAutoMemEntrypoint returns the path to the MEMORY.md inside the
// auto-memory directory.
func GetAutoMemEntrypoint(projectRoot string) string {
	dir := GetAutoMemPath(projectRoot)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, AutoMemEntrypointName)
}

// IsAutoMemPath checks if an absolute path is within EITHER the project-level
// or user-level auto-memory directory. Used by the path sandbox to allow
// Writes into either memory dir.
func IsAutoMemPath(absolutePath, projectRoot string) bool {
	abs := filepath.Clean(absolutePath)
	if dir := GetAutoMemPath(projectRoot); dir != "" {
		if strings.HasPrefix(abs+string(filepath.Separator), dir) {
			return true
		}
	}
	if dir := GetUserAutoMemPath(); dir != "" {
		if strings.HasPrefix(abs+string(filepath.Separator), dir) {
			return true
		}
	}
	return false
}

// GetUserAutoMemPath returns the user-level auto-memory directory:
// ~/.mygocode/memory/. Used for type=user / type=feedback memories that
// follow the human across projects (e.g. coding preferences). Returns ""
// if the home directory cannot be resolved.
//
// Trailing separator is preserved for prefix-based path matching.
func GetUserAutoMemPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".mygocode", "memory") + string(filepath.Separator)
}

// GetUserAutoMemEntrypoint returns the path to ~/.mygocode/memory/MEMORY.md.
func GetUserAutoMemEntrypoint() string {
	dir := GetUserAutoMemPath()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, AutoMemEntrypointName)
}

// IsUserAutoMemPath checks if an absolute path is within the user-level
// memory dir. Used in places where we need to distinguish user-scope from
// project-scope (sandbox already accepts both; this is for routing).
func IsUserAutoMemPath(absolutePath string) bool {
	dir := GetUserAutoMemPath()
	if dir == "" {
		return false
	}
	abs := filepath.Clean(absolutePath)
	return strings.HasPrefix(abs+string(filepath.Separator), dir)
}
