package coverage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

const cmakeReply = ".cmake/api/v1/reply"

func requestCodeModel(build string) error {
	query := filepath.Join(build, ".cmake/api/v1/query")
	if err := os.MkdirAll(query, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(query, "codemodel-v2"), nil, 0600)
}

type cmakeTarget struct {
	Name      string
	Type      string
	Artifacts []struct{ Path string }
}

func readCodeModel(ctx context.Context, build string) ([]cmakeTarget, error) {
	reply := filepath.Join(build, cmakeReply)
	models, err := filepath.Glob(filepath.Join(reply, "codemodel-v2-*.json"))
	if err != nil || len(models) != 1 {
		return nil, fmt.Errorf("expected one CMake File API codemodel")
	}
	var model struct {
		Configurations []struct {
			Targets []struct{ JSONFile string }
		}
	}
	if err := readJSON(models[0], &model); err != nil {
		return nil, err
	}
	if len(model.Configurations) != 1 {
		return nil, fmt.Errorf("expected a single CMake build configuration")
	}
	var targets []cmakeTarget
	for _, ref := range model.Configurations[0].Targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !filepath.IsLocal(ref.JSONFile) {
			return nil, fmt.Errorf("invalid CMake target reply path")
		}
		var target cmakeTarget
		if err := readJSON(filepath.Join(reply, ref.JSONFile), &target); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

var cmakeTargetName = regexp.MustCompile(`^[a-zA-Z0-9_.+][a-zA-Z0-9_.+-]*$`)

func selectedTargetPaths(targets []cmakeTarget, names []string) ([]string, error) {
	paths := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if !cmakeTargetName.MatchString(name) || seen[name] {
			return nil, fmt.Errorf("invalid or duplicate CMake test target %q", name)
		}
		seen[name] = true
		index := slices.IndexFunc(targets, func(target cmakeTarget) bool { return target.Name == name })
		if index < 0 {
			return nil, fmt.Errorf("CMake test target %q was not found", name)
		}
		target := targets[index]
		if target.Type != "EXECUTABLE" || len(target.Artifacts) != 1 {
			return nil, fmt.Errorf("CMake test target %q must produce a single executable artifact", name)
		}
		paths = append(paths, target.Artifacts[0].Path)
	}
	return paths, nil
}

func coverageObjects(build string, targets []cmakeTarget, binaries []string) ([]string, error) {
	objects := slices.Clone(binaries)
	for _, target := range targets {
		switch target.Type {
		case "EXECUTABLE":
			if len(binaries) != 0 {
				continue
			}
		case "SHARED_LIBRARY", "MODULE_LIBRARY":
		default:
			continue
		}
		for _, artifact := range target.Artifacts {
			object, err := buildArtifact(build, artifact.Path)
			if err == nil && !slices.Contains(objects, object) {
				objects = append(objects, object)
			}
		}
	}
	if len(objects) == 0 {
		return nil, fmt.Errorf("no executables or shared libraries found inside the CMake build")
	}
	return objects, nil
}

func buildArtifact(build, relative string) (string, error) {
	if !filepath.IsLocal(relative) {
		return "", fmt.Errorf("expected a relative path inside the coverage build: %q", relative)
	}
	path, err := filepath.EvalSymlinks(filepath.Join(build, relative))
	if err != nil {
		return "", err
	}
	local, err := filepath.Rel(build, path)
	if err != nil || !filepath.IsLocal(local) {
		return "", fmt.Errorf("artifact resolves outside the coverage build: %q", relative)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("artifact is not a regular file: %q", relative)
	}
	return path, nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
