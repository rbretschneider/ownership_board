package domain

import "encoding/json"

// RepoBuildTarget declares a repository's build-host verification policy.
type RepoBuildTarget struct {
	OS           BuildHostOS `json:"os"`
	Capabilities []string    `json:"capabilities"`
	Preset       string      `json:"preset"`
}

// NormalizeRepoBuildTargets validates a replacement and gives it independent storage.
func NormalizeRepoBuildTargets(targets []RepoBuildTarget) ([]RepoBuildTarget, error) {
	if len(targets) > 32 {
		return nil, Invalidf("at most 32 build targets are allowed")
	}
	out := make([]RepoBuildTarget, 0, len(targets))
	seen := map[string]bool{}
	for _, t := range targets {
		target := BuildJobTarget{OS: t.OS, RequiredCapabilities: t.Capabilities}
		if err := target.Validate(); err != nil {
			return nil, err
		}
		if t.Preset != "dotnet" && t.Preset != "cmake" && t.Preset != "script" {
			return nil, Invalidf("invalid build target preset")
		}
		t.Capabilities = target.RequiredCapabilities
		key, _ := json.Marshal(t)
		if seen[string(key)] {
			return nil, Invalidf("duplicate build target")
		}
		seen[string(key)] = true
		out = append(out, t)
	}
	return out, nil
}
