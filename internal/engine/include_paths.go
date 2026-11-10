package engine

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/scryer-labs/scryer-cpp-engine/internal/cmake"
)

type includePath struct {
	cmake.IncludeDirectory
	Directory string `json:"directory"`
}

type includePaths struct {
	targetByID  map[string]string
	targets     map[string][]cmake.Target
	directories map[string][]includePath
	links       map[string][]cmake.Link
	cache       map[string][]includePath
}

func indexIncludePaths(root string, build cmake.Facts) *includePaths {
	index := &includePaths{
		targetByID: map[string]string{}, targets: map[string][]cmake.Target{},
		directories: map[string][]includePath{}, links: map[string][]cmake.Link{}, cache: map[string][]includePath{},
	}
	for _, target := range build.Targets {
		index.targetByID[componentID(target)] = target.Name
		index.targets[target.Name] = append(index.targets[target.Name], target)
	}
	for _, link := range build.Links {
		index.links[link.From] = append(index.links[link.From], link)
	}
	for _, declaration := range build.IncludeDirectories {
		if directory, ok := sourceIncludePath(root, declaration); ok {
			index.directories[declaration.Target] = append(index.directories[declaration.Target], includePath{IncludeDirectory: declaration, Directory: directory})
		}
	}
	return index
}

func sourceIncludePath(root string, declaration cmake.IncludeDirectory) (string, bool) {
	// An included script can run from a different CMake source directory.
	if path.Base(declaration.File) != "CMakeLists.txt" {
		return "", false
	}
	value := declaration.Expression
	buildExpression := strings.HasPrefix(value, "$<BUILD_INTERFACE:")
	if buildExpression {
		if !strings.HasSuffix(value, ">") {
			return "", false
		}
		value = strings.TrimSuffix(strings.TrimPrefix(value, "$<BUILD_INTERFACE:"), ">")
	}
	sourceDir := filepath.Join(root, filepath.FromSlash(path.Dir(declaration.File)))
	const variable = "${CMAKE_CURRENT_SOURCE_DIR}"
	if value == variable || strings.HasPrefix(value, variable+"/") {
		value = filepath.ToSlash(sourceDir) + strings.TrimPrefix(value, variable)
	}
	if value == "" || strings.ContainsAny(value, "$;\\<>\r\n") {
		return "", false
	}
	absolute := filepath.FromSlash(value)
	if !filepath.IsAbs(absolute) {
		if buildExpression {
			return "", false
		}
		absolute = filepath.Join(sourceDir, absolute)
	}
	relative, err := filepath.Rel(root, filepath.Clean(absolute))
	if err != nil || !filepath.IsLocal(relative) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func (index *includePaths) forComponent(id string) []includePath {
	if paths, ok := index.cache[id]; ok {
		return paths
	}
	root := index.targetByID[id]
	if len(index.targets[root]) != 1 {
		return nil
	}
	var paths []includePath
	seen := map[string]bool{root: true}
	var visitInterface func(string)
	visitInterface = func(name string) {
		targets := index.targets[name]
		if len(targets) != 1 || seen[name] {
			return
		}
		seen[name] = true
		if alias := targets[0].AliasOf; alias != "" {
			visitInterface(alias)
			return
		}
		for _, dir := range index.directories[name] {
			if dir.Visibility != "PRIVATE" {
				paths = append(paths, dir)
			}
		}
		for _, link := range index.links[name] {
			if link.Visibility != "PRIVATE" && link.Visibility != "LINK_PRIVATE" {
				visitInterface(link.To)
			}
		}
	}
	for _, dir := range index.directories[root] {
		if dir.Visibility != "INTERFACE" {
			paths = append(paths, dir)
		}
	}
	for _, link := range index.links[root] {
		if link.Visibility != "INTERFACE" && link.Visibility != "LINK_INTERFACE_LIBRARIES" {
			visitInterface(link.To)
		}
	}
	index.cache[id] = paths
	return paths
}
