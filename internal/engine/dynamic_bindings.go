package engine

import (
	"strconv"

	"github.com/scryer-labs/scryer-cpp-engine/internal/preprocessor"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type dynamicBinding struct {
	CallSiteID         string      `json:"callSiteId"`
	API                string      `json:"api"`
	Site               sourceRange `json:"site"`
	Handle             string      `json:"handle,omitempty"`
	Expression         string      `json:"expression"`
	RequestedName      string      `json:"requestedName,omitempty"`
	Status             string      `json:"status"`
	CandidateSymbolIDs []string    `json:"candidateSymbolIds,omitempty"`
	Limitation         string      `json:"limitation"`
}
type dynamicBindingInput struct {
	dynamicBinding
	nameExpression string
}

func observeDynamicBinding(node *sitter.Node, file string, source []byte) *dynamicBindingInput {
	if node.Kind() != "call_expression" {
		return nil
	}
	callee := node.ChildByFieldName("function")
	if callee == nil {
		return nil
	}
	api := callee.Utf8Text(source)
	if api != "dlsym" && api != "dlopen" && api != "::dlsym" && api != "::dlopen" {
		return nil
	}
	args := node.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() == 0 {
		return nil
	}
	input := &dynamicBindingInput{dynamicBinding: dynamicBinding{CallSiteID: callID(file, node), API: api, Site: location(file, node), Expression: node.Utf8Text(source), Status: "unresolved", Limitation: "Source lookup request only. Loader search order, module identity, symbol interposition and actual invocation are not established."}}
	index := uint(0)
	if api == "dlsym" || api == "::dlsym" {
		if args.NamedChildCount() < 2 {
			return input
		}
		input.Handle = args.NamedChild(0).Utf8Text(source)
		index = 1
	}
	input.nameExpression = args.NamedChild(index).Utf8Text(source)
	return input
}
func callID(file string, node *sitter.Node) string {
	return file + ":" + strconv.FormatUint(uint64(node.StartByte()), 10)
}
func (g *callGraph) linkDynamicBindings(inputs []dynamicBindingInput, macros *preprocessor.Catalog) {
	for _, input := range inputs {
		binding := input.dynamicBinding
		if name, ok := macros.String(input.nameExpression); ok {
			binding.RequestedName = name
			binding.Status = "resolved-name"
			if binding.API == "dlsym" || binding.API == "::dlsym" {
				for _, symbol := range g.Symbols {
					if symbol.Name == name {
						binding.CandidateSymbolIDs = append(binding.CandidateSymbolIDs, symbol.ID)
					}
				}
			}
		}
		g.DynamicBindings = append(g.DynamicBindings, binding)
	}
}
