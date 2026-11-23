// Package posix records bounded native API evidence without executing sources.
package posix

import (
	"slices"
	"strings"

	"github.com/scryer-labs/scryer-cpp-engine/internal/preprocessor"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type Location struct {
	File   string `json:"file"`
	Line   uint   `json:"line"`
	Column uint   `json:"column"`
}
type Trace struct {
	Location
	Operation string `json:"operation"`
	Symbol    string `json:"symbol"`
}
type Entrypoint struct {
	Location
	Kind         string   `json:"kind,omitempty"`
	CallChain    []Trace  `json:"callChain,omitempty"`
	ComponentIDs []string `json:"componentIds,omitempty"`
}
type Observation struct {
	FunctionLocation *Location `json:"functionLocation,omitempty"`
	Transport        string    `json:"transport,omitempty"`
	Location
	API         string       `json:"api"`
	Function    string       `json:"function,omitempty"`
	Expression  string       `json:"expression"`
	Role        string       `json:"role,omitempty"`
	Name        string       `json:"name,omitempty"`
	Status      string       `json:"status"`
	Reason      string       `json:"reason,omitempty"`
	AccessFlags string       `json:"accessFlags,omitempty"`
	Entrypoints []Entrypoint `json:"entrypoints,omitempty"`
}
type Mechanism struct {
	Endpoints    []Observation `json:"endpoints"`
	Observations []Observation `json:"observations"`
}
type Facts struct {
	UnixSocket   Mechanism `json:"unixSocket"`
	SharedMemory Mechanism `json:"sharedMemory"`
	Limitations  []string  `json:"limitations"`
}
type raw struct {
	Observation
	args          []string
	key, variable string
}
type Collector struct {
	calls   []raw
	values  map[string][]string
	sockets map[string][]string
	memory  map[string][]string
}

func text(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	return node.Utf8Text(source)
}
func compact(value string) string { return strings.Join(strings.Fields(value), "") }
func (c *Collector) Observe(node *sitter.Node, file string, source []byte) {
	if c.values == nil {
		c.values = map[string][]string{}
		c.sockets = map[string][]string{}
		c.memory = map[string][]string{}
	}
	function := ""
	var functionLocation *Location
	for p := node.Parent(); p != nil; p = p.Parent() {
		if p.Kind() == "lambda_expression" {
			break
		}
		if p.Kind() == "function_definition" {
			decl := p.ChildByFieldName("declarator")
			function = text(decl, source)
			for decl != nil && decl.Kind() != "function_declarator" {
				decl = decl.ChildByFieldName("declarator")
			}
			if decl != nil {
				if name := decl.ChildByFieldName("declarator"); name != nil {
					point := name.StartPosition()
					functionLocation = &Location{file, point.Row + 1, point.Column + 1}
				}
			}
			break
		}
	}
	key := file + "|" + function + "|"
	addValue := func(name, value string) { c.values[key+compact(name)] = append(c.values[key+compact(name)], value) }
	if node.Kind() == "init_declarator" {
		addValue(text(node.ChildByFieldName("declarator"), source), text(node.ChildByFieldName("value"), source))
	}
	if node.Kind() == "assignment_expression" {
		addValue(text(node.ChildByFieldName("left"), source), text(node.ChildByFieldName("right"), source))
	}
	if node.Kind() != "call_expression" {
		return
	}
	name := strings.TrimPrefix(text(node.ChildByFieldName("function"), source), "::")
	argsNode := node.ChildByFieldName("arguments")
	if argsNode == nil {
		return
	}
	var args []string
	for i := uint(0); i < argsNode.NamedChildCount(); i++ {
		args = append(args, text(argsNode.NamedChild(i), source))
	}
	if (name == "strcpy" || name == "strncpy") && len(args) >= 2 {
		index := 1
		if len(args) > index {
			addValue(args[0], args[index])
		}
	}
	switch name {
	case "socket", "bind", "connect", "sendto", "recvfrom", "shm_open", "shm_unlink", "mmap", "munmap":
	default:
		return
	}
	pos := node.StartPosition()
	call := raw{Observation: Observation{Location: Location{file, pos.Row + 1, pos.Column + 1}, API: name, Function: function, FunctionLocation: functionLocation, Expression: text(node, source)}, args: args, key: key}
	if parent := node.Parent(); parent != nil && parent.Kind() == "init_declarator" {
		call.variable = compact(text(parent.ChildByFieldName("declarator"), source))
	}
	if parent := node.Parent(); parent != nil && parent.Kind() == "assignment_expression" {
		call.variable = compact(text(parent.ChildByFieldName("left"), source))
	}
	if name == "socket" && len(args) >= 2 && call.variable != "" {
		c.sockets[key+call.variable] = append(c.sockets[key+call.variable], compact(args[0])+"|"+compact(args[1]))
	}
	if name == "shm_open" && len(args) > 0 && call.variable != "" {
		c.memory[key+call.variable] = append(c.memory[key+call.variable], args[0])
	}
	c.calls = append(c.calls, call)
}
func unique(values []string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	for _, value := range values {
		if value != values[0] {
			return "", false
		}
	}
	return values[0], true
}
func (c *Collector) Facts(macros *preprocessor.Catalog) *Facts {
	result := &Facts{UnixSocket: Mechanism{Endpoints: []Observation{}, Observations: []Observation{}}, SharedMemory: Mechanism{Endpoints: []Observation{}, Observations: []Observation{}}, Limitations: []string{"Native API spelling is source evidence, not compiler binding. Socket resources require AF_UNIX/AF_LOCAL and unambiguous local sockaddr assignments. Cross-function aliasing, descriptor reuse, runtime formatting and global array data flow are not resolved. Endpoint pairing is potential resource compatibility, not observed traffic."}}
	for _, raw := range c.calls {
		observation := raw.Observation
		observation.Status = "unresolved"
		observation.Reason = "resource or native socket domain not resolved in bounded source analysis"
		mechanism := &result.UnixSocket
		arg := func(index int) string {
			if index < len(raw.args) {
				return raw.args[index]
			}
			return ""
		}
		resolve := func(expression string) (string, bool) {
			if value, ok := macros.String(expression); ok {
				return value, true
			}
			value, ok := unique(c.values[raw.key+compact(expression)])
			if ok {
				return macros.String(value)
			}
			return "", false
		}
		name, ok := "", false
		switch raw.API {
		case "shm_open", "shm_unlink":
			mechanism = &result.SharedMemory
			name, ok = resolve(arg(0))
			observation.Role = "open"
			if raw.API == "shm_unlink" {
				observation.Role = "unlink"
			}
			observation.AccessFlags = arg(1)
			if slices.Contains(strings.Split(compact(arg(1)), "|"), "O_CREAT") {
				observation.Role = "create"
			}
		case "mmap", "munmap":
			mechanism = &result.SharedMemory
			observation.Role = raw.API
			if expression, found := unique(c.memory[raw.key+compact(arg(4))]); found {
				name, ok = resolve(expression)
			}
		case "bind", "connect", "sendto":
			descriptor, found := unique(c.sockets[raw.key+compact(arg(0))])
			domain := strings.Split(descriptor, "|")[0]
			if found && (domain == "AF_UNIX" || domain == "AF_LOCAL") {
				address := arg(1)
				if raw.API == "sendto" {
					address = arg(4)
				}
				// Casts do not change the address variable's source identity.
				if offset := strings.LastIndex(address, "&"); offset >= 0 {
					address = strings.TrimSpace(address[offset+1:])
					address = strings.TrimRight(address, " )")
				}
				name, ok = resolve(compact(address) + ".sun_path")
				parts := strings.Split(descriptor, "|")
				if slices.Contains(parts, "SOCK_DGRAM") {
					observation.Transport = "datagram"
				} else if slices.Contains(parts, "SOCK_STREAM") {
					observation.Transport = "stream"
				}
			}
			observation.Role = "send"
			if raw.API == "connect" {
				observation.Role = "connect"
			}
			if raw.API == "bind" {
				observation.Role = "bind"
			}
		case "recvfrom":
			observation.Role = "receive"
		case "socket":
			observation.Role = "socket"
			observation.Reason = "socket creation alone does not identify a resource"
		}
		if ok && name != "" {
			observation.Name = name
			observation.Status = "resolved-resource"
			observation.Reason = "literal or unambiguous source assignment; runtime resource existence is not established"
		}
		if strings.HasPrefix(raw.Function, "main(") || strings.HasPrefix(raw.Function, "wmain(") {
			observation.Entrypoints = []Entrypoint{{Location: raw.Location}}
		}
		mechanism.Observations = append(mechanism.Observations, observation)
		if observation.Status == "resolved-resource" && (observation.Role == "bind" || observation.Role == "send" || observation.Role == "create" || observation.Role == "open") {
			mechanism.Endpoints = append(mechanism.Endpoints, observation)
		}
	}
	return result
}
