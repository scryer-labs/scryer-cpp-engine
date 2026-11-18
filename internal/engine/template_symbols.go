package engine

import sitter "github.com/tree-sitter/go-tree-sitter"

// Variable templates are declarations that can own compiler-instantiated
// expressions. Keeping them as graph nodes preserves chains through decltype.
func templateVariableName(node *sitter.Node) *sitter.Node {
	if node == nil || node.Kind() != "declaration" || node.Parent() == nil || node.Parent().Kind() != "template_declaration" {
		return nil
	}
	declarator := node.ChildByFieldName("declarator")
	if declarator == nil || declarator.Kind() != "init_declarator" {
		return nil
	}
	name := declarator.ChildByFieldName("declarator")
	if name == nil || name.Kind() != "identifier" {
		return nil
	}
	return name
}

// constexpr functions may run at runtime. Only the explicit consteval token
// establishes that runtime source-region coverage is inapplicable.
func hasConsteval(node *sitter.Node, source []byte) bool {
	if node == nil || node.Kind() == "compound_statement" {
		return false
	}
	if node.ChildCount() == 0 {
		return node.Utf8Text(source) == "consteval"
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		if hasConsteval(node.Child(i), source) {
			return true
		}
	}
	return false
}
