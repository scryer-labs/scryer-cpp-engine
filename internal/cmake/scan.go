// Package cmake extracts CMake declarations without executing the build.
package cmake

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Location struct {
	File    string   `json:"file"`
	Line    int      `json:"line"`
	Context []string `json:"context,omitempty"`
}

type Target struct {
	Location
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Type     string `json:"type,omitempty"`
	AliasOf  string `json:"aliasOf,omitempty"`
	Imported bool   `json:"imported,omitempty"`
}

type Link struct {
	Location
	From          string `json:"from"`
	To            string `json:"to"`
	Visibility    string `json:"visibility,omitempty"`
	Configuration string `json:"configuration,omitempty"`
}

type ExternalDependency struct {
	Location
	Command   string   `json:"command"`
	Name      string   `json:"name"`
	Arguments []string `json:"arguments"`
}

type Subdirectory struct {
	Location
	Path string `json:"path"`
}

type Unresolved struct {
	Location
	Target     string `json:"target,omitempty"`
	Command    string `json:"command"`
	Expression string `json:"expression,omitempty"`
	Reason     string `json:"reason"`
}

type TargetSource struct {
	Location
	Target string
	Path   string
}

type Facts struct {
	Sources              []TargetSource       `json:"-"`
	Targets              []Target             `json:"targets"`
	Links                []Link               `json:"links"`
	ExternalDependencies []ExternalDependency `json:"externalDependencies"`
	Subdirectories       []Subdirectory       `json:"subdirectories"`
	Unresolved           []Unresolved         `json:"unresolved"`
}

func Scan(ctx context.Context, root string) (Facts, error) {
	facts := Facts{
		Targets: []Target{}, Links: []Link{}, ExternalDependencies: []ExternalDependency{},
		Subdirectories: []Subdirectory{}, Unresolved: []Unresolved{},
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path != root && entry.IsDir() {
			switch entry.Name() {
			case ".git", "CMakeFiles", "_deps":
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "CMakeCache.txt")); err == nil {
				return filepath.SkipDir
			}
		}
		if !entry.Type().IsRegular() || (entry.Name() != "CMakeLists.txt" && !strings.HasSuffix(entry.Name(), ".cmake")) {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return facts.scanFile(ctx, filepath.ToSlash(file), strings.TrimPrefix(string(source), "\uFEFF"))
	})
	return facts, err
}

type block struct {
	kind     string
	branches []string
}

func (f *Facts) scanFile(ctx context.Context, file, source string) error {
	reader := lexer{source: source, line: 1}
	var blocks []block
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		cmd, ok, err := reader.next()
		if err != nil {
			location := Location{File: file, Line: max(cmd.line, reader.line)}
			f.Unresolved = append(f.Unresolved, Unresolved{Location: location, Command: cmd.name, Reason: err.Error()})
			return nil
		}
		if !ok {
			return nil
		}
		context := []string{}
		for _, block := range blocks {
			context = append(context, block.branches...)
		}
		location := Location{File: file, Line: cmd.line, Context: context}
		switch cmd.name {
		case "if", "foreach", "while", "function", "macro", "block":
			blocks = append(blocks, block{kind: cmd.name, branches: []string{cmd.expression()}})
			f.unresolved(location, cmd, cmd.expression(), "control flow is not evaluated")
		case "elseif", "else":
			if len(blocks) > 0 && blocks[len(blocks)-1].kind == "if" {
				current := &blocks[len(blocks)-1]
				current.branches = append(current.branches, cmd.expression())
			}
		case "endif", "endforeach", "endwhile", "endfunction", "endmacro", "endblock":
			if len(blocks) > 0 && blocks[len(blocks)-1].kind == strings.TrimPrefix(cmd.name, "end") {
				blocks = blocks[:len(blocks)-1]
			}
		case "add_library", "add_executable":
			f.target(location, cmd)
		case "target_sources":
			f.targetSources(location, cmd)
		case "target_link_libraries":
			f.links(location, cmd)
		case "find_package", "fetchcontent_declare", "externalproject_add":
			f.external(location, cmd)
		case "add_subdirectory":
			if path, ok := f.first(location, cmd); ok {
				f.Subdirectories = append(f.Subdirectories, Subdirectory{Location: location, Path: path})
			}
		}
	}
}

func (cmd invocation) expression() string {
	args := make([]string, 0, len(cmd.args))
	for _, arg := range cmd.args {
		args = append(args, arg.text)
	}
	return fmt.Sprintf("%s(%s)", cmd.name, strings.Join(args, " "))
}

func (f *Facts) unresolved(loc Location, cmd invocation, expression, reason string) {
	f.Unresolved = append(f.Unresolved, Unresolved{Location: loc, Command: cmd.name, Expression: expression, Reason: reason})
}

