package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectTrustStateLifecycle(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	state, err := ProjectTrustState(home, project)
	if err != nil || state != TrustUndecided {
		t.Fatalf("initial state = %q, %v", state, err)
	}
	if err := SetProjectTrust(home, project, TrustTrusted); err != nil {
		t.Fatal(err)
	}
	state, err = ProjectTrustState(home, project)
	if err != nil || state != TrustTrusted {
		t.Fatalf("trusted state = %q, %v", state, err)
	}
	if err := RevokeProjectTrust(home, project); err != nil {
		t.Fatal(err)
	}
	state, err = ProjectTrustState(home, project)
	if err != nil || state != TrustUndecided {
		t.Fatalf("revoked state = %q, %v", state, err)
	}
}

func TestLoadConfigForProjectRequiresTrust(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeConfig := func(path, name string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("providers:\n  - name: "+name+"\n    protocol: openai\n    base_url: https://example.test\n    model: model\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig(filepath.Join(home, ".mygocode", "config.yaml"), "global")
	writeConfig(filepath.Join(project, ".mygocode", "config.yaml"), "project")

	cfg, state, err := LoadConfigForProject(home, project)
	if err != nil || state != TrustUndecided || cfg.Providers[0].Name != "global" {
		t.Fatalf("untrusted load = %#v, %q, %v", cfg, state, err)
	}
	if err := SetProjectTrust(home, project, TrustTrusted); err != nil {
		t.Fatal(err)
	}
	cfg, state, err = LoadConfigForProject(home, project)
	if err != nil || state != TrustTrusted || cfg.Providers[0].Name != "project" {
		t.Fatalf("trusted load = %#v, %q, %v", cfg, state, err)
	}
}

func TestCorruptTrustStoreNeverTrusts(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".mygocode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trustFilePath(home), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ProjectTrustState(home, project)
	if err == nil || state != TrustUndecided {
		t.Fatalf("corrupt store state = %q, %v", state, err)
	}
}
