package engine

import (
	"strconv"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type position struct {
	Line   uint `json:"line"`
	Column uint `json:"column"`
}

type sourceRange struct {
	File  string   `json:"file"`
	Start position `json:"start"`
	End   position `json:"end"`
}

func location(file string, node *sitter.Node) sourceRange {
	start, end := node.StartPosition(), node.EndPosition()
	return sourceRange{File: file, Start: position{start.Row + 1, start.Column + 1}, End: position{end.Row + 1, end.Column + 1}}
}

type functionSymbol struct {
	ID         string      `json:"symbolId"`
	Name       string      `json:"name,omitempty"`
	Kind       string      `json:"kind"`
	Definition sourceRange `json:"definition"`
}

type callSite struct {
	CallerID    string      `json:"callerSymbolId,omitempty"`
	Callee      string      `json:"callee"`
	CandidateID string      `json:"candidateSymbolId,omitempty"`
	Resolution  string      `json:"resolution"`
	Site        sourceRange `json:"site"`
}

type callGraph struct {
	ProjectID string           `json:"projectId"`
	Basis     string           `json:"basis"`
	Symbols   []functionSymbol `json:"symbols"`
	Calls     []callSite       `json:"calls"`
}

type functionScope struct {
	id  string
	end uint
}

func (g *callGraph) collect(node *sitter.Node, file string, source []byte, scopes []functionScope) []functionScope {
	for len(scopes) > 0 && node.StartByte() >= scopes[len(scopes)-1].end {
		scopes = scopes[:len(scopes)-1]
	}
	switch node.Kind() {
	case "function_definition", "lambda_expression":
		symbol := functionSymbol{
			ID:         file + ":" + strconv.FormatUint(uint64(node.StartByte()), 10),
			Kind:       "function",
			Definition: location(file, node),
		}
		if node.Kind() == "lambda_expression" {
			symbol.Kind = "lambda"
		} else {
			symbol.Name = functionName(node.ChildByFieldName("declarator"), source)
		}
		g.Symbols = append(g.Symbols, symbol)
		scopes = append(scopes, functionScope{id: symbol.ID, end: node.EndByte()})
	case "call_expression":
		callee := node.ChildByFieldName("function")
		if callee != nil {
			call := callSite{Callee: callee.Utf8Text(source), Site: location(file, node), Resolution: "unresolved"}
			if len(scopes) > 0 {
				call.CallerID = scopes[len(scopes)-1].id
			}
			g.Calls = append(g.Calls, call)
		}
	}
	return scopes
}

func functionName(node *sitter.Node, source []byte) string {
	for node != nil {
		switch node.Kind() {
		case "identifier", "field_identifier", "qualified_identifier", "destructor_name", "operator_name", "operator_cast":
			return node.Utf8Text(source)
		case "reference_declarator", "parenthesized_declarator":
			node = node.NamedChild(0)
		default:
			node = node.ChildByFieldName("declarator")
		}
	}
	return ""
}

// A unique spelling is a navigation hint, not proof of C++ name lookup.
func (g *callGraph) findCandidates() {
	byName := make(map[string]string)
	for _, symbol := range g.Symbols {
		if symbol.Name == "" {
			continue
		}
		if _, exists := byName[symbol.Name]; exists {
			byName[symbol.Name] = ""
		} else {
			byName[symbol.Name] = symbol.ID
		}
	}
	for i := range g.Calls {
		call := &g.Calls[i]
		id, exists := byName[call.Callee]
		if id != "" {
			call.Resolution, call.CandidateID = "name-candidate", id
		} else if exists {
			call.Resolution = "ambiguous-name"
		}
	}
}
