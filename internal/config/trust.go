package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TrustState deliberately distinguishes an undecided project from a project
// the user has explicitly declined. Only Trusted permits project resources.
type TrustState string

const (
	TrustUndecided TrustState = "undecided"
	TrustDenied    TrustState = "denied"
	TrustTrusted   TrustState = "trusted"
)

type trustFile struct {
	Projects map[string]TrustState `json:"projects"`
}

func normalizeProjectPath(project string) (string, error) {
	abs, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func trustFilePath(home string) string {
	return filepath.Join(home, ".mygocode", "trusted-projects.json")
}

func readTrustFile(home string) (trustFile, error) {
	data, err := os.ReadFile(trustFilePath(home))
	if os.IsNotExist(err) {
		return trustFile{Projects: make(map[string]TrustState)}, nil
	}
	if err != nil {
		return trustFile{}, fmt.Errorf("read trusted projects: %w", err)
	}
	var file trustFile
	if err := json.Unmarshal(data, &file); err != nil {
		return trustFile{}, fmt.Errorf("parse trusted projects: %w", err)
	}
	if file.Projects == nil {
		file.Projects = make(map[string]TrustState)
	}
	return file, nil
}

func ProjectTrustState(home, project string) (TrustState, error) {
	path, err := normalizeProjectPath(project)
	if err != nil {
		return TrustUndecided, err
	}
	file, err := readTrustFile(home)
	if err != nil {
		// Corrupt or unreadable storage must never silently trust a project.
		return TrustUndecided, err
	}
	if state := file.Projects[path]; state == TrustTrusted || state == TrustDenied {
		return state, nil
	}
	return TrustUndecided, nil
}

func SetProjectTrust(home, project string, state TrustState) error {
	if state != TrustTrusted && state != TrustDenied {
		return fmt.Errorf("invalid trust state %q", state)
	}
	path, err := normalizeProjectPath(project)
	if err != nil {
		return err
	}
	file, err := readTrustFile(home)
	if err != nil {
		return err
	}
	file.Projects[path] = state
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(trustFilePath(home)), 0o700); err != nil {
		return err
	}
	return os.WriteFile(trustFilePath(home), append(data, '\n'), 0o600)
}

func RevokeProjectTrust(home, project string) error {
	path, err := normalizeProjectPath(project)
	if err != nil {
		return err
	}
	file, err := readTrustFile(home)
	if err != nil {
		return err
	}
	delete(file.Projects, path)
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(trustFilePath(home)), 0o700); err != nil {
		return err
	}
	return os.WriteFile(trustFilePath(home), append(data, '\n'), 0o600)
}

// LoadConfigForProject merges project configuration only for an explicitly
// trusted project. Global configuration remains available in every state.
func LoadConfigForProject(home, project string) (*AppConfig, TrustState, error) {
	state, stateErr := ProjectTrustState(home, project)
	if stateErr != nil {
		state = TrustUndecided
	}
	globalPath := filepath.Join(home, ".mygocode", "config.yaml")
	var merged *AppConfig
	if _, err := os.Stat(globalPath); err == nil {
		cfg, err := loadSingleFile(globalPath)
		if err != nil {
			return nil, state, err
		}
		merged = cfg
	}
	if state == TrustTrusted {
		for _, name := range []string{"config.yaml", "config.local.yaml"} {
			path := filepath.Join(project, ".mygocode", name)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				continue
			}
			cfg, err := loadSingleFile(path)
			if err != nil {
				return nil, state, err
			}
			if merged == nil {
				merged = cfg
			} else {
				merged = mergeConfig(merged, cfg)
			}
		}
	}
	if merged == nil {
		return nil, state, &ConfigError{Message: "No usable config found. Configure ~/.mygocode/config.yaml or trust this project in the TUI."}
	}
	if err := validateProviders(merged); err != nil {
		return nil, state, err
	}
	return merged, state, nil
}
