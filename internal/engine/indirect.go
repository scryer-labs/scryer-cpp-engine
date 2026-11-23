package engine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/scryer-labs/scryer-cpp-engine/internal/preprocessor"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type structField struct {
	Name, Type string
	Function   bool
}
type rawBinding struct {
	Type, Field, Expression string
	Site                    sourceRange
	Basis                   string
}
type indirectModel struct {
	Structs   map[string][]structField
	Conflicts map[string]bool
	Params    map[string]map[string]string
	Bindings  []rawBinding
	Arguments []rawArgument
}
type rawArgument struct {
	CallID, Expression string
	Index              int
	Site               sourceRange
}
type symbolReference struct {
	ID                 string      `json:"referenceId"`
	Kind               string      `json:"kind"`
	Expression         string      `json:"expression"`
	Site               sourceRange `json:"site"`
	StructType         string      `json:"structType,omitempty"`
	Field              string      `json:"field,omitempty"`
	CallSiteID         string      `json:"callSiteId,omitempty"`
	CandidateSymbolIDs []string    `json:"candidateSymbolIds"`
}
type macroExpansion struct {
	CallSiteID         string                `json:"callSiteId"`
	Macro              string                `json:"macro"`
	Definition         preprocessor.Location `json:"definition"`
	ForwardedCallee    string                `json:"forwardedCallee,omitempty"`
	CandidateSymbolIDs []string              `json:"candidateSymbolIds"`
}

