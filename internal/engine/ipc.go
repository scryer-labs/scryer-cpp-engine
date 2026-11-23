package engine

import (
	"github.com/scryer-labs/scryer-cpp-engine/internal/ipc/posix"
	"slices"

	"github.com/scryer-labs/scryer-cpp-engine/internal/cmake"
	"github.com/scryer-labs/scryer-cpp-engine/internal/ipc"
)

func attachIPCComponents(facts *ipc.Facts, build cmake.Facts) {
	executables := make(map[string][]string)
	for _, target := range build.Targets {
		if target.Kind == "executable" && !target.Imported {
			executables[target.Name] = append(executables[target.Name], componentID(target))
		}
	}
	bySource := make(map[string][]string)
	for _, source := range build.Sources {
		file := source.ProjectPath()
		for _, id := range executables[source.Target] {
			if !slices.Contains(bySource[file], id) {
				bySource[file] = append(bySource[file], id)
			}
		}
	}
	attachPOSIXComponents(facts.Posix, bySource)
	attach := func(entries []ipc.Entrypoint) {
		for i := range entries {
			entries[i].ComponentIDs = bySource[entries[i].File]
		}
	}
	for _, endpoint := range facts.Windows.NamedPipe.Endpoints {
		attach(endpoint.Entrypoints)
	}
	for _, call := range facts.Calls {
		attach(call.Entrypoints)
	}
	if memory := facts.Windows.SharedMemory; memory != nil {
		for _, mapping := range memory.Mappings {
			for _, observation := range mapping.Operations {
				attach(observation.Entrypoints)
			}
		}
		for _, observation := range memory.Views {
			attach(observation.Entrypoints)
		}
		for _, observation := range memory.Unresolved {
			attach(observation.Entrypoints)
		}
	}
}

func attachPOSIXComponents(facts *posix.Facts, sources map[string][]string) {
	if facts == nil {
		return
	}
	attach := func(observations []posix.Observation) {
		for i := range observations {
			for j := range observations[i].Entrypoints {
				entry := &observations[i].Entrypoints[j]
				entry.ComponentIDs = sources[entry.File]
			}
		}
	}
	attach(facts.UnixSocket.Endpoints)
	attach(facts.UnixSocket.Observations)
	attach(facts.SharedMemory.Endpoints)
	attach(facts.SharedMemory.Observations)
}
