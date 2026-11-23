package ipc

import "github.com/scryer-labs/scryer-cpp-engine/internal/ipc/posix"

// AttachPOSIXEntrypoints reuses the bounded, unique-name source call trace.
// It establishes a potential executable root, never a runtime invocation.
func (c *Collector) AttachPOSIXEntrypoints(facts *posix.Facts) {
	if facts == nil {
		return
	}
	analysis := ipcAnalysis{Collector: c, byName: map[string][]int{}}
	indices := map[Location]int{}
	for i, function := range c.functions {
		analysis.byName[function.name] = append(analysis.byName[function.name], i)
		indices[function.location] = i
	}
	trace := analysis.indexEntrypoints()
	cache := map[int][]posix.Entrypoint{}
	attach := func(observations []posix.Observation) {
		for i := range observations {
			source := observations[i].FunctionLocation
			if source == nil {
				continue
			}
			index, ok := indices[Location{File: source.File, Line: source.Line, Column: source.Column}]
			if !ok {
				continue
			}
			entries, known := cache[index]
			if !known {
				for _, entry := range trace.entrypoints(index) {
					converted := posix.Entrypoint{Location: posix.Location{File: entry.File, Line: entry.Line, Column: entry.Column}, Kind: entry.Kind}
					for _, step := range entry.CallChain {
						converted.CallChain = append(converted.CallChain, posix.Trace{Location: posix.Location{File: step.File, Line: step.Line, Column: step.Column}, Operation: step.Operation, Symbol: step.Symbol})
					}
					entries = append(entries, converted)
				}
				cache[index] = entries
			}
			observations[i].Entrypoints = entries
		}
	}
	attach(facts.UnixSocket.Endpoints)
	attach(facts.UnixSocket.Observations)
	attach(facts.SharedMemory.Endpoints)
	attach(facts.SharedMemory.Observations)
}
