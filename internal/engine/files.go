package engine

import (
	"path"
	"slices"
	"strings"

	"github.com/scryer-labs/scryer-cpp-engine/internal/cmake"
)

type membership struct {
	cmake.Location
	ComponentID string `json:"componentId"`
}

type sourceFile struct {
	Path         string       `json:"path"`
	ComponentIDs []string     `json:"componentIds"`
	Memberships  []membership `json:"memberships,omitempty"`
}

type fileReference struct {
	Include
	Column     uint   `json:"column"`
	To         string `json:"to,omitempty"`
	Resolution string `json:"resolution"`
}

type filesReport struct {
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
	for _, include := range source.includes {
		ref := fileReference{Include: include, Column: include.Column, Resolution: "unresolved"}
		if name, ok := strings.CutPrefix(include.Target, "\""); ok {
			if name, ok := strings.CutSuffix(name, "\""); ok && !path.IsAbs(name) {
				candidate := path.Join(path.Dir(include.File), name)
				if _, exists := byPath[candidate]; exists {
					ref.To, ref.Resolution = candidate, "relative-path"
				}
			}
		}
		result.References = append(result.References, ref)
	}
	return result
}
