package cmake

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// model interprets a deliberately bounded declaration subset. It never runs
// CMake, expands host environment/cache values, or invokes project commands.
type model struct {
	root        string
	byDirectory map[string][]string
	commands    map[string]bool
	sources     map[string]string
	facts       *Facts
	visited     map[string]bool
	active      map[string]bool
	steps       int
}

func newModel(root string, files []string, sources map[string]string, facts *Facts) *model {
	index := map[string][]string{}
	for _, file := range files {
		if compilableSource(file) {
			index[path.Dir(file)] = append(index[path.Dir(file)], file)
		}
	}
	return &model{root: root, byDirectory: index, commands: map[string]bool{}, sources: sources, facts: facts, visited: map[string]bool{}, active: map[string]bool{}}
}
func emptyFacts() Facts {
	return Facts{Targets: []Target{}, Links: []Link{}, ExternalDependencies: []ExternalDependency{}, Subdirectories: []Subdirectory{}, Unresolved: []Unresolved{}}
}

func Scan(ctx context.Context, root string) (Facts, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() && name != root {
			switch entry.Name() {
			case ".git", "CMakeFiles", "_deps":
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(name, "CMakeCache.txt")); err == nil {
				return filepath.SkipDir
			}
		}
		if entry.Type().IsRegular() {
			relative, err := filepath.Rel(root, name)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return Facts{}, err
	}
	return ScanFiles(ctx, root, files)
}

// ScanFiles consumes one explicit inventory for both build files and source-list
// enumeration. CMake collection has no dependency on the C++ parser or Core YAML.
func ScanFiles(ctx context.Context, root string, files []string) (Facts, error) {
	facts := emptyFacts()
	directory, err := os.OpenRoot(root)
	if err != nil {
		return facts, err
	}
	defer directory.Close()
	sources := map[string]string{}
	selected := append([]string{}, files...)
	sort.Strings(selected)
	for _, name := range selected {
		if err = ctx.Err(); err != nil {
			return facts, err
		}
		if path.Base(name) != "CMakeLists.txt" && !strings.HasSuffix(name, ".cmake") {
			continue
		}
		file, err := directory.Open(filepath.FromSlash(name))
		if err != nil {
			return facts, err
		}
		data, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			return facts, readErr
		}
		if closeErr != nil {
			return facts, closeErr
		}
		sources[name] = strings.TrimPrefix(string(data), "\uFEFF")
	}
	model := newModel(root, selected, sources, &facts)
	if _, ok := sources["CMakeLists.txt"]; ok {
		if err = model.scanFile(ctx, "CMakeLists.txt", ".", environment{}, nil); err != nil {
			return facts, err
		}
	}
	// Keep declaration discovery from disconnected/selected fragments. These have
	// no inferred parent variables and are explicitly marked as outside traversal.
	for _, name := range selected {
		if _, ok := sources[name]; !ok || model.visited[name] {
			continue
		}
		facts.Unresolved = append(facts.Unresolved, Unresolved{Location: Location{File: name, Line: 1}, Command: "listfile", Reason: "listfile not reached through selected add_subdirectory/include declarations; parent scope is unknown"})
		if err = model.scanFile(ctx, name, path.Dir(name), environment{}, nil); err != nil {
			return facts, err
		}
	}
	return facts, nil
}

type frame struct {
	kind         string
	branches     []string
	initial      environment
	parent       environment
	alternatives []environment
	hasElse      bool
}

