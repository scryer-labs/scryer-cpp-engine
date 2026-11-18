// Package testclassification identifies narrowly proven test scopes from
// compiler evidence. It does not infer purpose from filenames or test execution.
package testclassification

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed registrations.json
var registrationRules []byte

type registrationRule struct {
	Framework        string `json:"framework"`
	SymbolUSRPrefix  string `json:"symbolUsrPrefix"`
	DefinitionSuffix string `json:"definitionSuffix"`
}

var rules = loadRules()

func loadRules() []registrationRule {
	var result []registrationRule
	if err := json.Unmarshal(registrationRules, &result); err != nil {
		panic(err)
	}
	return result
}

type Evidence struct {
	Kind           string `json:"kind"`
	Framework      string `json:"framework"`
	SymbolUSR      string `json:"symbolUsr"`
	DefinitionFile string `json:"definitionFile"`
	File           string `json:"file"`
	Line           uint   `json:"line"`
	Column         uint   `json:"column"`
}

type Context struct {
	Scope    string     `json:"scope"`
	Evidence []Evidence `json:"evidence"`
}

// Registration requires compiler binding to a known registration API and its
// framework declaration. A matching spelling alone supplies no classification.
func Registration(usr, definition, file string, line, column uint) *Context {
	for _, rule := range rules {
		if strings.HasPrefix(usr, rule.SymbolUSRPrefix) && strings.HasSuffix(strings.ReplaceAll(definition, "\\", "/"), rule.DefinitionSuffix) {
			return &Context{Scope: "test-only", Evidence: []Evidence{{
				Kind: "compiler-test-registration", Framework: rule.Framework,
				SymbolUSR: usr, DefinitionFile: definition, File: file, Line: line, Column: column,
			}}}
		}
	}
	return nil
}
