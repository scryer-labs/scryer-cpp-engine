package engine

import (
	"testing"

	"github.com/scryer-labs/scryer-cpp-engine/internal/cmake"
)

func TestIncludeSearchUsesOwnPathsAndTransitiveInterfaces(t *testing.T) {
	root := t.TempDir()
	loc := cmake.Location{File: "lib/CMakeLists.txt", Line: 3}
	build := cmake.Facts{
		Targets: []cmake.Target{
			{Location: loc, Name: "app"},
			{Location: loc, Name: "middle"},
			{Location: loc, Name: "lib::middle", AliasOf: "middle"},
			{Location: loc, Name: "headers"},
		},
		IncludeDirectories: []cmake.IncludeDirectory{
			{Location: loc, Target: "app", Visibility: "PRIVATE", Expression: "$<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>"},
			{Location: loc, Target: "app", Visibility: "INTERFACE", Expression: "unused"},
			{Location: loc, Target: "middle", Visibility: "PRIVATE", Expression: "private"},
			{Location: loc, Target: "headers", Visibility: "INTERFACE", Expression: "headers"},
			{Location: loc, Target: "headers", Visibility: "INTERFACE", Expression: "$<INSTALL_INTERFACE:installed>"},
		},
		Links: []cmake.Link{
			{From: "app", To: "lib::middle", Visibility: "PRIVATE"},
			{From: "middle", To: "headers", Visibility: "PUBLIC"},
			{From: "headers", To: "middle", Visibility: "INTERFACE"},
		},
	}
	search := indexIncludePaths(root, build)
	ids := []string{componentID(build.Targets[0])}
	files := map[string]int{"lib/include/own.h": 0, "lib/headers/api.h": 1, "lib/private/private.h": 2,
		"lib/unused/unused.h": 3, "lib/installed/installed.h": 4, "lib/include/conflict.h": 5, "lib/headers/conflict.h": 6}
	for token, want := range map[string]string{"\"own.h\"": "lib/include/own.h", "<api.h>": "lib/headers/api.h", "<private.h>": "", "<unused.h>": "", "<installed.h>": ""} {
		result := resolveInclude(Include{File: "lib/src/app.cpp", Target: token}, ids, files, search)
		if result.To != want {
			t.Errorf("%s: got %+v, want %q", token, result, want)
		}
	}
	result := resolveInclude(Include{File: "lib/src/app.cpp", Target: "<conflict.h>"}, ids, files, search)
	if result.Resolution != "ambiguous" || result.To != "" || len(result.Candidates) != 2 {
		t.Fatalf("expected both candidates without choosing a winner: %+v", result)
	}
}
