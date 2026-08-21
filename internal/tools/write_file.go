package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"mygocode/internal/filehistory"
)

type WriteFileTool struct {
	FileHistory    *filehistory.History
	FileStateCache *FileStateCache
	// WorkDir anchors relative file_path values. Empty means the process
	// cwd (legacy behaviour); hosts that run the agent outside the project
	// root (remote mode, eval harness) must set it.
	WorkDir string
}

func (t *WriteFileTool) Name() string { return "WriteFile" }

func (t *WriteFileTool) Description() string { return WriteFileDescription }

func (t *WriteFileTool) Category() ToolCategory { return CategoryWrite }

func (t *WriteFileTool) Schema() map[string]any {
	return map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string", "description": "Path to the file to write"},
				"content":   map[string]any{"type": "string", "description": "Content to write to the file"},
			},
			"required": []string{"file_path", "content"},
		},
	}
}

func (t *WriteFileTool) Execute(_ context.Context, args map[string]any) ToolResult {
	filePath := resolveToolPath(t.WorkDir, argStr(args, "file_path"))
	content, _ := args["content"].(string)
	if filePath == "" {
		return ToolResult{Output: "Error: file_path is required", IsError: true}
	}

	// Read-before-edit gate (skipped for new files) + concurrent-edit claim.
	// BeginEdit rejects parallel writes to the same file; EndEdit releases
	// the claim after the write (or on failure).
	if t.FileStateCache != nil {
		if ok, errMsg := t.FileStateCache.BeginEdit(filePath); !ok {
			return ToolResult{Output: errMsg, IsError: true}
		}
		defer t.FileStateCache.EndEdit(filePath)
	}

	if t.FileHistory != nil {
		t.FileHistory.TrackEdit(filePath)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error creating directories: %s", err), IsError: true}
	}

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %s", err), IsError: true}
	}

	// Update cache after successful write
	if t.FileStateCache != nil {
		t.FileStateCache.Update(filePath, content)
	}

	return ToolResult{Output: fmt.Sprintf("Successfully wrote to %s", filePath)}
}
