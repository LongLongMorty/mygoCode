package skills

// LoadBuiltins returns embedded skills compiled into the binary.
// Currently empty — all skills are loaded from disk at runtime
// (user-level ~/.mygocode/skills/ or project-level .mygocode/skills/).
func LoadBuiltins() []*Skill {
	return nil
}
