package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type GlobTool struct {
	// WorkDir anchors relative path values. Empty means the process cwd
	// (legacy behaviour); hosts that run the agent outside the project root
	// (remote mode, eval harness) must set it.
	WorkDir string
}

func (t *GlobTool) Name() string { return "Glob" }

func (t *GlobTool) Description() string { return GlobDescription }

func (t *GlobTool) Category() ToolCategory { return CategoryRead }

func (t *GlobTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Glob pattern to match (e.g. '**/*.py')"},
				"path":    map[string]any{"type": "string", "description": "Base directory to search from", "default": "."},
			},
			"required": []string{"pattern"},
		},
	}
}

func (t *GlobTool) Execute(_ context.Context, args map[string]any) ToolResult {
	pattern, _ := args["pattern"].(string)
	basePath := resolveToolPath(t.WorkDir, argStr(args, "path"))
	if basePath == "" {
		basePath = "."
	}
	if pattern == "" {
		return ToolResult{Output: "Error: pattern is required", IsError: true}
	}

	info, err := os.Stat(basePath)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: path not found: %s", basePath), IsError: true}
	}
	if err != nil || !info.IsDir() {
		return ToolResult{Output: fmt.Sprintf("Error: path not found: %s", basePath), IsError: true}
	}

	// Recognize doublestar `**/` prefix and treat it as "match basePattern at
	// any depth". Go's filepath.Match doesn't understand `**`; without this,
	// the most common LLM-issued patterns like `**/*.go` silently match nothing.
	recursive := false
	basePattern := pattern
	for strings.HasPrefix(basePattern, "**/") {
		basePattern = basePattern[3:]
		recursive = true
	}

	var matches []string
	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if SkipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(basePath, path)
		matched := false
		if recursive {
			// `**/<basePattern>` — match basePattern against base name at any depth.
			matched, _ = filepath.Match(basePattern, filepath.Base(path))
		} else {
			matched, _ = filepath.Match(pattern, filepath.Base(path))
			if !matched {
				matched, _ = filepath.Match(pattern, rel)
			}
		}
		if matched {
			// 统一输出为 Unix 风格路径（/）：LLM 训练语料以 / 为主，
			// 跨平台输出一致；后续工具（ReadFile/EditFile 等）对两种
			// 分隔符都能解析，不受影响。
			matches = append(matches, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %s", err), IsError: true}
	}

	// 按修改时间倒序，最近修改的排前面
	sort.Slice(matches, func(i, j int) bool {
		fi, ei := os.Stat(filepath.Join(basePath, matches[i]))
		fj, ej := os.Stat(filepath.Join(basePath, matches[j]))
		if ei != nil || ej != nil {
			return matches[i] < matches[j]
		}
		return fi.ModTime().After(fj.ModTime())
	})
	if len(matches) == 0 {
		return ToolResult{Output: "No files matched the pattern."}
	}
	return ToolResult{Output: strings.Join(matches, "\n")}
}
