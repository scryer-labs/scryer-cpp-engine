package ipc

import (
	"path"
	"slices"
	"strings"
)

type Endpoint struct {
	Role       string     `json:"role"`
	Name       string     `json:"name"`
	SourceRole string     `json:"sourceRole"`
	Evidence   []Evidence `json:"evidence"`
}

type ParameterEvidence struct {
	Symbol         string     `json:"symbol"`
	ParameterIndex int        `json:"parameterIndex"`
	Role           string     `json:"role"`
	SourceRole     string     `json:"sourceRole"`
	Evidence       []Evidence `json:"evidence"`
}

type Unresolved struct {
	Role     string     `json:"role"`
	Evidence []Evidence `json:"evidence"`
}

type namedPipe struct {
	Endpoints  []Endpoint          `json:"endpoints"`
	Parameters []ParameterEvidence `json:"parameterEvidence"`
	Unresolved []Unresolved        `json:"unresolved"`
}

type Facts struct {
	Basis   string `json:"basis"`
	Windows struct {
		NamedPipe namedPipe `json:"namedPipe"`
	} `json:"windows"`
}

type namedPipeAnalysis struct {
	*Collector
	byName                map[string][]int
	output                *namedPipe
	constructorParameters map[int][]ParameterEvidence
}

type valueTrace struct {
	value *expression
	steps []Evidence
}

func (c *Collector) Facts() Facts {
	facts := Facts{Basis: "static-source"}
	facts.Windows.NamedPipe = namedPipe{Endpoints: []Endpoint{}, Parameters: []ParameterEvidence{}, Unresolved: []Unresolved{}}
	a := namedPipeAnalysis{Collector: c, byName: map[string][]int{}, output: &facts.Windows.NamedPipe, constructorParameters: map[int][]ParameterEvidence{}}
	for i, f := range c.functions {
		a.byName[f.name] = append(a.byName[f.name], i)
	}
	for _, f := range c.functions {
		for _, call := range f.native {
			a.native(f, call)
		}
	}
	for _, f := range c.functions {
		a.factory(f)
	}
	return facts
}

func (a *namedPipeAnalysis) find(scope, symbol string) (int, bool) {
	if strings.HasPrefix(symbol, "::") {
		matches := a.byName[strings.TrimPrefix(symbol, "::")]
		if len(matches) == 1 {
			return matches[0], true
		}
		return 0, false
	}
	for {
		name := symbol
		if scope != "" {
			name = scope + "::" + symbol
		}
		if matches := a.byName[name]; len(matches) > 0 {
			if len(matches) == 1 {
				return matches[0], true
			}
			return 0, false
		}
		if scope == "" {
			return 0, false
		}
		scope = parentSymbol(scope)
	}
}

func parentSymbol(name string) string {
	if index := strings.LastIndex(name, "::"); index >= 0 {
		return name[:index]
	}
	return ""
}

func leafSymbol(name string) string {
	if index := strings.LastIndex(name, "::"); index >= 0 {
		return name[index+2:]
	}
	return name
}

func (a *namedPipeAnalysis) trace(f function, input *expression, visiting map[int]bool) valueTrace {
	if input == nil || input.kind != "call" {
		return valueTrace{value: input}
	}
	i, ok := a.find(parentSymbol(f.name), input.value)
	if !ok || visiting[i] {
		return valueTrace{}
	}
	wrapper := a.functions[i]
	if len(wrapper.parameters) != 1 || wrapper.parameters[0] == "" {
		return valueTrace{}
	}
	visiting[i] = true
	returned := a.trace(wrapper, wrapper.result, visiting)
	delete(visiting, i)
	if returned.value == nil || returned.value.kind != "name" || returned.value.value != wrapper.parameters[0] {
		return valueTrace{}
	}
	actual := a.trace(f, input.input, visiting)
	actual.steps = append([]Evidence{{input.location, "name-forwarding-call", wrapper.name}, {wrapper.location, "name-forwarding-definition", wrapper.name}}, append(returned.steps, actual.steps...)...)
	return actual
}

