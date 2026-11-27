package engine

import (
	"path"
	"strconv"
	"strings"

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
	fileStatic        bool
	conditional       bool
	invalidDefinition bool
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
	fileStaticCall  bool
	localShadow     bool
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
	macroNames       map[string]bool
	cxxFiles         map[string]bool
	declaredNames    map[string]bool
	DynamicBindings  []dynamicBinding        `json:"dynamicBindings,omitempty"`
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
	id             string
	end            uint
	localNames     map[string]bool
	validCFunction bool
}

func (g *callGraph) collect(node *sitter.Node, file string, source []byte, scopes []functionScope) []functionScope {
	for len(scopes) > 0 && node.StartByte() >= scopes[len(scopes)-1].end {
		scopes = scopes[:len(scopes)-1]
	}
	if path.Ext(file) == ".c" {
		switch node.Kind() {
		case "function_definition":
			if !freeScope(node) {
				if g.cxxFiles == nil {
					g.cxxFiles = map[string]bool{}
				}
				g.cxxFiles[file] = true
			}
		case "namespace_definition", "class_specifier", "template_declaration", "lambda_expression", "qualified_identifier", "new_expression", "delete_expression":
			if g.cxxFiles == nil {
				g.cxxFiles = map[string]bool{}
			}
			g.cxxFiles[file] = true
		}
		if node.Kind() == "declaration" {
			if name := functionName(node.ChildByFieldName("declarator")); name != nil && freeScope(node) {
				if g.declaredNames == nil {
					g.declaredNames = map[string]bool{}
				}
				g.declaredNames[file+"\x00"+name.Utf8Text(source)] = true
			}
		}
	}
	if node.Kind() == "preproc_def" || node.Kind() == "preproc_function_def" {
		if name := node.ChildByFieldName("name"); name != nil {
			if g.macroNames == nil {
				g.macroNames = map[string]bool{}
			}
			g.macroNames[name.Utf8Text(source)] = true
		}
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
		symbol.fileStatic = node.Kind() == "function_definition" && staticFreeFunction(node, source)
		symbol.conditional = conditionalDefinition(node)
		symbol.invalidDefinition = node.HasError()
		g.Symbols = append(g.Symbols, symbol)
		scope := functionScope{id: symbol.ID, end: node.EndByte()}
		if path.Ext(file) == ".c" && node.Kind() == "function_definition" && !node.HasError() {
			scope.localNames = localBindingNames(node, source)
			scope.validCFunction = true
		}
		scopes = append(scopes, scope)
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
				scope := scopes[len(scopes)-1]
				call.CallerID = scope.id
				call.fileStaticCall = scope.validCFunction && callee.Kind() == "identifier"
				call.localShadow = scope.localNames[call.Callee]
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
	local := make(map[string][]functionSymbol)
	for _, symbol := range g.Symbols {
		if symbol.Kind == "function" && symbol.Name != "" && path.Ext(symbol.Definition.File) == ".c" {
			key := symbol.Definition.File + "\x00" + symbol.Name
			local[key] = append(local[key], symbol)
		}
	}
	for i := range g.Calls {
		call := &g.Calls[i]
		key := call.Site.File + "\x00" + call.Callee
		candidates := local[key]
		hasStatic := false
		for _, candidate := range candidates {
			hasStatic = hasStatic || candidate.fileStatic
		}
		if call.fileStaticCall && hasStatic && !g.cxxFiles[call.Site.File] {
			var boundary string
			switch {
			case call.localShadow:
				boundary = "local-binding-boundary"
			case g.macroNames[call.Callee]:
				boundary = "macro-name-boundary"
			case len(candidates) != 1:
				boundary = "ambiguous-name"
			case candidates[0].invalidDefinition:
				boundary = "unresolved-definition"
			case candidates[0].conditional:
				boundary = "conditional-definition"
			case strings.Contains(candidates[0].Name, "::"):
				boundary = "unresolved"
			}
			if boundary != "" {
				call.Resolution = boundary
				continue
			}
			if !g.declaredNames[key] {
				call.Resolution, call.CandidateID, call.CandidateBasis = "name-candidate", candidates[0].ID, "file-internal-definition"
				continue
			}
		}
		id, exists := byName[call.Callee]
		if id != "" {
			call.Resolution, call.CandidateID = "name-candidate", id
		} else if exists {
			call.Resolution = "ambiguous-name"
		}
	}
}
