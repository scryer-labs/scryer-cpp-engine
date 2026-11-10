package cmake

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func Test_should_collect_literal_facts_and_preserve_unevaluated_context(t *testing.T) {
	root := t.TempDir()
	source := `#[=[
add_library(ignored STATIC)
]=]
set(ignored [=[target_link_libraries(app ghost)]=])
target_compile_definitions(app PRIVATE NAME="quoted # ) text")
ADD_EXECUTABLE(app WIN32 main.cpp)
add_library(lib STATIC lib.cpp)
add_library(ns::lib ALIAS lib)
if(WIN32 AND (MSVC OR MINGW))
  target_link_libraries(app PRIVATE
    debug "ns::lib" optimized "[literal#)]"
    "${VAR}"
    "$<IF:$<BOOL:${OTHER}>,lib,none>")
else()
  find_package(Other REQUIRED)
endif()
FetchContent_Declare(dep URL "https://example.invalid/a(b).zip")
add_subdirectory("sub folder")
`
	if err := os.WriteFile(filepath.Join(root, "CMakeLists.txt"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	facts, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Targets) != 3 || facts.Targets[0].Name != "app" ||
		facts.Targets[0].Line != 6 || facts.Targets[1].Type != "STATIC" ||
		facts.Targets[2].AliasOf != "lib" {
		t.Fatalf("unexpected targets: %+v", facts.Targets)
	}
	if len(facts.Links) != 2 || facts.Links[0].To != "ns::lib" ||
		facts.Links[0].Line != 11 || facts.Links[0].Visibility != "PRIVATE" ||
		facts.Links[0].Configuration != "debug" ||
		facts.Links[1].To != "[literal#)]" || facts.Links[1].Configuration != "optimized" ||
		!reflect.DeepEqual(facts.Links[0].Context, []string{"if(WIN32 AND ( MSVC OR MINGW ))"}) {
		t.Fatalf("unexpected links: %+v", facts.Links)
	}
	if len(facts.Unresolved) != 3 || facts.Unresolved[1].Target != "app" ||
		facts.Unresolved[1].Expression != "${VAR}" || !strings.HasPrefix(facts.Unresolved[2].Expression, "$<IF:") {
		t.Fatalf("unexpected unresolved facts: %+v", facts.Unresolved)
	}
	if len(facts.ExternalDependencies) != 2 || facts.ExternalDependencies[0].Name != "Other" ||
		facts.ExternalDependencies[0].Context[1] != "else()" ||
		facts.ExternalDependencies[1].Arguments[1] != "https://example.invalid/a(b).zip" ||
		len(facts.Subdirectories) != 1 || facts.Subdirectories[0].Path != "sub folder" {
		t.Fatalf("unexpected build declarations: %+v", facts)
	}
}

func Test_should_preserve_prior_declarations_when_a_file_has_incomplete_syntax(t *testing.T) {
	var facts Facts
	err := facts.scanFile(context.Background(), "test.cmake", "add_library(ok STATIC)\nadd_executable(broken\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Targets) != 1 || facts.Targets[0].Name != "ok" || len(facts.Unresolved) != 1 ||
		facts.Unresolved[0].File != "test.cmake" || facts.Unresolved[0].Command != "add_executable" {
		t.Fatalf("unexpected scan facts: %+v", facts)
	}
}
