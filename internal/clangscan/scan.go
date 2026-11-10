//go:build cgo && darwin

// Package clangscan enriches static facts with an optional local libclang.
package clangscan

/*
#include "bridge.h"
*/
import "C"

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Location struct {
	File   string `json:"file"`
	Line   uint   `json:"line,omitempty"`
	Column uint   `json:"column,omitempty"`
}

type Edge struct {
	TargetKind string   `json:"targetKind"`
	Unit       string   `json:"unit"`
	Site       Location `json:"site"`
	Target     Location `json:"target"`
	Name       string   `json:"name,omitempty"`
	SymbolUSR  string   `json:"symbolUsr,omitempty"`
}

type Unit struct {
	ID            string   `json:"id"`
	File          string   `json:"file"`
	Target        string   `json:"target,omitempty"`
	Configuration string   `json:"configuration,omitempty"`
	Status        string   `json:"status"`
	Diagnostics   []string `json:"diagnostics,omitempty"`
}

type Result struct {
	Status               string   `json:"status"`
	Backend              string   `json:"backend"`
	Database             string   `json:"database,omitempty"`
	Library              string   `json:"library,omitempty"`
	Message              string   `json:"message,omitempty"`
	Units                []Unit   `json:"units"`
	FilesWithoutCommands []string `json:"filesWithoutCommands,omitempty"`
	Includes             []Edge   `json:"-"`
	Calls                []Edge   `json:"-"`
}

type unitState struct {
	ctx             context.Context
	root            string
	directory       string
	unit            Unit
	sources         map[string]bool
	includes, calls []Edge
}

func (s *unitState) path(file string) string {
	if !filepath.IsAbs(file) {
		file = filepath.Join(s.directory, file)
	}
	relative, err := filepath.Rel(s.root, filepath.Clean(file))
	if err == nil && filepath.IsLocal(relative) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(file)
}

func (s *unitState) isSource(file string) bool { return s.sources[s.path(file)] }

func Resolve(ctx context.Context, root, explicit string, files []string, includes, calls bool) (Result, error) {
	result := Result{Status: "skipped", Backend: "libclang", Units: []Unit{}, Includes: []Edge{}, Calls: []Edge{}}
	api, library := openClang(ctx)
	if api == nil {
		result.Message = "libclang was not found; set SCRYER_LIBCLANG to its library path"
		return result, ctx.Err()
	}
	defer C.scryer_clang_close(api)
	result.Library = library
	commands, database, cleanup, err := loadDatabase(ctx, root, explicit)
	defer cleanup()
	result.Database = database
	if err != nil {
		result.Message = err.Error()
		return result, ctx.Err()
	}

	sources := make(map[string]bool, len(files))
	for _, file := range files {
		sources[file] = true
	}
	covered := map[string]bool{}
	usable, clean := 0, 0
	var macSDK, resourceDir string
	var lookedUpToolchain bool
	for _, command := range commands {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		state := unitState{ctx: ctx, root: root, directory: command.Directory, sources: sources}
		file := state.path(command.File)
		if !sources[file] {
			continue
		}
		covered[file] = true
		state.unit = Unit{ID: file + ":" + strconv.Itoa(len(result.Units)+1), File: file}
		args, err := command.argv()
		if err == nil {
			state.unit.Target, state.unit.Configuration = buildFlags(args)
			if usesMacDriver(args[0], state.unit.Target) {
				if !lookedUpToolchain {
					lookedUpToolchain = true
					macSDK = xcrunPath(ctx, "--sdk", "macosx", "--show-sdk-path")
					resourceDir = xcrunPath(ctx, "clang", "-print-resource-dir")
				}
				if macSDK != "" && !hasCompilerOption(args, "-isysroot", "--sysroot") {
					args = slices.Insert(args, 1, "-isysroot", macSDK)
				}
				if resourceDir != "" && !hasCompilerOption(args, "-resource-dir") {
					args = slices.Insert(args, 1, "-resource-dir", resourceDir)
				}
			}
			err = parseUnit(api, args, &state, includes, calls)
		}
		if err != nil {
			state.unit.Status = "skipped"
			state.unit.Diagnostics = append(state.unit.Diagnostics, err.Error())
		} else {
			usable++
			state.unit.Status = "resolved"
			if len(state.unit.Diagnostics) != 0 {
				state.unit.Status = "partial"
			} else {
				clean++
			}
			result.Includes = append(result.Includes, state.includes...)
			result.Calls = append(result.Calls, state.calls...)
		}
		result.Units = append(result.Units, state.unit)
	}
	for _, file := range files {
		switch strings.ToLower(filepath.Ext(file)) {
		case ".c", ".cc", ".cpp", ".cxx", ".ixx", ".cppm":
			if !covered[file] {
				result.FilesWithoutCommands = append(result.FilesWithoutCommands, file)
			}
		}
	}
	switch {
	case usable == 0:
		result.Message = "no project translation unit could be parsed"
	case clean < len(result.Units) || len(result.FilesWithoutCommands) > 0:
		result.Status = "partial"
	default:
		result.Status = "resolved"
	}
	return result, ctx.Err()
}

// Apple's command-line driver supplies its SDK and builtin headers implicitly.
func usesMacDriver(compiler, target string) bool {
	switch compiler {
	case "/usr/bin/cc", "/usr/bin/c++", "/usr/bin/clang", "/usr/bin/clang++":
		return target == "" || strings.Contains(target, "-apple-macos") || strings.Contains(target, "-apple-darwin")
	default:
		return false
	}
}

func hasCompilerOption(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name || strings.HasPrefix(arg, name+"=") || (name == "-isysroot" && strings.HasPrefix(arg, name)) {
				return true
			}
		}
	}
	return false
}

func xcrunPath(ctx context.Context, args ...string) string {
	output, err := exec.CommandContext(ctx, "xcrun", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func buildFlags(args []string) (target, configuration string) {
	for i, arg := range args {
		if value, ok := strings.CutPrefix(arg, "--target="); ok {
			target = value
		}
		if (arg == "-target" || arg == "--target") && i+1 < len(args) {
			target = args[i+1]
		}
		if value, ok := strings.CutPrefix(arg, "-DCMAKE_INTDIR="); ok {
			configuration = strings.Trim(value, "\"")
		}
	}
	return target, configuration
}
