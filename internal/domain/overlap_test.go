package domain

import (
	"reflect"
	"testing"
)

func TestDesignFilesKeepsSourcePathsAndDropsNoise(t *testing.T) {
	design := "Modify `src/rebuild/src/RebuildEngine.cpp`, tests/unit/RebuildTests.cpp and ./src/a/Thing.h.\n" +
		"Read-only: tests/CMakeLists.txt, docs/amendments/94.md, .ownership/windows_verify.ps1, .github/workflows/ci.yml,\n" +
		"inc/X.h, origin/main...HEAD, tests/Mix3rr.Core, Mix3rr.sln, src/App/App.csproj and https://learn.microsoft.com/a/b.html."
	got := DesignFiles(design)
	want := map[string]bool{"src/rebuild/src/RebuildEngine.cpp": true, "tests/unit/RebuildTests.cpp": true, "src/a/Thing.h": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestHubFilesAreSharedRegistriesAndDoNotCountAsOverlap(t *testing.T) {
	catalog := "src/messages/src/CatalogEntries.cpp"
	var designs []map[string]bool
	for i := range 8 {
		files := map[string]bool{"src/own/File" + string(rune('A'+i)) + ".cpp": true}
		if i < 3 {
			files[catalog] = true
		}
		designs = append(designs, files)
	}
	hubs := HubFiles(designs)
	if !hubs[catalog] || len(hubs) != 1 {
		t.Fatalf("hubs %v", hubs)
	}
	a := map[string]bool{catalog: true, "src/x/Shared.cpp": true}
	b := map[string]bool{catalog: true, "src/x/Shared.cpp": true}
	if got := SharedFiles(a, b, hubs); !reflect.DeepEqual(got, []string{"src/x/Shared.cpp"}) {
		t.Fatalf("shared %v", got)
	}
	if len(HubFiles(designs[:5])) != 0 {
		t.Fatal("too few designs to call anything a hub")
	}
}
