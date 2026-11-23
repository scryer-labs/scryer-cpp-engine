package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scryer-labs/scryer-cpp-engine/internal/clangscan"
	"github.com/scryer-labs/scryer-cpp-engine/internal/selection"
	"github.com/scryer-labs/scryer-cpp-engine/internal/testclassification"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Impact projects the shared scan evidence. Core owns comparison and graph traversal.
type impactSymbol struct {
	Role          string                      `json:"role,omitempty"`
	TestContext   *testclassification.Context `json:"testContext,omitempty"`
	ExecutionKind string                      `json:"executionKind,omitempty"`
	ID            string                      `json:"id"`
	Key           string                      `json:"key"`
	Signature     string                      `json:"signature"`
	Fingerprint   string                      `json:"fingerprint"`
	Source        string                      `json:"source"`
	Location      sourceRange                 `json:"location"`
}

type impactContext struct {
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

type impactCall struct {
	Kind           string      `json:"kind,omitempty"`
	ID             string      `json:"callSiteId"`
	Caller         string      `json:"callerSymbolId,omitempty"`
	Candidates     []string    `json:"candidateSymbolIds,omitempty"`
	CandidateBasis string      `json:"candidateBasis,omitempty"`
	Candidate      string      `json:"candidateSymbolId,omitempty"`
	Target         string      `json:"targetSymbolId,omitempty"`
	Callee         string      `json:"callee"`
	Resolution     string      `json:"resolution"`
	Site           sourceRange `json:"site"`
}

type impactSnapshot struct {
	Selection  *selection.Receipt `json:"selection,omitempty"`
	Resolution *clangscan.Result  `json:"resolution,omitempty"`
	Contexts   []impactContext    `json:"contexts"`
	Symbols    []impactSymbol     `json:"symbols"`
	Calls      []impactCall       `json:"calls"`
	Notes      []string           `json:"notes"`
}

func tokenText(node *sitter.Node, source []byte, outsideFunctions bool) string {
	var output strings.Builder
	var visit func(*sitter.Node)
	visit = func(current *sitter.Node) {
		if current == nil || current.Kind() == "comment" || outsideFunctions && current.Kind() == "function_definition" && !current.HasError() {
			return
		}
		if current.ChildCount() == 0 {
			fmt.Fprintf(&output, "%d:%s;", len(current.Utf8Text(source)), current.Utf8Text(source))
			return
		}
		for i := uint(0); i < current.ChildCount(); i++ {
			visit(current.Child(i))
		}
	}
	visit(node)
	return output.String()
}

func impactIdentity(node *sitter.Node, file string, source []byte) (string, string, string) {
	if (node.Kind() != "function_definition" && templateVariableName(node) == nil) || node.HasError() {
		return "", "", ""
	}
	declarator := node.ChildByFieldName("declarator")
	if declarator == nil {
		return "", "", ""
	}
	scope := []string{}
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "namespace_definition", "class_specifier", "struct_specifier", "union_specifier":
			name := parent.ChildByFieldName("name")
			if name == nil {
				return "", "", ""
			}
			scope = append([]string{name.Utf8Text(source)}, scope...)
		}
	}
	signature := strings.Join(scope, "::")
	if signature != "" {
		signature += "::"
	}
	identityDeclarator := declarator
	if templateVariableName(node) != nil {
		identityDeclarator = templateVariableName(node)
	}
	signature += identityDeclarator.Utf8Text(source)
	scopeJSON, _ := json.Marshal(scope)
	key := file + "|" + string(scopeJSON) + "|" + tokenText(identityDeclarator, source, false)
	return key, signature, fmt.Sprintf("%x", sha256.Sum256([]byte(tokenText(node, source, false))))
}

