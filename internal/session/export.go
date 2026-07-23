package session

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxExportToolContent = 16 * 1024

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key\s*[:=]\s*["']?)[^\s"']+`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)(password\s*[:=]\s*["']?)[^\s"']+`),
	regexp.MustCompile(`(?i)(secret\s*[:=]\s*["']?)[^\s"']+`),
}

func redact(text string) string {
	for _, pattern := range sensitivePatterns {
		text = pattern.ReplaceAllString(text, "${1}[REDACTED]")
	}
	return text
}

func exportPath(workDir, sessionID, requested string) (string, error) {
	if requested == "" {
		return filepath.Join(workDir, ".mygocode", "exports", "session-"+sessionID+".html"), nil
	}
	if filepath.IsAbs(requested) || strings.Contains(filepath.Clean(requested), "..") {
		return "", fmt.Errorf("export path must be a relative path inside the project")
	}
	path := filepath.Join(workDir, requested)
	rel, err := filepath.Rel(workDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("export path must be inside the project")
	}
	return path, nil
}

// ExportHTML writes a self-contained, escaped and redacted transcript without
// changing the source JSONL. requested may be empty for the default path.
func ExportHTML(workDir, sessionID, requested string) (string, error) {
	msgs := LoadSession(workDir, sessionID)
	if len(msgs) == 0 {
		return "", fmt.Errorf("session %q not found or empty", sessionID)
	}
	path, err := exportPath(workDir, sessionID, requested)
	if err != nil {
		return "", err
	}
	var body strings.Builder
	for _, msg := range msgs {
		content := msg.Content
		if msg.Role == "tool" || msg.Role == "tool_result" {
			if len(content) > maxExportToolContent {
				content = content[:maxExportToolContent] + "\n[truncated]"
			}
		}
		timestamp := ""
		if msg.Ts > 0 {
			timestamp = time.Unix(msg.Ts, 0).Format(time.RFC3339)
		}
		fmt.Fprintf(&body, "<article><header>%s <time>%s</time></header><pre>%s</pre></article>\n", html.EscapeString(msg.Role), html.EscapeString(timestamp), html.EscapeString(redact(content)))
	}
	doc := "<!doctype html><html><head><meta charset=\"utf-8\"><title>Session " + html.EscapeString(sessionID) + "</title><style>body{font:14px system-ui;margin:2rem;max-width:960px}article{border-bottom:1px solid #ddd;padding:1rem 0}header{font-weight:700}time{font-weight:400;color:#666;margin-left:1rem}pre{white-space:pre-wrap;word-break:break-word}</style></head><body><h1>Session " + html.EscapeString(sessionID) + "</h1>" + body.String() + "</body></html>"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
