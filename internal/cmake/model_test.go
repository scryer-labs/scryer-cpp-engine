package cmake

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	selected := []string{}
	for name, source := range files {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		selected = append(selected, name)
	}
	return root, selected
}
func targetNames(facts Facts) map[string]bool {
	result := map[string]bool{}
	for _, v := range facts.Targets {
		result[v.Name] = true
	}
	return result
}

func TestDirectoryVariablesListsAndIncludedSourceContext(t *testing.T) {
	root, files := fixture(t, map[string]string{
		"CMakeLists.txt": `project(example LANGUAGES C CXX)
set(CORE_NAME core)
set(INHERITED parent)
add_subdirectory(lib)
add_subdirectory(app)
include(cmake/extra.cmake)
add_library(${AFTER_INCLUDE} INTERFACE)
`,
		"lib/CMakeLists.txt": `project(child)
set(INHERITED child)
aux_source_directory(source SOURCES)
set(HEADERS include/api.h)
add_library(${CORE_NAME} STATIC ${SOURCES} "${HEADERS}")
target_include_directories(${CORE_NAME} PUBLIC ${CMAKE_CURRENT_LIST_DIR}/include)
`,
		"app/CMakeLists.txt": `add_executable(${PROJECT_NAME} main.c)
add_library(${INHERITED} INTERFACE)
target_link_libraries(${PROJECT_NAME} PRIVATE ${CORE_NAME})
`,
		"cmake/extra.cmake": `set(AFTER_INCLUDE helper)
set(EXTRA_FILE "${CMAKE_CURRENT_LIST_DIR}/../extra.c")
add_library(extra STATIC ${EXTRA_FILE})
target_include_directories(extra PUBLIC include)
`,
		"lib/source/a.c":                 "int a(){return 1;}\n",
		"lib/source/b.cpp":               "int b(){return 2;}\n",
		"lib/source/skipped.c":           "int skipped(){return 0;}\n",
		"lib/source/nested/not_direct.c": "int nested(){return 0;}\n",
		"lib/include/api.h":              "int a();\n",
		"app/main.c":                     "int main(){return 0;}\n",
		"extra.c":                        "int extra(){return 0;}\n",
	})
	selected := []string{}
	for _, name := range files {
		if name != "lib/source/skipped.c" {
			selected = append(selected, name)
		}
	}
	facts, err := ScanFiles(context.Background(), root, selected)
	if err != nil {
		t.Fatal(err)
	}
	names := targetNames(facts)
	for _, name := range []string{"core", "example", "parent", "extra", "helper"} {
		if !names[name] {
			t.Fatalf("missing %s: %+v", name, facts)
		}
	}
	if names["child"] {
		t.Fatal("child scope leaked into sibling")
	}
	membership := map[string]string{}
	for _, source := range facts.Sources {
		membership[filepath.ToSlash(filepath.Clean(filepath.Join(source.SourceDirectory, source.Path)))] = source.Target
	}
	for file, owner := range map[string]string{"lib/source/a.c": "core", "lib/source/b.cpp": "core", "lib/include/api.h": "core", "app/main.c": "example", "extra.c": "extra"} {
		if membership[file] != owner {
			t.Fatalf("wrong owner for %s: %+v", file, facts.Sources)
		}
	}
	if membership["lib/source/skipped.c"] != "" || membership["lib/source/nested/not_direct.c"] != "" {
		t.Fatal("aux_source_directory escaped selection or recursed")
	}
	if len(facts.Links) != 1 || facts.Links[0].From != "example" || facts.Links[0].To != "core" {
		t.Fatal(facts.Links)
	}
	for _, source := range facts.Sources {
		if source.Target == "extra" && (source.File != "cmake/extra.cmake" || source.SourceDirectory != ".") {
			t.Fatal(source)
		}
	}
}

func TestConditionalAssignmentsDoNotChooseAHostBranch(t *testing.T) {
	root, _ := fixture(t, map[string]string{"CMakeLists.txt": `set(NAME initial)
if(WIN32)
set(NAME windows)
add_library(${NAME} INTERFACE)
else()
set(NAME unix)
add_library(${NAME} INTERFACE)
endif()
add_library(${NAME} STATIC main.c)
set(SAME common)
if(UNKNOWN)
set(SAME shared)
else()
set(SAME shared)
endif()
add_library(${SAME} INTERFACE)
`})
	facts, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	names := targetNames(facts)
	if !names["windows"] || !names["unix"] || !names["shared"] || names["initial"] {
		t.Fatal(facts.Targets)
	}
	if len(facts.Targets) != 3 {
		t.Fatal(facts.Targets)
	}
	found := false
	for _, u := range facts.Unresolved {
		found = found || u.Command == "add_library" && u.Expression == "${NAME}"
	}
	if !found {
		t.Fatal("lost conditional ambiguity", facts.Unresolved)
	}
	for _, target := range facts.Targets {
		if target.Name != "shared" && len(target.Context) == 0 {
			t.Fatal("lost branch provenance", target)
		}
	}
}

func TestUnknownCacheParentAndGlobAssignmentsDoNotUseOldValues(t *testing.T) {
	root, _ := fixture(t, map[string]string{
		"CMakeLists.txt": `set(NAME old)
add_subdirectory(child)
add_library(${NAME} INTERFACE)
set(SOURCE old.c)
file(GLOB SOURCE *.c)
add_library(globbed ${SOURCE})
set(CACHED initial)
set(CACHED replacement CACHE STRING "doc")
add_library(${CACHED} INTERFACE)
set(BRACKET [=[${LITERAL}]=])
add_library(${BRACKET} INTERFACE)
`,
		"child/CMakeLists.txt": `set(NAME modified PARENT_SCOPE)
set(NAME local)
`,
	})
	facts, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if targetNames(facts)["old"] || targetNames(facts)["initial"] {
		t.Fatal(facts.Targets)
	}
	for _, source := range facts.Sources {
		if strings.Contains(source.Path, "old.c") {
			t.Fatal("used stale glob source", source)
		}
	}
	if len(facts.Unresolved) < 3 {
		t.Fatal(facts.Unresolved)
	}
}

func TestIncludedConditionsAndCustomCommandsDoNotLeakStaleBindings(t *testing.T) {
	root, _ := fixture(t, map[string]string{
		"CMakeLists.txt": `set(NAME before)
include(cmake/config.cmake)
add_library(${NAME} INTERFACE)
set(CUSTOM before_call)
function(change_name)
  set(CUSTOM after_call PARENT_SCOPE)
endfunction()
change_name()
add_library(${CUSTOM} INTERFACE)
`,
		"cmake/config.cmake": `if(UNKNOWN)
set(NAME after)
else()
set(NAME after)
endif()
`,
	})
	facts, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	names := targetNames(facts)
	if !names["after"] || names["before"] || names["before_call"] || names["after_call"] {
		t.Fatal(facts.Targets)
	}
	found := false
	for _, item := range facts.Unresolved {
		found = found || item.Command == "change_name"
	}
	if !found {
		t.Fatal(facts.Unresolved)
	}
}

func TestOutsideSourcesStayUnresolvedWithoutInventedPaths(t *testing.T) {
	root, _ := fixture(t, map[string]string{"CMakeLists.txt": "add_library(app STATIC ../outside.c inside.c)\n", "inside.c": "int inside(){return 0;}\n"})
	facts, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Sources) != 1 || facts.Sources[0].Path != "inside.c" {
		t.Fatal(facts.Sources)
	}
	if len(facts.Unresolved) != 1 || facts.Unresolved[0].Expression != "../outside.c" {
		t.Fatal(facts.Unresolved)
	}
}
