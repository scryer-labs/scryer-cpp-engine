package cmake

import (
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type value struct {
	text        string
	known       bool
	parentWrite bool
}
type environment map[string]value

func assign(env environment, name string, binding value) {
	binding.parentWrite = binding.parentWrite || env[name].parentWrite
	env[name] = binding
}

func cloneEnvironment(env environment) environment {
	result := environment{}
	for name, v := range env {
		result[name] = v
	}
	return result
}
func replaceEnvironment(destination, source environment) environment {
	for name := range destination {
		delete(destination, name)
	}
	for name, v := range source {
		destination[name] = v
	}
	return destination
}
func mergeEnvironments(alternatives []environment) environment {
	result := environment{}
	for _, env := range alternatives {
		for name := range env {
			result[name] = value{}
		}
	}
	for name := range result {
		first, ok := alternatives[0][name]
		same := ok && first.known
		parentWrite := first.parentWrite
		for _, env := range alternatives[1:] {
			v, ok := env[name]
			same = same && ok && v.known && v.text == first.text
			parentWrite = parentWrite || v.parentWrite
		}
		result[name] = value{text: first.text, known: same, parentWrite: parentWrite}
	}
	return result
}

// Known values are substituted once. set() already expands values at assignment
// time; rescanning substituted bracket text would incorrectly expand literals.
func expand(text string, env, builtins environment) (string, bool) {
	if strings.Contains(text, "$<") || strings.Contains(text, "$ENV{") || strings.Contains(text, "$CACHE{") || strings.Contains(text, "\\") {
		return text, false
	}
	var result strings.Builder
	for {
		index := strings.Index(text, "${")
		if index < 0 {
			result.WriteString(text)
			return result.String(), true
		}
		result.WriteString(text[:index])
		text = text[index+2:]
		end := strings.IndexByte(text, '}')
		if end < 0 {
			return "", false
		}
		name := text[:end]
		if strings.ContainsAny(name, "${") {
			return "", false
		}
		v, ok := builtins[name]
		if !ok {
			v, ok = env[name]
		}
		if !ok || !v.known {
			return "", false
		}
		result.WriteString(v.text)
		text = text[end+1:]
	}
}
func expandArgument(arg argument, env, builtins environment) (argument, bool) {
	if arg.bracket {
		return arg, true
	}
	text, ok := expand(arg.text, env, builtins)
	if !ok {
		return arg, false
	}
	arg.text = text
	return arg, true
}
func expandInvocation(cmd invocation, env, builtins environment) invocation {
	result := cmd
	result.args = nil
	splitLists := cmd.name == "add_library" || cmd.name == "add_executable" || cmd.name == "target_sources" || cmd.name == "target_link_libraries" || cmd.name == "target_include_directories"
	for _, arg := range cmd.args {
		expanded, ok := expandArgument(arg, env, builtins)
		if !ok {
			result.args = append(result.args, arg)
			continue
		}
		if !expanded.bracket && (!expanded.quoted || splitLists) {
			for _, part := range strings.Split(expanded.text, ";") {
				if part != "" {
					item := expanded
					item.text = part
					result.args = append(result.args, item)
				}
			}
		} else {
			result.args = append(result.args, expanded)
		}
	}
	return result
}
func variableName(cmd invocation) (string, bool) {
	if len(cmd.args) == 0 || !cmd.args[0].literal() || strings.ContainsAny(cmd.args[0].text, "{}$") {
		return "", false
	}
	return cmd.args[0].text, true
}
func (m *model) binding(loc Location, cmd invocation, env, builtins environment) {
	name, ok := variableName(cmd)
	if !ok {
		return
	}
	switch cmd.name {
	case "set":
		parent := false
		unsupported := false
		for _, arg := range cmd.args[1:] {
			parent = parent || arg.text == "PARENT_SCOPE"
			unsupported = unsupported || arg.text == "CACHE"
		}
		if parent || unsupported {
			assign(env, name, value{parentWrite: parent})
			return
		}
		if len(cmd.args) == 1 {
			assign(env, name, value{})
			return
		} // unset may expose an unknown cache value
		values := []string{}
		for _, arg := range cmd.args[1:] {
			expanded, ok := expandArgument(arg, env, builtins)
			if !ok {
				assign(env, name, value{})
				return
			}
			if !arg.quoted && !arg.bracket && expanded.text == "" {
				continue
			}
			values = append(values, expanded.text)
		}
		assign(env, name, value{text: strings.Join(values, ";"), known: true, parentWrite: env[name].parentWrite})
	case "unset":
		parent := len(cmd.args) > 1 && cmd.args[1].text == "PARENT_SCOPE"
		assign(env, name, value{parentWrite: parent})
	case "project":
		resolved, ok := expandArgument(cmd.args[0], env, builtins)
		if !ok || !resolved.literal() {
			env["PROJECT_NAME"] = value{}
			return
		}
		env["PROJECT_NAME"] = value{text: resolved.text, known: true}
		env["PROJECT_SOURCE_DIR"] = builtins["CMAKE_CURRENT_SOURCE_DIR"]
		env[resolved.text+"_SOURCE_DIR"] = builtins["CMAKE_CURRENT_SOURCE_DIR"]
		if loc.SourceDirectory == "." {
			env["CMAKE_PROJECT_NAME"] = env["PROJECT_NAME"]
		}
	case "aux_source_directory":
		if len(cmd.args) != 2 || !cmd.args[1].literal() {
			return
		}
		name = cmd.args[1].text
		directory, ok := expandArgument(cmd.args[0], env, builtins)
		if !ok {
			assign(env, name, value{})
			m.facts.unresolved(loc, cmd, cmd.args[0].text, "source directory requires evaluation")
			return
		}
		relative, ok := m.localPath(loc.SourceDirectory, directory.text)
		if !ok {
			assign(env, name, value{})
			m.facts.unresolved(loc, cmd, directory.text, "source directory is outside the selected project")
			return
		}
		var files []string
		for _, file := range m.byDirectory[relative] {
			// Keep CMake's source-directory-relative spelling where possible.
			source, _ := filepath.Rel(filepath.Join(m.root, filepath.FromSlash(loc.SourceDirectory)), filepath.Join(m.root, filepath.FromSlash(file)))
			files = append(files, filepath.ToSlash(source))
		}
		sort.Strings(files)
		assign(env, name, value{text: strings.Join(files, ";"), known: true})
	case "list":
		if len(cmd.args) < 2 || !cmd.args[1].literal() {
			return
		}
		name = cmd.args[1].text
		current, ok := env[name]
		if !ok || !current.known {
			assign(env, name, value{})
			return
		}
		parts := []string{}
		if current.text != "" {
			parts = strings.Split(current.text, ";")
		}
		expanded := expandInvocation(invocation{name: "target_sources", args: cmd.args[2:]}, env, builtins)
		additions := []string{}
		for _, arg := range expanded.args {
			if !arg.literal() {
				assign(env, name, value{})
				return
			}
			additions = append(additions, arg.text)
		}
		switch cmd.args[0].text {
		case "APPEND":
			parts = append(parts, additions...)
		case "PREPEND":
			parts = append(additions, parts...)
		default:
			assign(env, name, value{})
			return
		}
		assign(env, name, value{text: strings.Join(parts, ";"), known: true, parentWrite: env[name].parentWrite})
	}
}
func compilableSource(file string) bool {
	switch strings.ToLower(path.Ext(file)) {
	case ".c", ".cc", ".cpp", ".cxx":
		return true
	}
	return false
}
