package engine

import (
	"path"
	"slices"

	"github.com/scryer-labs/scryer-cpp-engine/internal/cmake"
	"github.com/scryer-labs/scryer-cpp-engine/internal/ipc"
)

func attachMappingComponents(memory *ipc.SharedMemory, build cmake.Facts) {
	components := componentsByName(build.Targets)
	bySource := make(map[string][]string)
	for _, source := range build.Sources {
		file := path.Join(path.Dir(source.File), source.Path)
		for _, id := range components[source.Target] {
			if !slices.Contains(bySource[file], id) {
				bySource[file] = append(bySource[file], id)
			}
		}
	}
	attach := func(observations []ipc.MappingObservation) {
		for _, observation := range observations {
			for i, entry := range observation.Entrypoints {
				observation.Entrypoints[i].ComponentIDs = bySource[entry.File]
			}
		}
	}
	for _, mapping := range memory.Mappings {
		attach(mapping.Operations)
	}
	attach(memory.Views)
	attach(memory.Unresolved)
}
