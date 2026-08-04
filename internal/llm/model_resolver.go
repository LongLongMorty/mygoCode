package llm

import "mygocode/internal/config"

var modelAliases = map[string]string{

	"haiku":  "claude-haiku-4-5-20251001",
	"sonnet": "claude-sonnet-4-6-20250514",
	"opus":   "claude-opus-4-6-20250514",
}

func NewModelResolver(baseCfg config.ProviderConfig) func(string) (Client, error) {
	return func(shortName string) (Client, error) {
		modelID, ok := modelAliases[shortName]
		if !ok {
			modelID = shortName
		}

		cfg := baseCfg
		cfg.Model = modelID
		return NewClient(&cfg, "")
	}
}

// ResolveAuxModel applies the short-name alias table to an aux_model value
// (e.g. "haiku" → the concrete Claude Haiku model ID). Unmatched names are
// returned verbatim so provider-specific model IDs pass through unchanged.
func ResolveAuxModel(name string) string {
	if id, ok := modelAliases[name]; ok {
		return id
	}
	return name
}

// AuxClientConfig returns a copy of cfg with Model replaced by the resolved
// auxiliary model, or nil when no aux model is configured. All other fields
// (protocol, base_url, context window, thinking, …) are inherited from the
// main provider so the aux client speaks to the same endpoint with the same
// limits. Callers must fall back to the main config when this returns nil.
func AuxClientConfig(cfg *config.ProviderConfig) *config.ProviderConfig {
	if cfg == nil || cfg.AuxModel == "" {
		return nil
	}
	cp := *cfg
	cp.Model = ResolveAuxModel(cfg.AuxModel)
	return &cp
}
