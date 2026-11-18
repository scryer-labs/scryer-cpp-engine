package engine

import (
	"github.com/scryer-labs/scryer-cpp-engine/internal/clangscan"
	"github.com/scryer-labs/scryer-cpp-engine/internal/testclassification"
)

func (g *callGraph) classifyRegisteredTests(result *clangscan.Result, symbols rangeIndex) {
	byID := make(map[string]int, len(g.Symbols))
	for i, symbol := range g.Symbols {
		byID[symbol.ID] = i
	}
	for _, call := range result.Calls {
		evidence := testclassification.Registration(call.SymbolUSR, call.Target.File, call.Site.File, call.Site.Line, call.Site.Column)
		if evidence == nil {
			continue
		}
		id := g.enclosingSymbol(symbols, call.Site, "")
		i, exists := byID[id]
		if !exists {
			continue
		}
		symbol := &g.Symbols[i]
		// Registration generated at a macro-defined function's start establishes
		// that function's test role. A registration call inside an ordinary
		// function does not make that function or its helpers test-only.
		if symbol.Kind == "function" && symbol.Definition.Start == (position{Line: call.Site.Line, Column: call.Site.Column}) {
			symbol.Role, symbol.TestContext = "TEST", evidence
		}
	}
}
