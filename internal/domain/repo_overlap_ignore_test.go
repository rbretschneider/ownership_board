package domain

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeRepoOverlapIgnore(t *testing.T) {
	for _, pattern := range []string{"src/messages/Catalog.cpp", "src/*/include/mc/*/*ErrorCodes.h", "src/é.go", strings.Repeat("é", 200)} {
		got, err := NormalizeRepoOverlapIgnore([]string{" " + pattern + " "})
		if err != nil || len(got) != 1 || got[0] != pattern {
			t.Fatalf("%q: %v %v", pattern, got, err)
		}
	}
	for _, pattern := range []string{"", " ", "../file.go", "src/a..go", "/src/file.go", "C:/src/file.go", "src//file.go", "src/./file.go", "src/[bad.go", "src/file\\", "src/a\x00.go", strings.Repeat("é", 201), string([]byte{255})} {
		if _, err := NormalizeRepoOverlapIgnore([]string{pattern}); err == nil {
			t.Errorf("accepted %q", pattern)
		}
	}
	for _, n := range []int{100, 101} {
		values := make([]string, n)
		for i := range values {
			values[i] = "src/file.go"
		}
		_, err := NormalizeRepoOverlapIgnore(values)
		if (err == nil) != (n == 100) {
			t.Fatal(n, err)
		}
	}
	got, err := NormalizeRepoOverlapIgnore(nil)
	if err != nil || got == nil {
		t.Fatal(got, err)
	}
	input := []string{"src/file.go"}
	got, _ = NormalizeRepoOverlapIgnore(input)
	input[0] = "other.go"
	if got[0] != "src/file.go" {
		t.Fatal("aliased input")
	}
	repo, err := NewManualRepo(ManualRepoDraft{FullName: "example/app"}, time.Now())
	if err != nil || repo.OverlapIgnore == nil {
		t.Fatal(repo, err)
	}
}

func TestDesignFilesRepoPatterns(t *testing.T) {
	design := "src/messages/Catalog.cpp  src/core/include/mc/core/CoreErrorCodes.h  src/core/Engine.cpp  src/core/deep/Engine.cpp  docs/guide.md  src/core/CMakeLists.txt"
	files := DesignFiles(design, "src/messages/Catalog.cpp", "src/*/include/mc/*/*ErrorCodes.h")
	if len(files) != 2 || !files["src/core/Engine.cpp"] || !files["src/core/deep/Engine.cpp"] {
		t.Fatal(files)
	}
	for _, pattern := range []string{"SRC/*/Engine.cpp", "src/**/Engine.cpp/extra", "*/Engine.cpp", "src/*/*.h"} {
		if !DesignFiles(design, pattern)["src/core/Engine.cpp"] {
			t.Fatal(pattern)
		}
	}
	if !matchesOverlapIgnore("src/core/Engine.cpp", []string{"src/c?re/[AE]*.cpp"}) || matchesOverlapIgnore("src/core/deep/Engine.cpp", []string{"src/**/Engine.cpp"}) {
		t.Fatal("glob syntax or recursive matching")
	}
	if DesignFiles(design, "src/*/Engine.cpp")["src/core/Engine.cpp"] || !DesignFiles(design, "src/*/Engine.cpp")["src/core/deep/Engine.cpp"] {
		t.Fatal("segment boundary")
	}
}

func TestRepoOverlapIgnoreRestyleOwnsSlice(t *testing.T) {
	repo := Repo{}
	patterns := []string{"src/file.go", "src/file.go"}
	if err := repo.Restyle(RepoStyle{OverlapIgnore: &patterns}); err != nil {
		t.Fatal(err)
	}
	patterns[0] = "src/other.go"
	if len(repo.OverlapIgnore) != 2 || repo.OverlapIgnore[0] != "src/file.go" {
		t.Fatal(repo.OverlapIgnore)
	}
	if err := repo.Restyle(RepoStyle{}); err != nil || len(repo.OverlapIgnore) != 2 {
		t.Fatal(repo, err)
	}
	patterns = nil
	if err := repo.Restyle(RepoStyle{OverlapIgnore: &patterns}); err != nil || repo.OverlapIgnore == nil || len(repo.OverlapIgnore) != 0 {
		t.Fatal(repo, err)
	}
}
