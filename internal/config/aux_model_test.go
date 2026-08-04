package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestParseAuxModel verifies the aux_model field is parsed from YAML and
// defaults to the empty string when absent.
func TestParseAuxModel(t *testing.T) {
	var cfg AppConfig
	data := `
providers:
  - name: claude
    protocol: anthropic
    base_url: https://api.anthropic.com
    model: claude-sonnet-4-6-20250514
    aux_model: haiku
`
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("parse with aux_model: %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.Providers))
	}
	if got := cfg.Providers[0].AuxModel; got != "haiku" {
		t.Errorf("AuxModel = %q, want %q", got, "haiku")
	}

	var absent AppConfig
	dataNoAux := `
providers:
  - name: openai
    protocol: openai
    base_url: https://api.openai.com/v1
    model: gpt-4.1
`
	if err := yaml.Unmarshal([]byte(dataNoAux), &absent); err != nil {
		t.Fatalf("parse without aux_model: %v", err)
	}
	if got := absent.Providers[0].AuxModel; got != "" {
		t.Errorf("AuxModel = %q, want empty when absent", got)
	}
}

// TestMergeConfig_AuxModel verifies provider-level merging carries aux_model
// through when the override replaces the provider.
func TestMergeConfig_AuxModel(t *testing.T) {
	base := &AppConfig{Providers: []ProviderConfig{{
		Name: "claude", Protocol: "anthropic", Model: "sonnet",
	}}}
	override := &AppConfig{Providers: []ProviderConfig{{
		Name: "claude", Protocol: "anthropic", Model: "opus", AuxModel: "haiku",
	}}}
	merged := mergeConfig(base, override)
	if len(merged.Providers) != 1 {
		t.Fatalf("expected 1 provider after merge, got %d", len(merged.Providers))
	}
	p := merged.Providers[0]
	if p.Model != "opus" || p.AuxModel != "haiku" {
		t.Errorf("merged provider = %+v, want model=opus aux_model=haiku", p)
	}

	// AuxModel alone must not leak into a provider that doesn't set it.
	mergedNoAux := mergeConfig(base, &AppConfig{Providers: []ProviderConfig{{
		Name: "claude", Protocol: "anthropic", Model: "sonnet",
	}}})
	if mergedNoAux.Providers[0].AuxModel != "" {
		t.Errorf("AuxModel = %q, want empty", mergedNoAux.Providers[0].AuxModel)
	}
}

// TestValidateProviders_AuxModel ensures validation does not reject a
// provider merely for carrying an aux_model.
func TestValidateProviders_AuxModel(t *testing.T) {
	cfg := &AppConfig{Providers: []ProviderConfig{{
		Name: "claude", Protocol: "anthropic",
		BaseURL: "https://api.anthropic.com", Model: "claude-sonnet-4-6-20250514",
		AuxModel: "haiku",
	}}}
	if err := validateProviders(cfg); err != nil {
		t.Fatalf("validateProviders with aux_model: %v", err)
	}
	if strings.TrimSpace(cfg.Providers[0].AuxModel) == "" {
		t.Error("aux_model was lost after validation")
	}
}