func (arg argument) literal() bool {
	if arg.bracket {
		return arg.text != "" && !strings.ContainsAny(arg.text, ";\r\n") && !strings.Contains(arg.text, "$<")
	}
	return arg.text != "" && !strings.ContainsAny(arg.text, ";\\\r\n") &&
		!strings.Contains(arg.text, "${") && !strings.Contains(arg.text, "$ENV{") &&
		!strings.Contains(arg.text, "$CACHE{") && !strings.Contains(arg.text, "$<")
}

func (f *Facts) first(loc Location, cmd invocation) (string, bool) {
	if len(cmd.args) == 0 {
		f.unresolved(loc, cmd, "", "missing name or path")
		return "", false
	}
	arg := cmd.args[0]
	if !arg.literal() {
		f.unresolved(loc, cmd, arg.text, "name or path requires evaluation")
		return "", false
	}
	return arg.text, true
}

func (f *Facts) target(loc Location, cmd invocation) {
	name, ok := f.first(loc, cmd)
	if !ok {
		return
	}
	target := Target{Location: loc, Name: name, Kind: strings.TrimPrefix(cmd.name, "add_")}
	if len(cmd.args) > 1 && cmd.args[1].text == "ALIAS" {
		target.Type = "ALIAS"
		if len(cmd.args) == 3 && cmd.args[2].literal() {
			target.AliasOf = cmd.args[2].text
		} else {
			f.unresolved(loc, cmd, cmd.expression(), "alias target requires evaluation")
		}
	} else {
		for i, arg := range cmd.args[1:] {
			switch arg.text {
			case "STATIC", "SHARED", "MODULE", "OBJECT", "INTERFACE", "UNKNOWN":
				if target.Kind == "library" && target.Type == "" {
					target.Type = arg.text
					continue
				}
			case "IMPORTED":
				target.Imported = true
				continue
			case "GLOBAL", "WIN32", "MACOSX_BUNDLE", "EXCLUDE_FROM_ALL":
				continue
			}
			if !target.Imported {
				f.addSources(loc, name, cmd.args[i+1:])
			}
			break
		}
	}
	f.Targets = append(f.Targets, target)
}

func (f *Facts) targetSources(loc Location, cmd invocation) {
	if len(cmd.args) < 2 {
		return
	}
	target, ok := f.first(loc, cmd)
	if !ok {
		return
	}
	// File sets have a different grammar and are not interpreted as ordinary sources.
	for _, arg := range cmd.args[1:] {
		if arg.text == "FILE_SET" {
			return
		}
	}
	f.addSources(loc, target, cmd.args[1:])
}

func (f *Facts) addSources(loc Location, target string, args []argument) {
	// Relative paths in included scripts depend on their invocation context.
	if filepath.Base(loc.File) != "CMakeLists.txt" {
		return
	}
	for _, arg := range args {
		switch arg.text {
		case "PRIVATE", "PUBLIC", "INTERFACE":
			continue
		}
		if arg.literal() && filepath.IsLocal(filepath.FromSlash(arg.text)) {
			loc.Line = arg.line
			f.Sources = append(f.Sources, TargetSource{Location: loc, Target: target, Path: arg.text})
		}
	}
}

func (f *Facts) links(loc Location, cmd invocation) {
	from, ok := f.first(loc, cmd)
	if !ok {
		return
	}
	visibility, configuration := "", ""
	for _, arg := range cmd.args[1:] {
		switch arg.text {
		case "PRIVATE", "PUBLIC", "INTERFACE", "LINK_PRIVATE", "LINK_PUBLIC", "LINK_INTERFACE_LIBRARIES":
			visibility = arg.text
			continue
		case "debug", "optimized", "general":
			configuration = arg.text
			continue
		}
		loc.Line = arg.line
		if !arg.literal() {
			f.Unresolved = append(f.Unresolved, Unresolved{Location: loc, Command: cmd.name, Target: from, Expression: arg.text, Reason: "link item requires evaluation"})
		} else {
			f.Links = append(f.Links, Link{Location: loc, From: from, To: arg.text, Visibility: visibility, Configuration: configuration})
		}
		configuration = ""
	}
}

func (f *Facts) external(loc Location, cmd invocation) {
	name, ok := f.first(loc, cmd)
	if !ok {
		return
	}
	args := make([]string, 0, len(cmd.args)-1)
	for _, arg := range cmd.args[1:] {
		args = append(args, arg.text)
		if arg.text != "" && !arg.literal() {
			argLoc := loc
			argLoc.Line = arg.line
			f.unresolved(argLoc, cmd, arg.text, "external dependency argument requires evaluation")
		}
	}
	f.ExternalDependencies = append(f.ExternalDependencies, ExternalDependency{Location: loc, Command: cmd.name, Name: name, Arguments: args})
}
