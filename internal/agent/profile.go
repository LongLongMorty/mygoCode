package agent

import (
	"fmt"
	"strings"

	"mygocode/internal/permissions"
)

// Profile describes the three built-in main-agent operating modes. Profiles
// are intentionally mappings over the existing Agent and Checker rather than
// separate agent implementations.
type Profile struct {
	Name           string
	Description    string
	PermissionMode permissions.PermissionMode
	PromptSuffix   string
	ReadOnly       bool
}

var profiles = map[string]Profile{
	"build": {Name: "build", Description: "Default development profile", PermissionMode: permissions.ModeDefault},
	"plan": {Name: "plan", Description: "Read-only planning profile", PermissionMode: permissions.ModePlan, ReadOnly: true,
		PromptSuffix: "Work in planning mode. Inspect and explain changes; do not modify project files."},
	"review": {Name: "review", Description: "Read-only code review profile", PermissionMode: permissions.ModeDefault, ReadOnly: true,
		PromptSuffix: "Review the current code carefully. Report correctness, security, and maintainability findings without changing files."},
}

func DefaultProfile() Profile { return profiles["build"] }

func LookupProfile(name string) (Profile, error) {
	p, ok := profiles[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Profile{}, fmt.Errorf("unknown agent profile %q; available: build, plan, review", name)
	}
	return p, nil
}

func Profiles() []Profile { return []Profile{profiles["build"], profiles["plan"], profiles["review"]} }

// AllowsTool enforces the model-visible read-only boundary. The existing
// Checker still applies to every actual tool invocation.
func (p Profile) AllowsTool(name string) bool {
	if !p.ReadOnly {
		return true
	}
	switch name {
	case "WriteFile", "EditFile", "Agent", "EnterWorktree", "ExitWorktree":
		return false
	case "Bash":
		return false
	default:
		return true
	}
}

func (p Profile) Instructions(base string) string {
	if p.PromptSuffix == "" {
		return base
	}
	if base == "" {
		return p.PromptSuffix
	}
	return base + "\n\n" + p.PromptSuffix
}
