package agents

import (
	"strings"
	"testing"
	"time"
)

func TestTaskSnapshotsAndSummaryAreReadOnly(t *testing.T) {
	mgr := NewTaskManager()
	id := mgr.CreateTask(strings.Repeat("long-name-", 10))
	mgr.SetRunning(id, func() {})
	snapshots := mgr.Snapshots()
	if len(snapshots) != 1 || snapshots[0].Status != TaskRunning {
		t.Fatalf("snapshots: %#v", snapshots)
	}
	summary := FormatTaskSummary(snapshots, time.Now())
	if !strings.Contains(summary, "sub-agent") || strings.Contains(summary, "long-name-long-name-long-name-long-name-long-name-long-name") {
		t.Fatalf("summary: %q", summary)
	}
	if mgr.GetTask(id).Status != TaskRunning {
		t.Fatal("summary mutated task")
	}
}
