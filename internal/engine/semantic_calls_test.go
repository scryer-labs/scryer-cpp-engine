package engine

import (
	"testing"

	"github.com/scryer-labs/scryer-cpp-engine/internal/clangscan"
)

func Test_should_bind_a_cross_unit_call_when_clang_reports_a_declaration(t *testing.T) {
	graph := callGraph{Symbols: []functionSymbol{
		{ID: "caller", baseName: "caller", Definition: sourceRange{File: "main.cpp", Start: position{2, 1}, End: position{4, 2}}},
		{ID: "int-overload", baseName: "value", Definition: sourceRange{File: "int.cpp", Start: position{1, 1}, End: position{1, 40}}},
		{ID: "double-overload", baseName: "value", Definition: sourceRange{File: "double.cpp", Start: position{1, 1}, End: position{1, 40}}},
	}}
	result := clangscan.Result{
		Calls: []clangscan.Edge{{
			TargetKind: "declaration", Name: "value", SymbolUSR: "value(int)",
			Site:   clangscan.Location{File: "main.cpp", Line: 3, Column: 5},
			Target: clangscan.Location{File: "main.cpp", Line: 1, Column: 5},
		}},
		Definitions: []clangscan.Definition{
			{USR: "value(int)", Name: "value", Location: clangscan.Location{File: "int.cpp", Line: 1, Column: 5}},
			{USR: "value(double)", Name: "value", Location: clangscan.Location{File: "double.cpp", Line: 1, Column: 8}},
		},
	}
	graph.joinSemantic(&result)
	edge := graph.Semantic.Edges[0]
	if edge.CallerSymbolID != "caller" || edge.TargetSymbolID != "int-overload" || edge.TargetKind != "definition" || edge.Target.File != "int.cpp" {
		t.Fatalf("compiler identity did not select the declared overload: %+v", edge)
	}
	result.Definitions[1].USR = "value(int)"
	graph.joinSemantic(&result)
	if graph.Semantic.Edges[0].TargetSymbolID != "" {
		t.Fatal("ambiguous compiler definitions must remain unresolved")
	}
}
