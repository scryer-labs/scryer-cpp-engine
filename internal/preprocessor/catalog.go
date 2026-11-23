// Package preprocessor records source macro definitions without executing a C
// preprocessor. It supplies bounded forwarding/constant evidence to collectors.
package preprocessor

import (
	"regexp"
	"strconv"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type Location struct {
	File   string `json:"file"`
	Line   uint   `json:"line"`
	Column uint   `json:"column"`
}
type Definition struct {
	Location
	Name        string   `json:"name"`
	Parameters  []string `json:"parameters,omitempty"`
	Replacement string   `json:"replacement"`
	Function    bool     `json:"functionLike"`
}
type Catalog struct{ Definitions []Definition }

func (c *Catalog) Observe(node *sitter.Node, file string, source []byte) {
	if node.Kind() != "preproc_function_def" && node.Kind() != "preproc_def" {
		return
	}
	name, value := node.ChildByFieldName("name"), node.ChildByFieldName("value")
	if name == nil || value == nil {
		return
	}
	start := node.StartPosition()
	d := Definition{Location: Location{file, start.Row + 1, start.Column + 1}, Name: name.Utf8Text(source), Replacement: strings.TrimSpace(strings.ReplaceAll(value.Utf8Text(source), "\\\n", "")), Function: node.Kind() == "preproc_function_def"}
	if params := node.ChildByFieldName("parameters"); params != nil {
		for i := uint(0); i < params.NamedChildCount(); i++ {
			d.Parameters = append(d.Parameters, params.NamedChild(i).Utf8Text(source))
		}
	}
	c.Definitions = append(c.Definitions, d)
}
func (c *Catalog) Find(name string) []Definition {
	var result []Definition
	for _, d := range c.Definitions {
		if d.Name == name {
			result = append(result, d)
		}
	}
	return result
}

var forwarding = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z_0-9:]*)\s*\(`)

func (d Definition) Forward() string {
	if !d.Function {
		return ""
	}
	match := forwarding.FindStringSubmatch(d.Replacement)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

var cString = regexp.MustCompile(`^(?:u8|L|u|U)?("(?:[^"\\]|\\.)*")`)

func (c *Catalog) String(expression string) (string, bool) {
	return c.stringValue(strings.TrimSpace(expression), map[string]bool{})
}
func (c *Catalog) stringValue(expression string, active map[string]bool) (string, bool) {
	if expression == "" {
		return "", false
	}
	var result strings.Builder
	if strings.HasPrefix(expression, "\"") || strings.HasPrefix(expression, "L\"") || strings.HasPrefix(expression, "u8\"") {
		for expression != "" {
			match := cString.FindStringSubmatch(expression)
			if len(match) < 2 {
				return "", false
			}
			value, err := strconv.Unquote(match[1])
			if err != nil {
				return "", false
			}
			result.WriteString(value)
			expression = strings.TrimSpace(expression[len(match[0]):])
		}
		return result.String(), true
	}
	if active[expression] {
		return "", false
	}
	active[expression] = true
	defer delete(active, expression)
	definitions := c.Find(expression)
	if len(definitions) == 0 {
		return "", false
	}
	value := ""
	first := true
	for _, d := range definitions {
		if d.Function {
			return "", false
		}
		item, ok := c.stringValue(d.Replacement, active)
		if !ok || !first && item != value {
			return "", false
		}
		value = item
		first = false
	}
	return value, true
}