func (m *model) scanFile(ctx context.Context, file, sourceDir string, env environment, inherited []string) error {
	if m.active[file] || len(m.active) >= 64 {
		m.facts.Unresolved = append(m.facts.Unresolved, Unresolved{Location: Location{File: file, Line: 1}, Command: "listfile", Reason: "recursive or excessive listfile traversal"})
		return nil
	}
	m.active[file], m.visited[file] = true, true
	defer delete(m.active, file)
	reader := lexer{source: m.sources[file], line: 1}
	var blocks []frame
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		command, ok, err := reader.next()
		if err != nil {
			m.facts.Unresolved = append(m.facts.Unresolved, Unresolved{Location: Location{File: file, Line: max(command.line, reader.line)}, Command: command.name, Reason: err.Error()})
			return nil
		}
		if !ok {
			return nil
		}
		m.steps++
		if m.steps > 1_000_000 {
			return fmt.Errorf("CMake declaration traversal exceeds one million commands")
		}
		contexts := append([]string{}, inherited...)
		for _, block := range blocks {
			contexts = append(contexts, block.branches...)
		}
		loc := Location{File: file, Line: command.line, Context: contexts, SourceDirectory: sourceDir}
		switch command.name {
		case "if", "foreach", "while", "function", "macro", "block":
			if (command.name == "function" || command.name == "macro") && len(command.args) > 0 {
				m.commands[strings.ToLower(command.args[0].text)] = true
			}
			blocks = append(blocks, frame{kind: command.name, branches: []string{command.expression()}, initial: cloneEnvironment(env), parent: env})
			env = cloneEnvironment(env)
			if command.name != "if" && command.name != "block" {
				for key := range env {
					env[key] = value{}
				}
			}
			m.facts.unresolved(loc, command, command.expression(), "control flow is not evaluated")
			continue
		case "elseif", "else":
			if len(blocks) > 0 && blocks[len(blocks)-1].kind == "if" {
				top := &blocks[len(blocks)-1]
				top.alternatives = append(top.alternatives, cloneEnvironment(env))
				env = cloneEnvironment(top.initial)
				top.branches = append(top.branches, command.expression())
				top.hasElse = command.name == "else"
			}
			continue
		case "endif", "endforeach", "endwhile", "endfunction", "endmacro", "endblock":
			if len(blocks) > 0 && blocks[len(blocks)-1].kind == strings.TrimPrefix(command.name, "end") {
				top := blocks[len(blocks)-1]
				blocks = blocks[:len(blocks)-1]
				if top.kind == "function" || top.kind == "macro" {
					env = top.parent
				} else {
					top.alternatives = append(top.alternatives, cloneEnvironment(env))
					if top.kind != "if" || !top.hasElse {
						top.alternatives = append(top.alternatives, top.initial)
					}
					merged := mergeEnvironments(top.alternatives)
					env = replaceEnvironment(top.parent, merged)
				}
			}
			continue
		}
		if m.commands[command.name] {
			for key := range env {
				env[key] = value{parentWrite: env[key].parentWrite}
			}
			m.facts.unresolved(loc, command, command.expression(), "custom CMake commands are not executed; variable effects are unknown")
			continue
		}
		// Builtins depend on invocation context; they must never leak from a child
		// listfile back to its caller's current-list/current-source directory.
		builtins := environment{
			"CMAKE_SOURCE_DIR":         {text: filepath.ToSlash(m.root), known: true},
			"CMAKE_CURRENT_SOURCE_DIR": {text: filepath.ToSlash(filepath.Join(m.root, filepath.FromSlash(sourceDir))), known: true},
			"CMAKE_CURRENT_LIST_DIR":   {text: filepath.ToSlash(filepath.Join(m.root, filepath.FromSlash(path.Dir(file)))), known: true},
			"CMAKE_CURRENT_LIST_FILE":  {text: filepath.ToSlash(filepath.Join(m.root, filepath.FromSlash(file))), known: true},
		}

		if (command.name == "set" || command.name == "set_tests_properties" || command.name == "add_custom_target" || command.name == "add_test") && strings.Contains(command.expression(), "LD_PRELOAD") {
			m.facts.RuntimeBindings = append(m.facts.RuntimeBindings, RuntimeBinding{Location: loc, Command: command.name, Expression: command.expression(), Status: "declared-environment"})
		}
		switch command.name {
		case "file":
			if len(command.args) > 1 && (command.args[0].text == "GLOB" || command.args[0].text == "GLOB_RECURSE") {
				env[command.args[1].text] = value{}
				m.facts.unresolved(loc, command, command.expression(), "file glob evaluation is not supported")
			}
			continue
		case "set", "unset", "list", "project", "aux_source_directory":
			m.binding(loc, command, env, builtins)
			continue
		}
		if command.name == "include" || command.name == "add_subdirectory" {
			if err = m.traverse(ctx, loc, command, env, builtins); err != nil {
				return err
			}
			continue
		}
		switch command.name {
		case "add_library", "add_executable", "target_sources", "target_link_libraries", "target_include_directories", "find_package", "fetchcontent_declare", "externalproject_add":
			expanded := expandInvocation(command, env, builtins)
			switch command.name {
			case "add_library", "add_executable":
				m.target(loc, expanded)
			case "target_sources":
				m.targetSources(loc, expanded)
			case "target_link_libraries":
				m.facts.links(loc, expanded)
			case "target_include_directories":
				m.facts.includeDirectories(loc, expanded)
				for _, arg := range expanded.args[1:] {
					if !arg.literal() && !strings.HasPrefix(arg.text, "$<BUILD_INTERFACE:") {
						argLoc := loc
						argLoc.Line = arg.line
						m.facts.unresolved(argLoc, command, arg.text, "include directory requires evaluation")
					}
				}
			default:
				m.facts.external(loc, expanded)
			}
		}
	}
}

func (m *model) traverse(ctx context.Context, loc Location, cmd invocation, env, builtins environment) error {
	expanded := expandInvocation(cmd, env, builtins)
	name, ok := m.facts.first(loc, expanded)
	if !ok {
		return nil
	}
	sourceDir := loc.SourceDirectory
	if cmd.name == "add_subdirectory" {
		m.facts.Subdirectories = append(m.facts.Subdirectories, Subdirectory{Location: loc, Path: name})
		directory, ok := m.localPath(sourceDir, name)
		if !ok {
			m.facts.unresolved(loc, cmd, name, "subdirectory is outside the selected project")
			return nil
		}
		name = path.Join(directory, "CMakeLists.txt")
		sourceDir = directory
	} else {
		// System modules remain external declarations: no lookup into host CMake or
		// implicit module-path/cache evaluation is performed.
		if !strings.Contains(name, "/") && !strings.HasSuffix(name, ".cmake") {
			return nil
		}
		resolved, ok := m.localPath(sourceDir, name)
		if !ok {
			m.facts.unresolved(loc, cmd, name, "include is outside the selected project")
			return nil
		}
		name = resolved
	}
	if _, ok := m.sources[name]; !ok {
		m.facts.unresolved(loc, cmd, name, "selected listfile is unavailable; inherited declarations were not collected")
		return nil
	}
	parent := env
	if cmd.name == "add_subdirectory" {
		env = cloneEnvironment(env)
	}
	err := m.scanFile(ctx, name, sourceDir, env, loc.Context)
	if cmd.name == "add_subdirectory" {
		for key, v := range env {
			if v.parentWrite {
				parent[key] = value{parentWrite: parent[key].parentWrite}
			}
		}
	}
	return err
}

func (m *model) localPath(directory, value string) (string, bool) {
	absolute := filepath.FromSlash(value)
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(m.root, filepath.FromSlash(directory), absolute)
	}
	relative, err := filepath.Rel(m.root, filepath.Clean(absolute))
	if err != nil || !filepath.IsLocal(relative) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}
