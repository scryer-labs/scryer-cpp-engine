package engine

import (
	"cmp"
	"slices"
	"sort"
	"strconv"

	"github.com/scryer-labs/scryer-cpp-engine/internal/clangscan"
)

type semanticCallEdge struct {
	clangscan.Edge
	ID             string `json:"semanticEdgeId"`
	CallSiteID     string `json:"callSiteId,omitempty"`
	CallerSymbolID string `json:"callerSymbolId,omitempty"`
	TargetSymbolID string `json:"targetSymbolId,omitempty"`
}

type semanticCallFacts struct {
	Resolution *clangscan.Result  `json:"resolution"`
	Edges      []semanticCallEdge `json:"edges"`
}

type indexedRange struct {
	start, end, maxEnd position
	index              int
}

type rangeIndex map[string][]indexedRange

func comparePosition(a, b position) int {
	if order := cmp.Compare(a.Line, b.Line); order != 0 {
		return order
	}
	return cmp.Compare(a.Column, b.Column)
}

func (index rangeIndex) add(span sourceRange, i int) {
	index[span.File] = append(index[span.File], indexedRange{start: span.Start, end: span.End, index: i})
}

func (index rangeIndex) prepare() {
	for _, ranges := range index {
		slices.SortStableFunc(ranges, func(a, b indexedRange) int { return comparePosition(a.start, b.start) })
		var maximum position
		for i := range ranges {
			if comparePosition(ranges[i].end, maximum) > 0 {
				maximum = ranges[i].end
			}
			ranges[i].maxEnd = maximum
		}
	}
}

func (index rangeIndex) visit(site clangscan.Location, match func(int)) {
	ranges := index[site.File]
	point := position{Line: site.Line, Column: site.Column}
	end := sort.Search(len(ranges), func(i int) bool { return comparePosition(ranges[i].start, point) > 0 })
	for i := end - 1; i >= 0 && comparePosition(ranges[i].maxEnd, point) > 0; i-- {
		if comparePosition(ranges[i].end, point) > 0 {
			match(ranges[i].index)
		}
	}
}

func (g *callGraph) joinSemantic(result *clangscan.Result) {
	g.Semantic = &semanticCallFacts{Resolution: result, Edges: make([]semanticCallEdge, 0, len(result.Calls))}
	if len(result.Calls) == 0 {
		return
	}
	calls, symbols := rangeIndex{}, rangeIndex{}
	for i, call := range g.Calls {
		if call.name != "" {
			calls.add(call.calleeRange, i)
		}
	}
	for i, symbol := range g.Symbols {
		symbols.add(symbol.Definition, i)
	}
	calls.prepare()
	symbols.prepare()
	for i, fact := range result.Calls {
		edge := semanticCallEdge{Edge: fact, ID: "clang:" + strconv.Itoa(i+1)}
		point := position{Line: fact.Site.Line, Column: fact.Site.Column}
		best, score, ambiguous := -1, 0, false
		calls.visit(fact.Site, func(index int) {
			call := g.Calls[index]
			if call.name != fact.Name {
				return
			}
			rank := 1
			if comparePosition(call.nameRange.Start, point) <= 0 && comparePosition(point, call.nameRange.End) < 0 {
				rank = 2
			}
			if rank > score {
				best, score, ambiguous = index, rank, false
			} else if rank == score {
				ambiguous = true
			}
		})
		if best >= 0 && !ambiguous {
			call := &g.Calls[best]
			call.SemanticEdgeIDs = append(call.SemanticEdgeIDs, edge.ID)
			edge.CallSiteID, edge.CallerSymbolID = call.ID, call.CallerID
		}
		if edge.CallerSymbolID == "" {
			edge.CallerSymbolID = g.enclosingSymbol(symbols, fact.Site, "")
		}
		if fact.TargetKind == "definition" {
			edge.TargetSymbolID = g.enclosingSymbol(symbols, fact.Target, fact.Name)
		}
		g.Semantic.Edges = append(g.Semantic.Edges, edge)
	}
}

func (g *callGraph) enclosingSymbol(index rangeIndex, site clangscan.Location, name string) string {
	var best *functionSymbol
	index.visit(site, func(i int) {
		symbol := &g.Symbols[i]
		if name != "" && symbol.baseName != name {
			return
		}
		if best == nil || comparePosition(symbol.Definition.Start, best.Definition.Start) > 0 ||
			(symbol.Definition.Start == best.Definition.Start && comparePosition(symbol.Definition.End, best.Definition.End) < 0) {
			best = symbol
		}
	})
	if best == nil {
		return ""
	}
	return best.ID
}
