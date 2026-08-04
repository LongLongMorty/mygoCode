package llm

import (
	"testing"

	"mygocode/internal/config"
)

// TestResolveAuxModel verifies alias resolution and passthrough of
// provider-specific model names.
func TestResolveAuxModel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"haiku", "claude-haiku-4-5-20251001"},
		{"sonnet", "claude-sonnet-4-6-20250514"},
		{"opus", "claude-opus-4-6-20250514"},
		{"glm-5.1", "glm-5.1"},
		{"deepseek-v4-flash", "deepseek-v4-flash"},
	}
	for _, tt := range tests {
		if got := ResolveAuxModel(tt.in); got != tt.want {
			t.Errorf("ResolveAuxModel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestAuxClientConfig verifies the copy semantics: only Model changes, all
// other fields are inherited, and nil is returned when no aux model is set.
func TestAuxClientConfig(t *testing.T) {
	// No aux model → nil (callers fall back to the main config).
	plain := &config.ProviderConfig{
		Name: "claude", Protocol: "anthropic", BaseURL: "https://api.anthropic.com",
		Model: "claude-sonnet-4-6-20250514", ContextWindow: 200000,
		MaxOutputTokens: 8192, Thinking: true,
	}
	if got := AuxClientConfig(plain); got != nil {
		t.Fatalf("AuxClientConfig without aux_model = %+v, want nil", got)
	}
	if got := AuxClientConfig(nil); got != nil {
		t.Fatalf("AuxClientConfig(nil) = %+v, want nil", got)
	}

	// Aux model set → copy with Model replaced, everything else identical.
	withAux := &config.ProviderConfig{
		Name: "claude", Protocol: "anthropic", BaseURL: "https://api.anthropic.com",
		Model: "claude-sonnet-4-6-20250514", AuxModel: "haiku",
		ContextWindow: 200000, MaxOutputTokens: 64000, Thinking: true, APIKey: "sk-test",
	}
	got := AuxClientConfig(withAux)
	if got == nil {
		t.Fatal("AuxClientConfig with aux_model returned nil")
	}
	if got.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("Model = %q, want resolved haiku ID", got.Model)
	}
	if got.Name != withAux.Name || got.Protocol != withAux.Protocol ||
		got.BaseURL != withAux.BaseURL || got.APIKey != withAux.APIKey ||
		got.ContextWindow != withAux.ContextWindow || got.MaxOutputTokens != withAux.MaxOutputTokens ||
		got.Thinking != withAux.Thinking {
		t.Errorf("aux config lost inherited fields: %+v", got)
	}
	// The source config must not be mutated.
	if withAux.Model != "claude-sonnet-4-6-20250514" {
		t.Errorf("source config mutated: Model = %q", withAux.Model)
	}
}