func collectImpactSnapshot(ctx context.Context, root string, resolve bool) (impactSnapshot, error) {
	return collectSelectedImpactSnapshot(ctx, root, resolve, nil)
}
func collectSelectedImpactSnapshot(ctx context.Context, root string, resolve bool, selected *selection.Reference) (impactSnapshot, error) {
	request := Request{}
	request.Scan.Repository = root
	request.Scan.FileSelection = selected
	request.Scan.Facts = []string{"call-graph"}
	static := !resolve
	request.Scan.Static = &static
	request.Scan.forImpact = true
	scanned, failure := request.scan(ctx)
	if failure != nil {
		return impactSnapshot{}, fmt.Errorf("%s: %s", failure.Code, failure.Message)
	}
	graph, ok := scanned.Facts["call-graph"].(callGraph)
	if !ok {
		return impactSnapshot{}, fmt.Errorf("scan omitted its call graph")
	}
	if resolve && (graph.Semantic == nil || graph.Semantic.Resolution.Status != "resolved") {
		var resolution *clangscan.Result
		if graph.Semantic != nil {
			resolution = graph.Semantic.Resolution
		}
		detail, _ := json.Marshal(resolution)
		return impactSnapshot{}, fmt.Errorf("compiler scan did not resolve successfully; impact stopped: %s", detail)
	}
	result := impactSnapshot{
		Selection: scanned.Selection, Symbols: []impactSymbol{}, Calls: []impactCall{}, Contexts: scanned.impactContexts,
		Notes: []string{
			"Syntax-only C++ impact. Name candidates are potential links, without compiler name or overload resolution.",
			"Named function definitions and initialized variable-template declarations receive comparison keys. Lambdas, anonymous scopes and parse errors remain outside symbol correspondence.",
			"Header declarations, macros, global state, types, generated code, conditional build selection, virtual dispatch and dynamic calls may affect unchanged functions beyond this graph.",
			"Source paths and declaration tokens identify correspondence within each file. Signature changes and file renames appear as removals and additions.",
		},
	}
	for _, symbol := range graph.Symbols {
		if symbol.impactKey == "" {
			continue
		}
		result.Symbols = append(result.Symbols, impactSymbol{
			Role: symbol.Role, TestContext: symbol.TestContext, ExecutionKind: symbol.ExecutionKind, ID: symbol.ID, Key: symbol.impactKey, Signature: symbol.impactSignature,
			Fingerprint: symbol.impactFingerprint, Source: symbol.impactSource, Location: symbol.Definition,
		})
	}
	if resolve {
		result.Resolution = graph.Semantic.Resolution
		result.Notes[0] = "Compiler-resolved calls and declaration/type references from scan. Variable-template instantiations share a source declaration node; their dependency union is conservative, not a per-instantiation runtime path."
		seenSites := map[string]bool{}
		edges := append(append([]semanticCallEdge{}, graph.Semantic.Edges...), graph.Semantic.References...)
		for _, edge := range edges {
			site := sourceRange{File: edge.Site.File, Start: position{Line: edge.Site.Line, Column: edge.Site.Column}, End: position{Line: edge.Site.Line, Column: edge.Site.Column + 1}}
			result.Calls = append(result.Calls, impactCall{Kind: edge.Kind, ID: edge.ID, Caller: edge.CallerSymbolID, Target: edge.TargetSymbolID, Callee: edge.Name, Resolution: "compiler-" + edge.TargetKind, Site: site})
			if edge.Kind == "call" {
				seenSites[edge.CallSiteID] = true
			}
		}
		for _, call := range graph.Calls {
			if !seenSites[call.ID] {
				result.Calls = append(result.Calls, impactCall{ID: call.ID, Caller: call.CallerID, Callee: call.Callee, Resolution: "unresolved-compiler-target", Site: call.Site})
			}
		}
	} else {
		for _, call := range graph.Calls {
			result.Calls = append(result.Calls, impactCall{ID: call.ID, Caller: call.CallerID, Candidate: call.CandidateID, Candidates: call.CandidateIDs, CandidateBasis: call.CandidateBasis, Callee: call.Callee, Resolution: call.Resolution, Site: call.Site})
		}
	}
	return result, nil
}

func (r Request) collectImpact(ctx context.Context) (any, *protocolError) {
	request := r.Impact
	for _, root := range []string{request.BeforeRoot, request.AfterRoot} {
		if !filepath.IsAbs(root) {
			return nil, &protocolError{"INVALID_REQUEST", "impact requires absolute snapshot directories"}
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return nil, &protocolError{"INVALID_REQUEST", "impact snapshot must be an existing directory"}
		}
	}
	before, err := collectSelectedImpactSnapshot(ctx, request.BeforeRoot, request.Resolve, request.BeforeSelection)
	if err != nil {
		return nil, &protocolError{"CPP_IMPACT_ERROR", "before scan: " + err.Error()}
	}
	after, err := collectSelectedImpactSnapshot(ctx, request.AfterRoot, request.Resolve, request.AfterSelection)
	if err != nil {
		return nil, &protocolError{"CPP_IMPACT_ERROR", "after scan: " + err.Error()}
	}
	return struct {
		SchemaVersion int            `json:"schemaVersion"`
		BeforeSHA     string         `json:"beforeSha"`
		AfterSHA      string         `json:"afterSha"`
		Before        impactSnapshot `json:"before"`
		After         impactSnapshot `json:"after"`
	}{1, request.Before, request.After, before, after}, nil
}
