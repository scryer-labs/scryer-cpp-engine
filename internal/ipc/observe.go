package ipc

import (
	"context"
	"slices"
	"strconv"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type Location struct {
	File   string `json:"file"`
	Line   uint   `json:"line"`
	Column uint   `json:"column"`
}

type Evidence struct {
	Location
	Operation string `json:"operation"`
	Symbol    string `json:"symbol"`
}

type expression struct {
	kind     string
	value    string
	input    *expression
	location Location
}

type nativeCall struct {
	name     string
	input    *expression
	location Location
}

type qualifiedCall struct {
	symbol    string
	arguments []*expression
	location  Location
}

type function struct {
	name       string
	location   Location
	parameters []string
	fields     map[string]*expression
	result     *expression
	native     []nativeCall
	calls      []qualifiedCall
}

type Collector struct {
	functions []function
	constants map[string]map[string][]*expression
}

func location(file string, node *sitter.Node) Location {
	pos := node.StartPosition()
	return Location{file, pos.Row + 1, pos.Column + 1}
}

func walk(ctx context.Context, node *sitter.Node, visit func(*sitter.Node) bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !visit(node) {
		return nil
	}
	cursor := node.Walk()
	defer cursor.Close()
	if cursor.GotoFirstChild() {
		for {
			child := cursor.Node()
			if child.IsNamed() {
				if err := walk(ctx, child, visit); err != nil {
					return err
				}
			}
			if !cursor.GotoNextSibling() {
				break
			}
		}
	}
	return nil
}

func (c *Collector) Observe(ctx context.Context, root *sitter.Node, file string, source []byte) error {
	var scopes []struct {
		name string
		end  uint
	}
	if c.constants == nil {
		c.constants = map[string]map[string][]*expression{}
	}
	c.constants[file] = map[string][]*expression{}
	var scanError error
	err := walk(ctx, root, func(node *sitter.Node) bool {
		for len(scopes) > 0 && node.StartByte() >= scopes[len(scopes)-1].end {
			scopes = scopes[:len(scopes)-1]
		}
		switch node.Kind() {
		case "namespace_definition", "class_specifier", "struct_specifier":
			if name := node.ChildByFieldName("name"); name != nil {
				scopes = append(scopes, struct {
					name string
					end  uint
				}{name.Utf8Text(source), node.EndByte()})
			}
		case "function_definition":
			parts := make([]string, 0, len(scopes))
			for _, scope := range scopes {
				parts = append(parts, scope.name)
			}
			scanError = c.observeFunction(ctx, node, file, strings.Join(parts, "::"), source)
			return false
		case "declaration":
			c.observeConstant(node, file, source)
		}
		return scanError == nil
	})
	if err != nil {
		return err
	}
	return scanError
}

func (c *Collector) observeConstant(node *sitter.Node, file string, source []byte) {
	constant := false
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "type_qualifier" && slices.Contains([]string{"const", "constexpr"}, child.Utf8Text(source)) {
			constant = true
		}
	}
	if !constant {
		return
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		decl := node.NamedChild(i)
		if decl.Kind() != "init_declarator" {
			continue
		}
		name := decl.ChildByFieldName("declarator")
		if name == nil || name.Kind() != "identifier" {
			continue
		}
		text := name.Utf8Text(source)
		c.constants[file][text] = append(c.constants[file][text], readExpression(decl.ChildByFieldName("value"), file, source))
	}
}

