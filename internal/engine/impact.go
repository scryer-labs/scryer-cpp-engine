package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Impact supplies syntax observations. Core owns comparison and graph traversal.
type impactSymbol struct {
	ID          string      `json:"id"`
	Key         string      `json:"key"`
	Signature   string      `json:"signature"`
	Fingerprint string      `json:"fingerprint"`
	Source      string      `json:"source"`
	Location    sourceRange `json:"location"`
}

type impactContext struct {
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

type impactSnapshot struct {
	Contexts []impactContext `json:"contexts"`
	Symbols  []impactSymbol  `json:"symbols"`
	Calls    []callSite      `json:"calls"`
	Notes    []string        `json:"notes"`
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
	if node.Kind() != "function_definition" || node.HasError() {
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
	signature += declarator.Utf8Text(source)
	scopeJSON, _ := json.Marshal(scope)
	key := file + "|" + string(scopeJSON) + "|" + tokenText(declarator, source, false)
	return key, signature, fmt.Sprintf("%x", sha256.Sum256([]byte(tokenText(node, source, false))))
}

func collectImpactSnapshot(ctx context.Context, root string) (impactSnapshot, error) {
	source, err := scanSources(ctx, root, false, true, false, true)
	if err != nil {
		return impactSnapshot{}, err
	}
	result := impactSnapshot{
		Symbols: []impactSymbol{}, Calls: source.graph.Calls, Contexts: source.impactContexts,
		Notes: []string{
			"Syntax-only C++ impact. Name candidates are potential links, without compiler name or overload resolution.",
			"Only function definitions with an explicit declarator and named enclosing scopes receive comparison keys. Lambdas and parse errors remain outside symbol correspondence.",
			"Header declarations, macros, global state, types, generated code, conditional build selection, virtual dispatch and dynamic calls may affect unchanged functions beyond this graph.",
			"Source paths and declaration tokens identify correspondence within each file. Signature changes and file renames appear as removals and additions.",
		},
	}
	for _, symbol := range source.graph.Symbols {
		if symbol.impactKey == "" {
			continue
		}
		result.Symbols = append(result.Symbols, impactSymbol{
			ID: symbol.ID, Key: symbol.impactKey, Signature: symbol.impactSignature,
			Fingerprint: symbol.impactFingerprint, Source: symbol.impactSource, Location: symbol.Definition,
		})
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
	before, err := collectImpactSnapshot(ctx, request.BeforeRoot)
	if err != nil {
		return nil, &protocolError{"CPP_IMPACT_ERROR", err.Error()}
	}
	after, err := collectImpactSnapshot(ctx, request.AfterRoot)
	if err != nil {
		return nil, &protocolError{"CPP_IMPACT_ERROR", err.Error()}
	}
	return struct {
		SchemaVersion int            `json:"schemaVersion"`
		BeforeSHA     string         `json:"beforeSha"`
		AfterSHA      string         `json:"afterSha"`
		Before        impactSnapshot `json:"before"`
		After         impactSnapshot `json:"after"`
	}{1, request.Before, request.After, before, after}, nil
}
