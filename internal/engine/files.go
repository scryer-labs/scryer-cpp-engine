package engine

import (
	"path"
	"slices"

	"github.com/scryer-labs/scryer-cpp-engine/internal/cmake"
)

type membership struct {
	cmake.Location
	ComponentID string `json:"componentId"`
}

type sourceFile struct {
	Path               string       `json:"path"`
	ComponentIDs       []string     `json:"componentIds"`
	SearchComponentIDs []string     `json:"searchComponentIds,omitempty"`
	Memberships        []membership `json:"memberships,omitempty"`
}

type fileReference struct {
	Include
	Column     uint               `json:"column"`
	To         string             `json:"to,omitempty"`
	Resolution string             `json:"resolution"`
	Candidates []includeCandidate `json:"candidates,omitempty"`
}

type includeCandidate struct {
	Path        string        `json:"path"`
	SearchPaths []includePath `json:"searchPaths"`
}

type filesReport struct {
	Semantic   *semanticFacts  `json:"semantic,omitempty"`
	ProjectID  string          `json:"projectId"`
	Files      []sourceFile    `json:"files"`
	References []fileReference `json:"references"`
}

func describeFiles(root string, build cmake.Facts, source sourceFacts) filesReport {
	components := componentsByName(build.Targets)
	result := filesReport{ProjectID: root, Files: make([]sourceFile, 0, len(source.files)), References: make([]fileReference, 0, len(source.includes))}
	byPath := make(map[string]int, len(source.files))
	for _, file := range source.files {
		byPath[file] = len(result.Files)
		result.Files = append(result.Files, sourceFile{Path: file, ComponentIDs: []string{}})
	}
	for _, source := range build.Sources {
		file := path.Join(path.Dir(source.File), source.Path)
		index, exists := byPath[file]
		if !exists {
			continue
		}
		entry := &result.Files[index]
		for _, id := range components[source.Target] {
			if !slices.Contains(entry.ComponentIDs, id) {
				entry.ComponentIDs = append(entry.ComponentIDs, id)
			}
			entry.Memberships = append(entry.Memberships, membership{Location: source.Location, ComponentID: id})
		}
	}
	if source.resolution != nil {
		result.Semantic = &semanticFacts{Resolution: source.resolution, Edges: source.resolution.Includes}
	}
	search := indexIncludePaths(root, build)
	propagateIncludeContexts(result.Files, source.includes, byPath, search)
	for _, include := range source.includes {
		ref := resolveInclude(include, result.Files[byPath[include.File]].SearchComponentIDs, byPath, search)
		result.References = append(result.References, ref)
	}
	return result
}

// Each reachable file is visited once per compilation target. The context
// describes how it was reached, without adding a CMake membership.
func propagateIncludeContexts(files []sourceFile, includes []Include, byPath map[string]int, search *includePaths) {
	byFile := make(map[string][]int)
	for i, include := range includes {
		byFile[include.File] = append(byFile[include.File], i)
	}
	type visit struct {
		file        int
		componentID string
	}
	var pending []visit
	seen := map[visit]bool{}
	enqueue := func(file int, componentID string) {
		item := visit{file: file, componentID: componentID}
		if !seen[item] {
			seen[item] = true
			files[file].SearchComponentIDs = append(files[file].SearchComponentIDs, componentID)
			pending = append(pending, item)
		}
	}
	for i, file := range files {
		for _, id := range file.ComponentIDs {
			enqueue(i, id)
		}
	}
	for head := 0; head < len(pending); head++ {
		item := pending[head]
		for _, i := range byFile[files[item.file].Path] {
			ref := resolveInclude(includes[i], []string{item.componentID}, byPath, search)
			if ref.To != "" {
				enqueue(byPath[ref.To], item.componentID)
			}
		}
	}
}

func resolveInclude(include Include, componentIDs []string, byPath map[string]int, search *includePaths) fileReference {
	ref := fileReference{Include: include, Column: include.Column, Resolution: "unresolved"}
	raw := include.Target
	if len(raw) < 3 {
		return ref
	}
	quoted := raw[0] == '"' && raw[len(raw)-1] == '"'
	angled := raw[0] == '<' && raw[len(raw)-1] == '>'
	name := raw[1 : len(raw)-1]
	if (!quoted && !angled) || path.IsAbs(name) {
		return ref
	}
	if quoted {
		candidate := path.Join(path.Dir(include.File), name)
		if _, exists := byPath[candidate]; exists {
			ref.To, ref.Resolution = candidate, "relative-path"
			return ref
		}
	}

	candidateIndex := map[string]int{}
	for _, id := range componentIDs {
		for _, dir := range search.forComponent(id) {
			candidate := path.Join(dir.Directory, name)
			if _, exists := byPath[candidate]; !exists {
				continue
			}
			i, exists := candidateIndex[candidate]
			if !exists {
				i = len(ref.Candidates)
				candidateIndex[candidate] = i
				ref.Candidates = append(ref.Candidates, includeCandidate{Path: candidate})
			}
			paths := ref.Candidates[i].SearchPaths
			if !slices.ContainsFunc(paths, func(existing includePath) bool {
				return existing.Target == dir.Target && existing.File == dir.File && existing.Line == dir.Line && existing.Expression == dir.Expression
			}) {
				ref.Candidates[i].SearchPaths = append(paths, dir)
			}
		}
	}
	switch len(ref.Candidates) {
	case 1:
		ref.To, ref.Resolution = ref.Candidates[0].Path, "cmake-include-path"
	default:
		if len(ref.Candidates) > 1 {
			ref.Resolution = "ambiguous"
		}
	}
	return ref
}
