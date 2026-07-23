package agent

import (
	"mygocode/internal/permissions"
	"testing"
)

func TestProfilesHaveSafeMappings(t *testing.T) {
	if DefaultProfile().Name != "build" {
		t.Fatal("build must be default")
	}
	for _, name := range []string{"plan", "review"} {
		p, err := LookupProfile(name)
		if err != nil || !p.ReadOnly || p.AllowsTool("WriteFile") || p.AllowsTool("EditFile") || p.AllowsTool("Bash") {
			t.Fatalf("unsafe %s profile: %#v, %v", name, p, err)
		}
	}
	p, _ := LookupProfile("plan")
	if p.PermissionMode != permissions.ModePlan {
		t.Fatalf("plan mode = %s", p.PermissionMode)
	}
	if _, err := LookupProfile("unknown"); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
