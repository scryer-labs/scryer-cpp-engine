package engine

import sitter "github.com/tree-sitter/go-tree-sitter"

// A storage specifier is structural evidence, independent of spelling order or
// trivia. A class static method is not a file-internal free function.
func staticFreeFunction(node *sitter.Node, source []byte) bool {
	if node.Kind() != "function_definition" && node.Kind() != "declaration" {
		return false
	}
	if !freeScope(node) {
		return false
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "storage_class_specifier" && child.Utf8Text(source) == "static" {
			return true
		}
	}
	return false
}

func conditionalDefinition(node *sitter.Node) bool {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "preproc_if", "preproc_ifdef", "preproc_else", "preproc_elif", "preproc_elifdef":
			return true
		}
	}
	return false
}

// This deliberately over-approximates function-local shadowing. It does not
// claim block-order lookup or infer a function-valued variable's target.
func localBindingNames(node *sitter.Node, source []byte) map[string]bool {
	result := map[string]bool{}
	declare := func(current *sitter.Node) {
		for i := uint(0); i < current.NamedChildCount(); i++ {
			if current.FieldNameForNamedChild(uint32(i)) == "declarator" {
				if name := localBindingName(current.NamedChild(i), source); name != "" {
					result[name] = true
				}
			}
		}
	}
	declarator := node.ChildByFieldName("declarator")
	for declarator != nil && declarator.Kind() != "function_declarator" {
		declarator = declarator.ChildByFieldName("declarator")
	}
	if declarator != nil {
		if params := declarator.ChildByFieldName("parameters"); params != nil {
			for i := uint(0); i < params.NamedChildCount(); i++ {
				declare(params.NamedChild(i))
			}
		}
	}
	var visit func(*sitter.Node)
	visit = func(current *sitter.Node) {
		if current.Kind() == "declaration" || current.Kind() == "type_definition" {
			declare(current)
		}
		for i := uint(0); i < current.NamedChildCount(); i++ {
			child := current.NamedChild(i)
			switch child.Kind() {
			case "function_definition", "lambda_expression", "parameter_list", "struct_specifier", "class_specifier", "union_specifier":
				continue
			}
			visit(child)
		}
	}
	if body := node.ChildByFieldName("body"); body != nil {
		visit(body)
	}
	return result
}

func freeScope(node *sitter.Node) bool {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "class_specifier", "struct_specifier", "union_specifier", "function_definition", "lambda_expression":
			return false
		}
	}
	return true
}

// Typedef names are type_identifier nodes, unlike variable declarators. Keep this
// local to lexical-boundary collection; indirect callback/type joins use their
// own declarator semantics.
func localBindingName(node *sitter.Node, source []byte) string {
	for node != nil {
		switch node.Kind() {
		case "identifier", "field_identifier", "type_identifier":
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
