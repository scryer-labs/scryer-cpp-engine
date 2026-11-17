package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/scryer-labs/scryer-cpp-engine/internal/clangscan"
	"github.com/scryer-labs/scryer-cpp-engine/internal/cmake"
	"github.com/scryer-labs/scryer-cpp-engine/internal/coverage"
)

const (
	MaxRequestBytes   = 1 << 20
	MaxRequestIDBytes = 256
)

type Request struct {
	ProtocolVersion int    `json:"protocolVersion"`
	ID              string `json:"requestId"`
	Stack           string `json:"stack"`
	Operation       string `json:"operation"`
	Scan            struct {
		Repository      string   `json:"repository"`
		Static          *bool    `json:"static"`
		CompileCommands string   `json:"compileCommands"`
		Facts           []string `json:"facts"`
	} `json:"scan"`
	Coverage struct {
		Repository string `json:"repository"`
		Artifact   string `json:"artifact"`
	} `json:"coverage"`
}

func ParseRequest(data []byte) (Request, error) {
	var request Request
	if err := json.Unmarshal(data, &request); err != nil || request.ID == "" || len(request.ID) > MaxRequestIDBytes {
		return Request{}, errors.New("invalid request JSON or requestId")
	}
	return request, nil
}

type protocolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type report struct {
	SchemaVersion int            `json:"schemaVersion"`
	Root          string         `json:"root"`
	Facts         map[string]any `json:"facts"`
}

func (r Request) Execute(ctx context.Context) ([]byte, error) {
	var result any
	var failure *protocolError
	switch {
	case r.ProtocolVersion != 1 || r.Stack != "cpp":
		failure = &protocolError{"INVALID_REQUEST", "unsupported protocol version or stack"}
	case r.Operation == "scan":
		result, failure = r.scan(ctx)
	case r.Operation == "coverage":
		result, failure = r.collectCoverage(ctx)
	default:
		failure = &protocolError{"UNSUPPORTED_OPERATION", "C++ engine does not support operation " + r.Operation}
	}
	if failure != nil {
		result = nil
	}
	return json.Marshal(struct {
		ProtocolVersion int            `json:"protocolVersion"`
		ID              string         `json:"requestId"`
		Stack           string         `json:"stack"`
		Operation       string         `json:"operation"`
		Report          any            `json:"report"`
		Error           *protocolError `json:"error"`
	}{1, r.ID, "cpp", r.Operation, result, failure})
}

func (r Request) scan(ctx context.Context) (*report, *protocolError) {
	selected := r.Scan.Facts
	if len(selected) == 0 {
		selected = []string{"dependencies"}
	}
	for _, fact := range selected {
		if fact != "dependencies" && fact != "architecture" && fact != "files" && fact != "call-graph" && fact != "ipc" {
			return nil, &protocolError{"UNSUPPORTED_FACT", "C++ engine does not support " + fact}
		}
	}
	root := r.Scan.Repository
	if !filepath.IsAbs(root) {
		return nil, &protocolError{"INVALID_REQUEST", "scan requires an absolute repository directory"}
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil, &protocolError{"INVALID_REQUEST", "scan repository must be an existing directory"}
	}
	architectureRequested := slices.Contains(selected, "architecture")
	filesRequested := slices.Contains(selected, "files")
	dependenciesRequested := slices.Contains(selected, "dependencies")
	callsRequested := slices.Contains(selected, "call-graph")
	ipcRequested := slices.Contains(selected, "ipc")

	var buildFacts cmake.Facts
	if architectureRequested || filesRequested || dependenciesRequested || ipcRequested {
		var err error
		buildFacts, err = cmake.Scan(ctx, root)
		if err != nil {
			return nil, &protocolError{"CMAKE_SCAN_ERROR", err.Error()}
		}
	}

	facts := make(map[string]any, len(selected))
	if architectureRequested {
		facts["architecture"] = describeArchitecture(root, buildFacts)
	}
	if filesRequested || dependenciesRequested || callsRequested || ipcRequested {
		sources, err := scanSources(ctx, root, filesRequested || dependenciesRequested, callsRequested, ipcRequested)
		if err != nil {
			return nil, &protocolError{"CPP_SCAN_ERROR", err.Error()}
		}
		if ipcRequested {
			ipcFacts := sources.ipc.Facts()
			attachIPCComponents(&ipcFacts, buildFacts)
			facts["ipc"] = ipcFacts
		}
		if dependenciesRequested {
			facts["dependencies"] = map[string]any{"includes": sources.includes, "cmake": buildFacts}
		}
		if r.Scan.Static != nil && !*r.Scan.Static && (filesRequested || callsRequested) {
			resolution, err := clangscan.Resolve(ctx, root, r.Scan.CompileCommands, sources.files, filesRequested, callsRequested)
			if err != nil {
				return nil, &protocolError{"CPP_SCAN_ERROR", err.Error()}
			}
			sources.resolution = &resolution
		}
		if filesRequested {
			facts["files"] = describeFiles(root, buildFacts, sources)
		}
		if callsRequested {
			if sources.resolution != nil {
				sources.graph.joinSemantic(sources.resolution)
			}
			facts["call-graph"] = sources.graph
		}
	}
	return &report{1, root, facts}, nil
}

func (r Request) collectCoverage(ctx context.Context) (any, *protocolError) {
	root, artifact := r.Coverage.Repository, r.Coverage.Artifact
	if !filepath.IsAbs(root) || !filepath.IsAbs(artifact) {
		return nil, &protocolError{"INVALID_REQUEST", "coverage requires absolute project and artifact paths"}
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil, &protocolError{"INVALID_REQUEST", "coverage project must be an existing directory"}
	}
	evidence, err := coverage.ImportLLVM(ctx, root, artifact)
	if err != nil {
		return nil, &protocolError{"CPP_COVERAGE_ERROR", err.Error()}
	}
	return struct {
		SchemaVersion int               `json:"schemaVersion"`
		Root          string            `json:"root"`
		Evidence      coverage.Evidence `json:"coverage"`
	}{1, root, evidence}, nil
}
