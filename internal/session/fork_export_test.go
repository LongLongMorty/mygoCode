package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForkSessionPreservesSourceAndMetadata(t *testing.T) {
	dir := t.TempDir()
	SaveMessage(dir, "parent", Message{Role: "user", Content: "hello", Ts: 1})
	SaveCompactBoundary(dir, "parent", "summary", []KeepMessage{{Role: "assistant", Content: "tail"}})
	source, err := os.ReadFile(SessionFilePath(dir, "parent"))
	if err != nil {
		t.Fatal(err)
	}

	child, err := ForkSession(dir, "parent")
	if err != nil {
		t.Fatal(err)
	}
	if child == "parent" {
		t.Fatal("fork reused parent ID")
	}
	metadata, ok, err := GetSessionMetadata(dir, child)
	if err != nil || !ok || metadata.ParentSessionID != "parent" || metadata.Version != 1 {
		t.Fatalf("metadata = %#v, %t, %v", metadata, ok, err)
	}
	childMessages := LoadSession(dir, child)
	if len(childMessages) != 2 || childMessages[0].Content != "hello" || childMessages[1].Type != TypeCompactBoundary {
		t.Fatalf("forked messages = %#v", childMessages)
	}
	SaveMessage(dir, child, Message{Role: "assistant", Content: "child-only", Ts: 2})
	parentNow, err := os.ReadFile(SessionFilePath(dir, "parent"))
	if err != nil || string(parentNow) != string(source) {
		t.Fatalf("parent changed: %v", err)
	}
	infos := ListSessions(dir)
	for _, info := range infos {
		if info.ID == child && info.ParentSessionID != "parent" {
			t.Fatalf("parent label missing: %#v", info)
		}
	}
}

func TestExportHTMLEscapesRedactsAndDoesNotChangeSession(t *testing.T) {
	dir := t.TempDir()
	sid := "export"
	SaveMessage(dir, sid, Message{Role: "user", Content: "<script>alert(1)</script> & api_key=super-secret", Ts: 1})
	SaveMessage(dir, sid, Message{Role: "tool", Content: strings.Repeat("x", maxExportToolContent+10), Ts: 2})
	before, err := os.ReadFile(SessionFilePath(dir, sid))
	if err != nil {
		t.Fatal(err)
	}
	path, err := ExportHTML(dir, sid, "")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, ".mygocode", "exports", "session-export.html") {
		t.Fatalf("path = %s", path)
	}
	html, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(html)
	if strings.Contains(text, "<script>alert") || !strings.Contains(text, "&lt;script&gt;") || strings.Contains(text, "super-secret") || !strings.Contains(text, "[REDACTED]") || !strings.Contains(text, "[truncated]") {
		t.Fatalf("unsafe export: %s", text[:min(len(text), 400)])
	}
	after, err := os.ReadFile(SessionFilePath(dir, sid))
	if err != nil || string(before) != string(after) {
		t.Fatalf("source changed: %v", err)
	}
}

func TestExportHTMLRejectsTraversalAndEmptySession(t *testing.T) {
	dir := t.TempDir()
	SaveMessage(dir, "s", Message{Role: "user", Content: "hello", Ts: 1})
	if _, err := ExportHTML(dir, "s", "../outside.html"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if _, err := ExportHTML(dir, "missing", ""); err == nil {
		t.Fatal("expected missing session error")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
