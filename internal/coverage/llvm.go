// Package coverage imports observations produced by coverage tools.
package coverage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type count struct {
	Covered uint64 `json:"covered"`
	Count   uint64 `json:"count"`
}

type summary struct {
	Lines     *count `json:"lines"`
	Functions *count `json:"functions"`
	Regions   *count `json:"regions"`
}

type File struct {
	Path     string          `json:"path"`
	Summary  summary         `json:"summary"`
	Segments json.RawMessage `json:"llvmSegments,omitempty"`
}

type Evidence struct {
	Format      string   `json:"format"`
	Version     string   `json:"version"`
	Artifact    string   `json:"artifact"`
	SHA256      string   `json:"sha256"`
	SourceMatch string   `json:"sourceMatch"`
	Granularity string   `json:"granularity"`
	Files       []File   `json:"files"`
	Notes       []string `json:"notes"`
}

const maxArtifactBytes = 128 << 20

func ImportLLVM(ctx context.Context, root, artifact string) (Evidence, error) {
	info, err := os.Stat(artifact)
	if err != nil {
		return Evidence{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxArtifactBytes {
		return Evidence{}, fmt.Errorf("coverage input must be a regular file no larger than 128 MiB")
	}
	file, err := os.Open(artifact)
	if err != nil {
		return Evidence{}, err
	}
	defer file.Close()
	hash := sha256.New()
	input := &io.LimitedReader{R: file, N: maxArtifactBytes + 1}
	decoder := json.NewDecoder(io.TeeReader(input, hash))
	var export struct {
		Type    string `json:"type"`
		Version string `json:"version"`
		Data    []struct {
			Files []struct {
				Filename string          `json:"filename"`
				Summary  summary         `json:"summary"`
				Segments json.RawMessage `json:"segments"`
			} `json:"files"`
		} `json:"data"`
	}
	err = decoder.Decode(&export)
	if input.N == 0 {
		return Evidence{}, fmt.Errorf("coverage input exceeds 128 MiB")
	}
	if err != nil {
		return Evidence{}, fmt.Errorf("decode llvm-cov export: %w", err)
	}
	if decoder.Decode(new(json.RawMessage)) != io.EOF || input.N == 0 {
		return Evidence{}, fmt.Errorf("coverage input must contain one JSON document within 128 MiB")
	}
	major, _, _ := strings.Cut(export.Version, ".")
	if export.Type != "llvm.coverage.json.export" || (major != "2" && major != "3") || len(export.Data) != 1 || export.Data[0].Files == nil {
		return Evidence{}, fmt.Errorf("expected one llvm-cov JSON dataset using export format v2 or v3 (got %q %q)", export.Type, export.Version)
	}
	result := Evidence{
		Format: "llvm-cov", Version: export.Version, Artifact: artifact,
		SHA256: fmt.Sprintf("%x", hash.Sum(nil)), SourceMatch: "unverified",
		Granularity: "aggregate", Files: []File{},
		Notes: []string{"The artifact does not identify the source revision or individual test runs."},
	}
	excludedPaths := false
	seen := map[string]bool{}
	for _, source := range export.Data[0].Files {
		if err := ctx.Err(); err != nil {
			return Evidence{}, err
		}
		if !filepath.IsAbs(source.Filename) {
			excludedPaths = true
			continue
		}
		path, err := filepath.Rel(root, source.Filename)
		if err != nil || !filepath.IsLocal(path) || path == "." {
			excludedPaths = true
			continue
		}
		path = filepath.ToSlash(path)
		if seen[path] {
			return Evidence{}, fmt.Errorf("duplicate coverage path %q", path)
		}
		seen[path] = true
		for _, metric := range []*count{source.Summary.Lines, source.Summary.Functions, source.Summary.Regions} {
			if metric == nil || metric.Covered > metric.Count {
				return Evidence{}, fmt.Errorf("invalid coverage summary for %q", path)
			}
		}
		if len(source.Segments) > 0 && source.Segments[0] != '[' {
			return Evidence{}, fmt.Errorf("invalid coverage segments for %q", path)
		}
		result.Files = append(result.Files, File{Path: path, Summary: source.Summary, Segments: source.Segments})
	}
	if excludedPaths {
		result.Notes = append(result.Notes, "Coverage paths that could not be mapped to the project root were omitted.")
	}
	return result, nil
}