func (c *Collector) observeFunction(ctx context.Context, node *sitter.Node, file, scope string, source []byte) error {
	decl := node.ChildByFieldName("declarator")
	for decl != nil && decl.Kind() != "function_declarator" {
		decl = decl.ChildByFieldName("declarator")
	}
	if decl == nil {
		return nil
	}
	name := decl.ChildByFieldName("declarator")
	params := decl.ChildByFieldName("parameters")
	body := node.ChildByFieldName("body")
	if name == nil || params == nil || body == nil {
		return nil
	}
	symbol := name.Utf8Text(source)
	if scope != "" {
		symbol = scope + "::" + symbol
	}
	f := function{name: symbol, location: location(file, name), fields: map[string]*expression{}}
	for i := uint(0); i < params.NamedChildCount(); i++ {
		arg := params.NamedChild(i).ChildByFieldName("declarator")
		for arg != nil && arg.Kind() != "identifier" {
			child := arg.ChildByFieldName("declarator")
			if child == nil && arg.Kind() == "reference_declarator" {
				child = arg.NamedChild(0)
			}
			arg = child
		}
		var parameter string
		if arg != nil {
			parameter = arg.Utf8Text(source)
		}
		f.parameters = append(f.parameters, parameter)
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		list := node.NamedChild(i)
		if list.Kind() != "field_initializer_list" {
			continue
		}
		for j := uint(0); j < list.NamedChildCount(); j++ {
			init := list.NamedChild(j)
			field, args := init.NamedChild(0), init.NamedChild(1)
			if field != nil && args != nil && args.NamedChildCount() == 1 {
				f.fields[field.Utf8Text(source)] = readExpression(args.NamedChild(0), file, source)
			}
		}
	}
	if body.NamedChildCount() == 1 && body.NamedChild(0).Kind() == "return_statement" {
		f.result = readExpression(body.NamedChild(0).NamedChild(0), file, source)
	}
	err := walk(ctx, node, func(call *sitter.Node) bool {
		if call.Kind() != "call_expression" {
			return true
		}
		callee, args := call.ChildByFieldName("function"), call.ChildByFieldName("arguments")
		if callee == nil || args == nil || args.NamedChildCount() == 0 {
			return true
		}
		if callee.Kind() == "qualified_identifier" {
			qualified := qualifiedCall{symbol: callee.Utf8Text(source), location: location(file, call)}
			for i := uint(0); i < args.NamedChildCount(); i++ {
				arg := args.NamedChild(i)
				if arg.Kind() != "comment" {
					qualified.arguments = append(qualified.arguments, readExpression(arg, file, source))
				}
			}
			f.calls = append(f.calls, qualified)
		}
		api := strings.TrimPrefix(callee.Utf8Text(source), "::")
		switch api {
		case "CreateNamedPipe", "CreateNamedPipeA", "CreateNamedPipeW", "CreateFile", "CreateFileA", "CreateFileW", "WaitNamedPipe", "WaitNamedPipeA", "WaitNamedPipeW":
			f.native = append(f.native, nativeCall{api, readExpression(args.NamedChild(0), file, source), location(file, call)})
		}
		return true
	})
	if err == nil {
		c.functions = append(c.functions, f)
	}
	return err
}

func readExpression(node *sitter.Node, file string, source []byte) *expression {
	if node == nil {
		return nil
	}
	loc := location(file, node)
	switch node.Kind() {
	case "identifier", "field_identifier":
		return &expression{kind: "name", value: node.Utf8Text(source), location: loc}
	case "field_expression":
		object, field := node.ChildByFieldName("argument"), node.ChildByFieldName("field")
		if object != nil && object.Kind() == "this" && field != nil {
			return &expression{kind: "member", value: field.Utf8Text(source), location: loc}
		}
	case "string_literal":
		raw := node.Utf8Text(source)
		if start := strings.IndexByte(raw, '"'); start >= 0 {
			if value, err := strconv.Unquote(raw[start:]); err == nil {
				return &expression{kind: "literal", value: value, location: loc}
			}
		}
	case "raw_string_literal":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child.Kind() == "raw_string_content" {
				return &expression{kind: "literal", value: child.Utf8Text(source), location: loc}
			}
		}
	case "parenthesized_expression":
		return readExpression(node.NamedChild(0), file, source)
	case "call_expression":
		callee, args := node.ChildByFieldName("function"), node.ChildByFieldName("arguments")
		if callee == nil || args == nil {
			return nil
		}
		if callee.Kind() == "field_expression" && args.NamedChildCount() == 0 {
			field := callee.ChildByFieldName("field")
			if field != nil && field.Utf8Text(source) == "c_str" {
				return readExpression(callee.ChildByFieldName("argument"), file, source)
			}
		}
		if args.NamedChildCount() != 1 {
			return nil
		}
		name := callee.Utf8Text(source)
		input := readExpression(args.NamedChild(0), file, source)
		if name == "std::filesystem::path" || name == "std::string" || name == "std::wstring" || name == "std::string_view" || name == "std::wstring_view" {
			return input
		}
		prefix := "std::make_unique<"
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ">") {
			return &expression{kind: "construct", value: strings.TrimSpace(name[len(prefix) : len(name)-1]), input: input, location: loc}
		}
		return &expression{kind: "call", value: name, input: input, location: loc}
	}
	return nil
}
