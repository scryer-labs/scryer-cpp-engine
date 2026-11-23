package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCatchCompilerRole(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local compiler-backed acceptance uses the macOS toolchain")
	}
	root := t.TempDir()
	header := filepath.Join(root, "catch2/internal/catch_test_registry.hpp")
	if err := os.MkdirAll(filepath.Dir(header), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(header, []byte("namespace Catch { struct AutoReg { AutoReg(int) {} }; }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := `#include "catch2/internal/catch_test_registry.hpp"
int helper(){return 1;}
#define CUSTOM_CASE(name) static void generated_test(); static Catch::AutoReg reg(0); static void generated_test()
CUSTOM_CASE("case") { helper(); }
`
	if err := os.WriteFile(filepath.Join(root, "test.cpp"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	db, _ := json.Marshal([]map[string]any{{"directory": root, "file": filepath.Join(root, "test.cpp"), "arguments": []string{"/usr/bin/clang++", "-std=c++17", "-c", filepath.Join(root, "test.cpp")}}})
	if err := os.WriteFile(filepath.Join(root, "compile_commands.json"), db, 0600); err != nil {
		t.Fatal(err)
	}
	request := Request{}
	request.Scan.Repository = root
	request.Scan.Facts = []string{"call-graph"}
	static := false
	request.Scan.Static = &static
	report, failure := request.scan(context.Background())
	if failure != nil {
		t.Fatal(failure)
	}
	graph := report.Facts["call-graph"].(callGraph)
	found := false
	for _, symbol := range graph.Symbols {
		if symbol.Kind == "macro-body" && symbol.Role == "TEST" && symbol.TestContext.Evidence[0].Framework == "catch2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing compiler-bound Catch registration: %+v; semantic=%+v", graph.Symbols, graph.Semantic)
	}
}
