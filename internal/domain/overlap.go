package domain

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

// Builds that edit the same files in parallel end in merge conflicts, and
// the second PR spends a pass merging the first. A design names the files it
// will touch, so the board holds back a build whose design shares files with
// a build already under way in the same repo (2026-11-06).

// designPath matches a repo-relative path with at least one directory and a
// lower-case file extension, such as src/rebuild/src/RebuildEngine.cpp. The
// lower case leaves out dotted folders such as tests/Mix3rr.Core.
var designPath = regexp.MustCompile("(?:^|[\\s`'\"(\\[<,])((?:\\./)?(?:\\.?[A-Za-z0-9_-]+/)+[A-Za-z0-9_.-]+\\.[a-z][a-z0-9]{0,7})(?:$|[\\s`'\"):\\],.;<])")

// ignoredNames are build and project files nearly every story touches with
// small, mechanical edits that merge cleanly or are resolved in one pass.
var ignoredNames = map[string]bool{
	"CMakeLists.txt": true, "CMakePresets.json": true, "vcpkg.json": true,
	"Directory.Build.props": true, "Directory.Packages.props": true, "package.json": true, "package-lock.json": true,
	"go.mod": true, "go.sum": true,
}

var ignoredExtensions = map[string]bool{".md": true, ".sln": true, ".csproj": true, ".txt": true}

var ignoredPrefixes = []string{".ownership/", ".github/", "docs/", "origin/"}

// DesignFiles returns the files a design names that matter for conflicts.
func DesignFiles(design string) map[string]bool {
	files := map[string]bool{}
	for _, match := range designPath.FindAllStringSubmatch(design, -1) {
		file := strings.TrimPrefix(match[1], "./")
		name := path.Base(file)
		stem := strings.TrimSuffix(name, path.Ext(name))
		if ignoredNames[name] || ignoredExtensions[strings.ToLower(path.Ext(name))] || len(stem) < 2 || strings.Contains(file, "..") {
			continue
		}
		if hasAnyPrefix(file, ignoredPrefixes) {
			continue
		}
		files[file] = true
	}
	return files
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

// Hub files are a repo's shared registries, such as a message catalog or a
// module list, that a large share of its designs edit. Holding builds back on
// them would run the repo one story at a time, and their edits are appends
// that merge easily, so they do not count as overlap.
const (
	hubMinDesigns = 6
	hubShare      = 0.25
)

// HubFiles returns the files named by at least a quarter of a repo's
// designs, once the repo has enough designs to tell.
func HubFiles(designs []map[string]bool) map[string]bool {
	hubs := map[string]bool{}
	if len(designs) < hubMinDesigns {
		return hubs
	}
	counts := map[string]int{}
	for _, files := range designs {
		for file := range files {
			counts[file]++
		}
	}
	for file, count := range counts {
		if float64(count) >= hubShare*float64(len(designs)) {
			hubs[file] = true
		}
	}
	return hubs
}

// SharedFiles returns the files two designs both name, leaving out hubs.
func SharedFiles(a, b, hubs map[string]bool) []string {
	var shared []string
	for file := range a {
		if b[file] && !hubs[file] {
			shared = append(shared, file)
		}
	}
	sort.Strings(shared)
	return shared
}
