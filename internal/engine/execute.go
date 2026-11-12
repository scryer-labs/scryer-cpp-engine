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
	result, failure := r.scan(ctx)
	return json.Marshal(struct {
		ProtocolVersion int            `json:"protocolVersion"`
		ID              string         `json:"requestId"`
		Stack           string         `json:"stack"`
		Operation       string         `json:"operation"`
		Report          *report        `json:"report"`
		Error           *protocolError `json:"error"`
	}{1, r.ID, "cpp", r.Operation, result, failure})
}

func (r Request) scan(ctx context.Context) (*report, *protocolError) {
	if r.ProtocolVersion != 1 || r.Stack != "cpp" {
		return nil, &protocolError{"INVALID_REQUEST", "unsupported protocol version or stack"}
	}
	if r.Operation != "scan" {
		return nil, &protocolError{"UNSUPPORTED_OPERATION", "C++ engine does not support operation " + r.Operation}
	}
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
	if architectureRequested || filesRequested || dependenciesRequested {
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
			facts["ipc"] = sources.ipc.Facts()
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
