package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

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
		Repository string   `json:"repository"`
		Facts      []string `json:"facts"`
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
	for _, fact := range r.Scan.Facts {
		if fact != "dependencies" {
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
	includes, err := ScanIncludes(ctx, root)
	if err != nil {
		return nil, &protocolError{"CPP_SCAN_ERROR", err.Error()}
	}
	buildFacts, err := cmake.Scan(ctx, root)
	if err != nil {
		return nil, &protocolError{"CMAKE_SCAN_ERROR", err.Error()}
	}
	return &report{1, root, map[string]any{
		"dependencies": map[string]any{"includes": includes, "cmake": buildFacts},
	}}, nil
}
