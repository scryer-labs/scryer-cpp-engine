package ipc

import (
	"slices"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type MappingEntrypoint struct {
	Location
	Kind         string     `json:"kind"`
	Conditions   []string   `json:"conditions,omitempty"`
	ComponentIDs []string   `json:"componentIds,omitempty"`
	CallChain    []Evidence `json:"callChain"`
}

type mappingInvocation struct {
	Location
	symbol     string
	receiver   string
	conditions []string
}

func (c *Collector) observeMappingAlias(node *sitter.Node, file string, source []byte) {
	name, target := node.ChildByFieldName("name"), node.ChildByFieldName("type")
	if name == nil || target == nil {
		return
	}
	text := name.Utf8Text(source)
	c.mappingAliases[file][text] = append(c.mappingAliases[file][text], mappingType(target.Utf8Text(source)))
}

// The invocation trace follows named calls and methods on the result of a
// named factory with an explicit smart-pointer return type.
func readMappingInvocation(call *sitter.Node, file string, source []byte) (mappingInvocation, bool) {
	callee := call.ChildByFieldName("function")
	if callee == nil {
		return mappingInvocation{}, false
	}
	invocation := mappingInvocation{Location: location(file, call)}
	switch callee.Kind() {
	case "identifier", "qualified_identifier":
		invocation.symbol = callee.Utf8Text(source)
	case "field_expression":
		field := callee.ChildByFieldName("field")
		object := callee.ChildByFieldName("argument")
		if field == nil || object == nil || object.Kind() != "call_expression" {
			return mappingInvocation{}, false
		}
		receiver := object.ChildByFieldName("function")
		if receiver == nil || receiver.Kind() != "identifier" {
			return mappingInvocation{}, false
		}
		invocation.symbol = field.Utf8Text(source)
		invocation.receiver = receiver.Utf8Text(source)
	default:
		return mappingInvocation{}, false
	}
	return invocation, true
}

func mappingConditions(call *sitter.Node, source []byte) []string {
	var conditions []string
	for child, parent := call, call.Parent(); parent != nil; child, parent = parent, parent.Parent() {
		if parent.Kind() != "preproc_ifdef" {
			continue
		}
		if alternative := parent.ChildByFieldName("alternative"); alternative != nil && alternative.Id() == child.Id() {
			continue
		}
		directive, name := parent.Child(0), parent.ChildByFieldName("name")
		if directive != nil && name != nil {
			conditions = append(conditions, directive.Utf8Text(source)+" "+name.Utf8Text(source))
		}
	}
	slices.Reverse(conditions)
	return conditions
}

func mappingInitializer(call *sitter.Node) bool {
	parent := call.Parent()
	if parent == nil || parent.Kind() != "init_declarator" {
		return false
	}
	value := parent.ChildByFieldName("value")
	return value != nil && value.Id() == call.Id()
}

func mappingType(text string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(text), "<")
	return strings.TrimSpace(name)
}

func mappingPointee(text string) string {
	for _, prefix := range []string{"std::shared_ptr<", "std::unique_ptr<"} {
		if strings.HasPrefix(text, prefix) && strings.HasSuffix(text, ">") {
			return mappingType(text[len(prefix) : len(text)-1])
		}
	}
	return ""
}

func (a *ipcAnalysis) mappingCallee(invocation mappingInvocation, scope string) (int, bool) {
	symbol := invocation.symbol
	if invocation.receiver != "" {
		i, ok := a.find(scope, invocation.receiver)
		if !ok || a.functions[i].pointee == "" {
			return 0, false
		}
		symbol = a.functions[i].pointee + "::" + symbol
		scope = parentSymbol(a.functions[i].name)
	} else if qualifier, method, ok := strings.Cut(symbol, "::"); ok && qualifier != "" {
		aliases := a.mappingAliases[invocation.File][qualifier]
		switch len(aliases) {
		case 0:
		case 1:
			symbol = aliases[0] + "::" + method
		default:
			return 0, false
		}
	}
	return a.find(scope, symbol)
}

type mappingCaller struct {
	function int
	evidence Evidence
}

type mappingTrace struct {
	callers map[int][]mappingCaller
	roots   map[int][]MappingEntrypoint
}

func (a *ipcAnalysis) indexMappingInvocations() *mappingTrace {
	trace := &mappingTrace{callers: map[int][]mappingCaller{}, roots: map[int][]MappingEntrypoint{}}
	for i, f := range a.functions {
		if f.name == "main" {
			trace.roots[i] = append(trace.roots[i], MappingEntrypoint{Location: f.location, Kind: "main"})
		}
		for _, call := range f.invocations {
			if target, ok := a.mappingCallee(call, parentSymbol(f.name)); ok && target != i {
				evidence := Evidence{call.Location, "call", a.functions[target].name}
				trace.callers[target] = append(trace.callers[target], mappingCaller{i, evidence})
			}
		}
	}
	for _, call := range a.mappingInitializers {
		if target, ok := a.mappingCallee(call, ""); ok {
			evidence := Evidence{call.Location, "call", a.functions[target].name}
			trace.roots[target] = append(trace.roots[target], MappingEntrypoint{
				Location: call.Location, Kind: "initializer", Conditions: call.conditions, CallChain: []Evidence{evidence},
			})
		}
	}
	return trace
}

// Retain one shortest trace per reachable entrypoint.
func (t *mappingTrace) entrypoints(function int) []MappingEntrypoint {
	type visit struct {
		function int
		path     []Evidence
	}
	pending := []visit{{function: function}}
	seen := map[int]bool{function: true}
	var entries []MappingEntrypoint
	for i := 0; i < len(pending); i++ {
		current := pending[i]
		for _, root := range t.roots[current.function] {
			root.CallChain = append(append([]Evidence{}, root.CallChain...), current.path...)
			entries = append(entries, root)
		}
		for _, caller := range t.callers[current.function] {
			if !seen[caller.function] {
				seen[caller.function] = true
				path := append([]Evidence{caller.evidence}, current.path...)
				pending = append(pending, visit{caller.function, path})
			}
		}
	}
	return entries
}
