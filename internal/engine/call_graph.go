package engine

import (
	"strconv"

	"github.com/scryer-labs/scryer-cpp-engine/internal/testclassification"

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
	Projection        string                      `json:"projection,omitempty"`
	Role              string                      `json:"role,omitempty"`
	TestContext       *testclassification.Context `json:"testContext,omitempty"`
	impactKey         string
	impactSignature   string
	impactFingerprint string
	impactSource      string
	ExecutionKind     string `json:"executionKind,omitempty"`
	ID                string `json:"symbolId"`
	baseName          string
	Name              string      `json:"name,omitempty"`
	Kind              string      `json:"kind"`
	Definition        sourceRange `json:"definition"`
}

type callSite struct {
	CandidateIDs    []string `json:"candidateSymbolIds,omitempty"`
	CandidateBasis  string   `json:"candidateBasis,omitempty"`
	ID              string   `json:"callSiteId"`
	SemanticEdgeIDs []string `json:"semanticEdgeIds,omitempty"`
	name            string
	nameRange       sourceRange
	calleeRange     sourceRange
	CallerID        string      `json:"callerSymbolId,omitempty"`
	Callee          string      `json:"callee"`
	CandidateID     string      `json:"candidateSymbolId,omitempty"`
	Resolution      string      `json:"resolution"`
	Site            sourceRange `json:"site"`
}

type callGraph struct {
	SymbolReferences []symbolReference       `json:"symbolReferences,omitempty"`
	MacroExpansions  []macroExpansion        `json:"macroExpansions,omitempty"`
	Completeness     *collectionCompleteness `json:"completeness,omitempty"`
	withImpact       bool
	Semantic         *semanticCallFacts `json:"semantic,omitempty"`
	ProjectID        string             `json:"projectId"`
	Basis            string             `json:"basis"`
	Symbols          []functionSymbol   `json:"symbols"`
	Calls            []callSite         `json:"calls"`
}

type functionScope struct {
	id  string
	end uint
}

func (g *callGraph) collect(node *sitter.Node, file string, source []byte, scopes []functionScope) []functionScope {
	for len(scopes) > 0 && node.StartByte() >= scopes[len(scopes)-1].end {
		scopes = scopes[:len(scopes)-1]
	}
	scopes = g.macroBody(node, file, source, scopes)
	switch node.Kind() {
	case "function_definition", "lambda_expression", "declaration":
		variable := templateVariableName(node)
		if node.Kind() == "declaration" && variable == nil {
			break
		}
		symbol := functionSymbol{
			ID:         file + ":" + strconv.FormatUint(uint64(node.StartByte()), 10),
			Kind:       "function",
			Definition: location(file, node),
		}
		if variable != nil {
			symbol.Kind, symbol.ExecutionKind = "variable-template", "declaration"
			symbol.Name, symbol.baseName = variable.Utf8Text(source), variable.Utf8Text(source)
		} else if node.Kind() == "lambda_expression" {
			symbol.Kind = "lambda"
		} else {
			if name := functionName(node.ChildByFieldName("declarator")); name != nil {
				symbol.Name = name.Utf8Text(source)
				if leaf := terminalName(name); leaf != nil {
					symbol.baseName = leaf.Utf8Text(source)
				}
			}
		}
		if node.Kind() == "function_definition" && hasConsteval(node, source) {
			symbol.ExecutionKind = "compile-time"
		}
		if g.withImpact {
			symbol.impactKey, symbol.impactSignature, symbol.impactFingerprint = impactIdentity(node, file, source)
			if symbol.impactKey != "" {
				symbol.impactSource = node.Utf8Text(source)
			}
		}
		g.Symbols = append(g.Symbols, symbol)
		scopes = append(scopes, functionScope{id: symbol.ID, end: node.EndByte()})
	case "call_expression":
		callee := node.ChildByFieldName("function")
		if callee != nil {
			call := callSite{
				ID:     file + ":" + strconv.FormatUint(uint64(node.StartByte()), 10) + "-" + strconv.FormatUint(uint64(node.EndByte()), 10),
				Callee: callee.Utf8Text(source), Site: location(file, node), Resolution: "unresolved",
				calleeRange: location(file, callee),
			}
			if name := terminalName(callee); name != nil {
				call.name = name.Utf8Text(source)
				call.nameRange = location(file, name)
			}
			if len(scopes) > 0 {
				call.CallerID = scopes[len(scopes)-1].id
			}
			g.Calls = append(g.Calls, call)
		}
	}
	return scopes
}

func functionName(node *sitter.Node) *sitter.Node {
	for node != nil {
		switch node.Kind() {
		case "identifier", "field_identifier", "qualified_identifier", "destructor_name", "operator_name", "operator_cast":
			return node
		case "reference_declarator", "parenthesized_declarator":
			node = node.NamedChild(0)
		default:
			node = node.ChildByFieldName("declarator")
		}
	}
	return nil
}

func terminalName(node *sitter.Node) *sitter.Node {
	for node != nil {
		switch node.Kind() {
		case "identifier", "field_identifier", "destructor_name", "operator_name", "operator_cast":
			return node
		case "qualified_identifier", "template_function", "template_method":
			node = node.ChildByFieldName("name")
		case "field_expression":
			node = node.ChildByFieldName("field")
		case "parenthesized_expression":
			node = node.NamedChild(0)
		default:
			return nil
		}
	}
	return nil
}

// A unique spelling is a navigation hint, not proof of C++ name lookup.
func (g *callGraph) findCandidates() {
	byName := make(map[string]string)
	for _, symbol := range g.Symbols {
		if symbol.Name == "" || symbol.Kind == "variable-template" {
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
