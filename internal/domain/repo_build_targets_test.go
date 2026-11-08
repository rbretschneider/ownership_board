package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestRepoBuildTargets(t *testing.T) {
	for _, os := range []BuildHostOS{"windows", "linux", "macos"} {
		for _, preset := range []string{"dotnet", "cmake", "script"} {
			targets := []RepoBuildTarget{{OS: os, Preset: preset, Capabilities: []string{"msvc", "gl", "gl"}}}
			repo := Repo{}
			if err := repo.Restyle(RepoStyle{BuildTargets: &targets}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(repo.BuildTargets[0].Capabilities, []string{"gl", "msvc"}) {
				t.Fatal(repo.BuildTargets)
			}
			targets[0].Capabilities[0] = "changed"
			if repo.BuildTargets[0].Capabilities[1] != "msvc" {
				t.Fatal("shared capabilities")
			}
			if err := repo.Restyle(RepoStyle{}); err != nil || len(repo.BuildTargets) != 1 {
				t.Fatal("omitted edit")
			}
			empty := []RepoBuildTarget{}
			if err := repo.Restyle(RepoStyle{BuildTargets: &empty}); err != nil || repo.BuildTargets == nil || len(repo.BuildTargets) != 0 {
				t.Fatal("clear", err)
			}
		}
	}
	for name, targets := range map[string][]RepoBuildTarget{
		"os": {{OS: "other", Preset: "cmake"}}, "preset": {{OS: "linux", Preset: "other"}},
		"token":                 {{OS: "linux", Preset: "cmake", Capabilities: []string{"Bad"}}},
		"long token":            {{OS: "linux", Preset: "cmake", Capabilities: []string{strings.Repeat("a", 65)}}},
		"too many capabilities": {{OS: "linux", Preset: "cmake", Capabilities: make([]string, 33)}},
		"too many targets":      make([]RepoBuildTarget, 33),
		"duplicate":             {{OS: "linux", Preset: "cmake", Capabilities: []string{"gl", "msvc"}}, {OS: "linux", Preset: "cmake", Capabilities: []string{"msvc", "gl", "gl"}}},
	} {
		t.Run(name, func(t *testing.T) {
			repo := Repo{BuildTargets: []RepoBuildTarget{{OS: "windows", Preset: "script"}}}
			before := repo.BuildTargets
			if err := repo.Restyle(RepoStyle{BuildTargets: &targets}); err == nil {
				t.Fatal("accepted invalid targets")
			}
			if !reflect.DeepEqual(before, repo.BuildTargets) {
				t.Fatal("partial mutation")
			}
		})
	}
}
