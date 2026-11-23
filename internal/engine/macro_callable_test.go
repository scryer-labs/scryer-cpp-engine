package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMacroBodyCallerProjection(t *testing.T) {
	root := t.TempDir()
	source := `#include <catch2/catch_test_macros.hpp>
SCENARIO("case") { int local=1; REQUIRE(helper()==local); }
void ordinary(){invoke();}
UNKNOWN_CASE("custom") { verify(); }
`
	if err := os.WriteFile(filepath.Join(root, "test.cpp"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	facts, err := scanSources(context.Background(), root, false, true, false, true)
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string]functionSymbol{}
	for _, symbol := range facts.graph.Symbols {
		if symbol.Kind == "macro-body" {
			bodies[symbol.Name] = symbol
			if symbol.Role != "" || symbol.impactKey == "" {
				t.Fatal(symbol)
			}
		}
	}
	if len(bodies) != 2 {
		t.Fatal(facts.graph.Symbols)
	}
	for _, call := range facts.graph.Calls {
		switch call.Callee {
		case "helper", "REQUIRE":
			if call.CallerID != bodies[`SCENARIO("case")`].ID {
				t.Fatal(call)
			}
		case "verify":
			if call.CallerID != bodies[`UNKNOWN_CASE("custom")`].ID {
				t.Fatal(call)
			}
		case "invoke":
			if call.CallerID == "" {
				t.Fatal(call)
			}
		}
	}
	if facts.graph.Completeness.Status != "partial" || facts.graph.Completeness.MacroBodyProjections != 2 {
		t.Fatal(facts.graph.Completeness)
	}
}
