package engine

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type collectionCompleteness struct {
	Status               string   `json:"status"`
	ParseRecoveryFiles   []string `json:"parseRecoveryFiles"`
	MacroBodyProjections int      `json:"macroBodyProjections"`
	CallsWithoutCaller   int      `json:"callsWithoutCaller"`
	Limitations          []string `json:"limitations"`
}

// A missing-semicolon expression followed by a top-level block is retained as
// a macro-body source scope. It is not proof of macro expansion or a test role.
func (g *callGraph) macroBody(node *sitter.Node, file string, source []byte, scopes []functionScope) []functionScope {
	if len(scopes) > 0 || node.Kind() != "compound_statement" {
		return scopes
	}
	previous := node.PrevNamedSibling()
	for previous != nil && previous.Kind() == "comment" {
		previous = previous.PrevNamedSibling()
	}
	if previous == nil || previous.Kind() != "expression_statement" || !previous.HasError() || strings.HasSuffix(strings.TrimSpace(previous.Utf8Text(source)), ";") {
		return scopes
	}
	call := previous.NamedChild(0)
	if call == nil || call.Kind() != "call_expression" || call.HasError() {
		return scopes
	}
	name := call.ChildByFieldName("function")
	if name == nil || name.Kind() != "identifier" {
		return scopes
	}
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "compound_statement", "function_definition", "lambda_expression":
			return scopes
		}
	}
	definition := location(file, node)
	definition.Start = location(file, call).Start
	spelling := call.Utf8Text(source)
	symbol := functionSymbol{ID: file + ":" + strconv.FormatUint(uint64(call.StartByte()), 10), Kind: "macro-body", Name: spelling, Definition: definition, Projection: "syntactic-adjacent-block"}
	if g.withImpact {
		symbol.impactKey = file + "|macro-body|" + tokenText(call, source, false)
		symbol.impactSignature = spelling
		symbol.impactFingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(tokenText(node, source, false))))
		symbol.impactSource = string(source[call.StartByte():node.EndByte()])
	}
	g.Symbols = append(g.Symbols, symbol)
	return append(scopes, functionScope{id: symbol.ID, end: node.EndByte()})
}
func (g *callGraph) recordCompleteness(recovery []string) {
	result := &collectionCompleteness{Status: "bounded", ParseRecoveryFiles: recovery, Limitations: []string{"Syntax candidates are not compiler bindings or runtime paths. Missing callers can also be valid global initializers."}}
	if result.ParseRecoveryFiles == nil {
		result.ParseRecoveryFiles = []string{}
	}
	for _, symbol := range g.Symbols {
		if symbol.Kind == "macro-body" {
			result.MacroBodyProjections++
		}
	}
	for _, call := range g.Calls {
		if call.CallerID == "" {
			result.CallsWithoutCaller++
		}
	}
	if len(recovery) > 0 || result.MacroBodyProjections > 0 {
		result.Status = "partial"
		result.Limitations = append(result.Limitations, "Recovered macro bodies provide source grouping, not a proven expanded function or test identity.")
	}
	g.Completeness = result
}