func (a *namedPipeAnalysis) native(f function, call nativeCall) {
	if strings.HasPrefix(call.name, "WaitNamedPipe") {
		return
	}
	traced := a.trace(f, call.input, map[int]bool{})
	role := "server"
	var confirmation *Evidence
	if strings.HasPrefix(call.name, "CreateFile") {
		role = "client"
		// CreateFile also opens ordinary files and is used by some servers
		// to wake their own listener. Require pipe-specific evidence here.
		known, evidence := a.pipeClient(f, traced.value)
		if !known {
			return
		}
		confirmation = evidence
	}
	steps := traced.steps
	if confirmation != nil {
		steps = append(steps, *confirmation)
	}
	steps = append(steps, Evidence{call.location, "native-api", call.name})
	if literal := a.literal(f, traced.value); literal != nil && pipeName(literal.value) {
		evidence := append([]Evidence{{literal.location, "endpoint-name", literal.value}}, steps...)
		a.output.Endpoints = append(a.output.Endpoints, Endpoint{role, literal.value, sourceRole(f.location.File), evidence})
		return
	}
	if traced.value != nil {
		name := traced.value.value
		if index := parameterIndex(f, traced.value); index >= 0 {
			a.addParameter(f, index, role, steps)
			return
		}
		if traced.value.kind == "name" || traced.value.kind == "member" {
			class := parentSymbol(f.name)
			constructors := a.byName[class+"::"+leafSymbol(class)]
			if len(constructors) == 1 {
				i := constructors[0]
				constructor := a.functions[i]
				value := a.trace(constructor, constructor.fields[name], map[int]bool{})
				if index := parameterIndex(constructor, value.value); index >= 0 {
					evidence := append([]Evidence{{value.value.location, "member-initializer", name}}, value.steps...)
					evidence = append(evidence, steps...)
					record := a.addParameter(constructor, index, role, evidence)
					a.constructorParameters[i] = append(a.constructorParameters[i], record)
					return
				}
			}
		}
	}
	a.output.Unresolved = append(a.output.Unresolved, Unresolved{role, steps})
}

func (a *namedPipeAnalysis) pipeClient(f function, input *expression) (bool, *Evidence) {
	if literal := a.literal(f, input); literal != nil && pipeName(literal.value) {
		return true, nil
	}
	if input == nil || (input.kind != "name" && input.kind != "member") {
		return false, nil
	}
	for _, other := range f.native {
		if !strings.HasPrefix(other.name, "WaitNamedPipe") {
			continue
		}
		wait := a.trace(f, other.input, map[int]bool{}).value
		if wait != nil && wait.kind == input.kind && wait.value == input.value {
			return true, &Evidence{other.location, "pipe-name-confirmation", other.name}
		}
	}
	return false, nil
}

func (a *namedPipeAnalysis) literal(f function, input *expression) *expression {
	if input == nil {
		return nil
	}
	if input.kind == "literal" {
		return input
	}
	if input.kind == "name" && parameterIndex(f, input) < 0 {
		matches := a.constants[f.location.File][input.value]
		if len(matches) == 1 && matches[0] != nil && matches[0].kind == "literal" {
			return matches[0]
		}
	}
	return nil
}

func parameterIndex(f function, input *expression) int {
	if input == nil || input.kind != "name" || input.value == "" {
		return -1
	}
	return slices.Index(f.parameters, input.value)
}

func pipeName(value string) bool {
	host, name, ok := strings.Cut(strings.TrimPrefix(strings.ToLower(value), `\\`), `\pipe\`)
	return strings.HasPrefix(value, `\\`) && ok && host != "" && name != ""
}

func sourceRole(file string) string {
	for _, part := range strings.Split(path.Clean(file), "/") {
		if part == "tests" || part == "test" {
			return "test"
		}
	}
	return "source"
}

func (a *namedPipeAnalysis) addParameter(f function, index int, role string, steps []Evidence) ParameterEvidence {
	record := ParameterEvidence{f.name, index, role, sourceRole(f.location.File), steps}
	a.output.Parameters = append(a.output.Parameters, record)
	return record
}

func (a *namedPipeAnalysis) factory(f function) {
	if f.result == nil || f.result.kind != "construct" {
		return
	}
	target := f.result.value
	i, ok := a.find(parentSymbol(f.name), target+"::"+leafSymbol(target))
	if !ok {
		return
	}
	actual := a.trace(f, f.result.input, map[int]bool{})
	for _, record := range a.constructorParameters[i] {
		if record.ParameterIndex != 0 {
			continue
		}
		steps := append([]Evidence{{f.result.location, "factory-construction", record.Symbol}}, actual.steps...)
		steps = append(steps, record.Evidence...)
		if index := parameterIndex(f, actual.value); index >= 0 {
			a.addParameter(f, index, record.Role, steps)
		}
	}
}
