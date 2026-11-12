package ipc

import (
	"strconv"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type mappingCall struct {
	api      string
	action   string
	name     *expression
	flags    string
	handle   string
	backing  string
	location Location
}

type SharedMemory struct {
	Mappings   []SharedMapping      `json:"mappings"`
	Views      []MappingObservation `json:"views"`
	Unresolved []MappingObservation `json:"unresolved"`
}

type SharedMapping struct {
	Name       string               `json:"name"`
	Operations []MappingObservation `json:"operations"`
}

type MappingObservation struct {
	ID string `json:"id"`
	Location
	API                     string       `json:"api"`
	Operation               string       `json:"operation"`
	Symbol                  string       `json:"symbol"`
	SourceRole              string       `json:"sourceRole"`
	AccessMode              string       `json:"accessMode"`
	AccessFlags             string       `json:"accessFlags,omitempty"`
	Handle                  string       `json:"handle,omitempty"`
	Backing                 string       `json:"backing,omitempty"`
	NameLocation            *Location    `json:"nameLocation,omitempty"`
	HandleSourceCandidateID string       `json:"handleSourceCandidateId,omitempty"`
	Entrypoints             []Entrypoint `json:"entrypoints,omitempty"`
}

func readMappingCall(api string, call, arguments *sitter.Node, file string, source []byte) (mappingCall, bool) {
	var action string
	var nameIndex, flagIndex uint
	switch api {
	case "CreateFileMapping", "CreateFileMappingA", "CreateFileMappingW":
		action, nameIndex, flagIndex = "create", 5, 2
	case "OpenFileMapping", "OpenFileMappingA", "OpenFileMappingW":
		action, nameIndex, flagIndex = "open", 2, 0
	case "MapViewOfFile", "MapViewOfFileEx":
		action, flagIndex = "map", 1
	default:
		return mappingCall{}, false
	}

	var args []*sitter.Node
	for i := uint(0); i < arguments.NamedChildCount(); i++ {
		if arg := arguments.NamedChild(i); arg.Kind() != "comment" {
			args = append(args, arg)
		}
	}
	result := mappingCall{api: api, action: action, location: location(file, call)}
	if flagIndex < uint(len(args)) {
		result.flags = args[flagIndex].Utf8Text(source)
	}
	if action == "map" {
		if len(args) > 0 {
			result.handle = mappingHandle(args[0], source)
		}
		return result, true
	}
	if nameIndex < uint(len(args)) {
		result.name = readExpression(args[nameIndex], file, source)
	}
	if action == "create" && len(args) > 0 && args[0].Utf8Text(source) == "INVALID_HANDLE_VALUE" {
		result.backing = "pagefile"
	}
	if parent := call.Parent(); parent != nil {
		switch parent.Kind() {
		case "init_declarator":
			result.handle = mappingHandle(parent.ChildByFieldName("declarator"), source)
		case "assignment_expression":
			operator := parent.ChildByFieldName("operator")
			if operator != nil && operator.Kind() == "=" {
				result.handle = mappingHandle(parent.ChildByFieldName("left"), source)
			}
		}
	}
	return result, true
}

func mappingHandle(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "identifier", "field_identifier":
		return node.Utf8Text(source)
	case "field_expression":
		object, field := node.ChildByFieldName("argument"), node.ChildByFieldName("field")
		if object != nil && object.Kind() == "this" && field != nil {
			return field.Utf8Text(source)
		}
	}
	return ""
}

func mappingAccess(action, flags string) string {
	switch flags {
	case "PAGE_READONLY", "PAGE_EXECUTE_READ", "PAGE_WRITECOPY", "PAGE_EXECUTE_WRITECOPY":
		if action == "create" {
			return "read"
		}
	case "PAGE_READWRITE", "PAGE_EXECUTE_READWRITE":
		if action == "create" {
			return "read-write"
		}
	case "FILE_MAP_READ":
		if action != "create" {
			return "read"
		}
	case "FILE_MAP_WRITE", "FILE_MAP_ALL_ACCESS":
		if action != "create" {
			return "read-write"
		}
	case "FILE_MAP_COPY":
		if action == "open" {
			return "read"
		}
		if action == "map" {
			return "copy-on-write"
		}
	}
	return "unknown"
}

func (a *ipcAnalysis) sharedMemory(entrypoints func(int) []Entrypoint) *SharedMemory {
	result := &SharedMemory{Mappings: []SharedMapping{}, Views: []MappingObservation{}, Unresolved: []MappingObservation{}}
	mappings := map[string]int{}
	for i, f := range a.functions {
		if len(f.mappings) == 0 {
			continue
		}
		roots := entrypoints(i)
		handles := map[string][]string{}
		for _, call := range f.mappings {
			obs := MappingObservation{
				ID:       call.location.File + ":" + strconv.FormatUint(uint64(call.location.Line), 10) + ":" + strconv.FormatUint(uint64(call.location.Column), 10),
				Location: call.location, API: call.api, Operation: call.action, Symbol: f.name,
				SourceRole: sourceRole(f.location.File), AccessFlags: call.flags, AccessMode: mappingAccess(call.action, call.flags),
				Handle: call.handle, Backing: call.backing, Entrypoints: roots,
			}
			if call.action == "map" {
				if sources := handles[call.handle]; len(sources) == 1 && call.handle != "" {
					obs.HandleSourceCandidateID = sources[0]
				}
				result.Views = append(result.Views, obs)
				continue
			}
			if call.handle != "" {
				handles[call.handle] = append(handles[call.handle], obs.ID)
			}
			traced := a.trace(f, call.name, map[int]bool{})
			name := a.literal(f, traced.value)
			if name == nil || strings.TrimSpace(name.value) == "" {
				result.Unresolved = append(result.Unresolved, obs)
				continue
			}
			obs.NameLocation = &name.location
			index, ok := mappings[name.value]
			if !ok {
				index = len(result.Mappings)
				mappings[name.value] = index
				result.Mappings = append(result.Mappings, SharedMapping{Name: name.value})
			}
			result.Mappings[index].Operations = append(result.Mappings[index].Operations, obs)
		}
	}
	if len(result.Mappings)+len(result.Views)+len(result.Unresolved) == 0 {
		return nil
	}
	return result
}