func declaratorName(node *sitter.Node, source []byte) string {
	for node != nil {
		if node.Kind() == "identifier" || node.Kind() == "field_identifier" {
			return node.Utf8Text(source)
		}
		next := node.ChildByFieldName("declarator")
		if next == nil && node.NamedChildCount() > 0 {
			next = node.NamedChild(0)
		}
		node = next
	}
	return ""
}
func typeName(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	if node.Kind() == "struct_specifier" || node.Kind() == "class_specifier" {
		if name := node.ChildByFieldName("name"); name != nil {
			return name.Utf8Text(source)
		}
		return ""
	}
	if node.Kind() == "type_identifier" || node.Kind() == "qualified_identifier" {
		return node.Utf8Text(source)
	}
	return ""
}
func (m *indirectModel) observe(node *sitter.Node, file string, source []byte, scopes []functionScope) {
	if m.Structs == nil {
		m.Structs = map[string][]structField{}
		m.Conflicts = map[string]bool{}
		m.Params = map[string]map[string]string{}
	}
	if node.Kind() == "struct_specifier" {
		name, body := node.ChildByFieldName("name"), node.ChildByFieldName("body")
		if name != nil && body != nil {
			var fields []structField
			for i := uint(0); i < body.NamedChildCount(); i++ {
				field := body.NamedChild(i)
				if field.Kind() != "field_declaration" {
					continue
				}
				decl := field.ChildByFieldName("declarator")
				if decl == nil {
					continue
				}
				text := strings.ReplaceAll(decl.Utf8Text(source), " ", "")
				fields = append(fields, structField{Name: declaratorName(decl, source), Type: typeName(field.ChildByFieldName("type"), source), Function: strings.Contains(text, "(*") || strings.Contains(text, "(&")})
			}
			key := name.Utf8Text(source)
			if previous, exists := m.Structs[key]; exists && !slices.Equal(previous, fields) {
				m.Conflicts[key] = true
			} else {
				m.Structs[key] = fields
			}
		}
	}
	if node.Kind() == "function_definition" {
		id := file + ":" + strconv.FormatUint(uint64(node.StartByte()), 10)
		m.Params[id] = map[string]string{}
		decl := node.ChildByFieldName("declarator")
		for decl != nil && decl.Kind() != "function_declarator" {
			decl = decl.ChildByFieldName("declarator")
		}
		if decl != nil {
			if args := decl.ChildByFieldName("parameters"); args != nil {
				for i := uint(0); i < args.NamedChildCount(); i++ {
					p := args.NamedChild(i)
					m.Params[id][declaratorName(p.ChildByFieldName("declarator"), source)] = typeName(p.ChildByFieldName("type"), source)
				}
			}
		}
	}
	if node.Kind() == "declaration" && len(scopes) > 0 {
		id := scopes[len(scopes)-1].id
		if m.Params[id] == nil {
			m.Params[id] = map[string]string{}
		}
		m.Params[id][declaratorName(node.ChildByFieldName("declarator"), source)] = typeName(node.ChildByFieldName("type"), source)
	}
	if node.Kind() == "call_expression" {
		if args := node.ChildByFieldName("arguments"); args != nil {
			for i := uint(0); i < args.NamedChildCount(); i++ {
				arg := args.NamedChild(i)
				m.Arguments = append(m.Arguments, rawArgument{CallID: file + ":" + strconv.FormatUint(uint64(node.StartByte()), 10) + "-" + strconv.FormatUint(uint64(node.EndByte()), 10), Expression: arg.Utf8Text(source), Index: int(i), Site: location(file, arg)})
			}
		}
	}
	if node.Kind() == "initializer_list" {
		declaration := node.Parent()
		for declaration != nil && declaration.Kind() != "declaration" {
			declaration = declaration.Parent()
		}
		if declaration == nil {
			return
		}
		kind := typeName(declaration.ChildByFieldName("type"), source)
		if kind == "" {
			return
		}
		// Store positional slots now; structs can be declared in a later file.
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child.Kind() == "initializer_list" {
				continue
			}
			field := "#" + strconv.Itoa(int(i))
			expression := child.Utf8Text(source)
			if child.Kind() == "initializer_pair" {
				if designator := child.ChildByFieldName("designator"); designator != nil {
					field = strings.TrimPrefix(designator.Utf8Text(source), ".")
				}
				if value := child.ChildByFieldName("value"); value != nil {
					expression = value.Utf8Text(source)
				}
			}
			m.Bindings = append(m.Bindings, rawBinding{Type: kind, Field: field, Expression: expression, Site: location(file, child), Basis: "initializer-function-reference"})
		}
	}
}
func simpleSymbol(text string) string {
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "&"))
	for _, r := range text {
		if !(r == '_' || r == ':' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return ""
		}
	}
	return text
}
func splitInitializer(text string) []string {
	var parts []string
	start, depth := 0, 0
	for i, r := range text {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
	}
	return append(parts, strings.TrimSpace(text[start:]))
}
func (g *callGraph) linkIndirect(model *indirectModel, macros *preprocessor.Catalog) {
	byName := map[string][]functionSymbol{}
	for _, symbol := range g.Symbols {
		if symbol.Kind == "function" && symbol.Name != "" {
			byName[symbol.Name] = append(byName[symbol.Name], symbol)
		}
	}
	candidates := func(name, file string) []string {
		matches := byName[simpleSymbol(name)]
		var result, local []string
		for _, symbol := range matches {
			result = append(result, symbol.ID)
			if symbol.Definition.File == file {
				local = append(local, symbol.ID)
			}
		}
		if len(local) > 0 {
			result = local
		}
		slices.Sort(result)
		return result
	}
	fields := map[string][]string{}
	appendBinding := func(binding rawBinding) {
		if model.Conflicts[binding.Type] {
			return
		}
		schema := model.Structs[binding.Type]
		slot := binding.Field
		if strings.HasPrefix(slot, "#") {
			index, err := strconv.Atoi(slot[1:])
			if err != nil || index >= len(schema) {
				return
			}
			slot = schema[index].Name
		}
		functional := false
		for _, field := range schema {
			if field.Name == slot {
				functional = field.Function
			}
		}
		if !functional {
			return
		}
		ids := candidates(binding.Expression, binding.Site.File)
		reference := symbolReference{ID: fmt.Sprintf("%s:%d:%d:%s:%s", binding.Site.File, binding.Site.Start.Line, binding.Site.Start.Column, binding.Type, slot), Kind: binding.Basis, Expression: binding.Expression, Site: binding.Site, StructType: binding.Type, Field: slot, CandidateSymbolIDs: ids}
		if ids == nil {
			reference.CandidateSymbolIDs = []string{}
		}
		g.SymbolReferences = append(g.SymbolReferences, reference)
		key := binding.Type + "." + slot
		for _, id := range ids {
			if !slices.Contains(fields[key], id) {
				fields[key] = append(fields[key], id)
			}
		}
	}
	for _, binding := range model.Bindings {
		appendBinding(binding)
	}
	argsByCall := map[string][]rawArgument{}
	for _, arg := range model.Arguments {
		argsByCall[arg.CallID] = append(argsByCall[arg.CallID], arg)
		if ids := candidates(arg.Expression, arg.Site.File); len(ids) > 0 {
			g.SymbolReferences = append(g.SymbolReferences, symbolReference{ID: arg.CallID + ":arg:" + strconv.Itoa(arg.Index), Kind: "argument-function-reference", Expression: arg.Expression, Site: arg.Site, CallSiteID: arg.CallID, CandidateSymbolIDs: ids})
		}
	}
	for i := range g.Calls {
		call := &g.Calls[i]
		for _, definition := range macros.Find(call.Callee) {
			if !definition.Function {
				continue
			}
			forwarded := definition.Forward()
			ids := candidates(forwarded, call.Site.File)
			g.MacroExpansions = append(g.MacroExpansions, macroExpansion{CallSiteID: call.ID, Macro: definition.Name, Definition: definition.Location, ForwardedCallee: forwarded, CandidateSymbolIDs: ids})
			if len(ids) > 0 {
				call.CandidateBasis = "macro-forwarding"
				call.Resolution = "macro-forwarding-candidate"
				for _, id := range ids {
					if !slices.Contains(call.CandidateIDs, id) {
						call.CandidateIDs = append(call.CandidateIDs, id)
					}
				}
			}
			// A macro may construct an aggregate with function-valued parameters. Record
			// those assignments without assigning a business meaning to the macro name.
			replacement := definition.Replacement
			structAt := strings.Index(replacement, "struct ")
			open := strings.IndexByte(replacement, '{')
			close := strings.LastIndexByte(replacement, '}')
			if structAt < 0 || open < 0 || close <= open {
				continue
			}
			tokens := strings.Fields(replacement[structAt+7:])
			if len(tokens) == 0 {
				continue
			}
			kind := tokens[0]
			for slot, value := range splitInitializer(replacement[open+1 : close]) {
				for index, param := range definition.Parameters {
					if value != param {
						continue
					}
					for _, arg := range argsByCall[call.ID] {
						if arg.Index == index {
							appendBinding(rawBinding{Type: kind, Field: "#" + strconv.Itoa(slot), Expression: arg.Expression, Site: arg.Site, Basis: "macro-initializer-function-reference"})
						}
					}
				}
			}
		}
	}
	for i := range g.Calls {
		call := &g.Calls[i]
		spelling := strings.ReplaceAll(call.Callee, "->", ".")
		parts := strings.Split(spelling, ".")
		if len(parts) < 2 {
			continue
		}
		kind := model.Params[call.CallerID][parts[0]]
		for index, fieldName := range parts[1:] {
			if kind == "" || model.Conflicts[kind] {
				break
			}
			var field *structField
			for _, candidate := range model.Structs[kind] {
				if candidate.Name == fieldName {
					copy := candidate
					field = &copy
					break
				}
			}
			if field == nil {
				break
			}
			if index == len(parts)-2 && field.Function {
				ids := fields[kind+"."+field.Name]
				if len(ids) > 0 {
					call.CandidateBasis = "struct-field-initializer"
					call.Resolution = "field-initializer-candidate"
					call.CandidateIDs = append([]string{}, ids...)
					slices.Sort(call.CandidateIDs)
				}
			}
			kind = field.Type
		}
	}
}
