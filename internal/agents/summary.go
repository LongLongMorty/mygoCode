package agents

import (
	"fmt"
	"strings"
	"time"
)

const summaryTextLimit = 48

func truncateSummary(text string) string {
	text = strings.TrimSpace(text)
	if len([]rune(text)) <= summaryTextLimit {
		return text
	}
	return string([]rune(text)[:summaryTextLimit]) + "…"
}

// FormatTaskSummary is read-only: it renders only identity and lifecycle
// fields, never task output, errors, or tool arguments.
func FormatTaskSummary(tasks []TaskSnapshot, now time.Time) string {
	if len(tasks) == 0 {
		return "No active or completed sub-agent tasks."
	}
	var lines []string
	for _, task := range tasks {
		age := "-"
		if !task.CreatedAt.IsZero() {
			age = now.Sub(task.CreatedAt).Round(time.Second).String()
		}
		lines = append(lines, fmt.Sprintf("%s | sub-agent | %s | %s | tools: - | %s", task.ID, task.Status, truncateSummary(task.Name), age))
	}
	return strings.Join(lines, "\n")
}
