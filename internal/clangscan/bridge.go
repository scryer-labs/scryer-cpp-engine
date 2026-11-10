//go:build cgo && darwin

package clangscan

/*
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/cgo"
	"strings"
	"unsafe"
)

func openClang(ctx context.Context) (*C.ScryerClang, string) {
	paths := []string{os.Getenv("SCRYER_LIBCLANG")}
	if output, err := exec.CommandContext(ctx, "xcode-select", "-p").Output(); err == nil {
		paths = append(paths, filepath.Join(strings.TrimSpace(string(output)), "usr/lib/libclang.dylib"),
			filepath.Join(strings.TrimSpace(string(output)), "Toolchains/XcodeDefault.xctoolchain/usr/lib/libclang.dylib"))
	}
	paths = append(paths, "/opt/homebrew/opt/llvm/lib/libclang.dylib", "/usr/local/opt/llvm/lib/libclang.dylib", "libclang.dylib")
	for _, path := range paths {
		if path == "" {
			continue
		}
		name := C.CString(path)
		api := C.scryer_clang_open(name)
		C.free(unsafe.Pointer(name))
		if api != nil {
			return api, path
		}
	}
	return nil, ""
}

func parseUnit(api *C.ScryerClang, args []string, state *unitState, includes, calls bool) error {
	handle := cgo.NewHandle(state)
	defer handle.Delete()
	argv := make([]*C.char, len(args))
	for i, arg := range args {
		argv[i] = C.CString(arg)
		defer C.free(unsafe.Pointer(argv[i]))
	}
	var scanIncludes, scanCalls C.int
	if includes {
		scanIncludes = 1
	}
	if calls {
		scanCalls = 1
	}
	status := C.scryer_clang_scan(api, &argv[0], C.int(len(argv)), C.uintptr_t(handle), scanIncludes, scanCalls)
	if status != 0 {
		return fmt.Errorf("libclang could not parse translation unit (error %d)", status)
	}
	return nil
}

//export scryerClangCancelled
func scryerClangCancelled(handle C.uintptr_t) C.int {
	if cgo.Handle(handle).Value().(*unitState).ctx.Err() != nil {
		return 1
	}
	return 0
}

//export scryerClangLocal
func scryerClangLocal(handle C.uintptr_t, file *C.char) C.int {
	state := cgo.Handle(handle).Value().(*unitState)
	if state.isSource(C.GoString(file)) {
		return 1
	}
	return 0
}

//export scryerClangTarget
func scryerClangTarget(handle C.uintptr_t, triple *C.char) {
	state := cgo.Handle(handle).Value().(*unitState)
	if value := C.GoString(triple); value != "" {
		state.unit.Target = value
	}
}

//export scryerClangDiagnostic
func scryerClangDiagnostic(handle C.uintptr_t, severity C.uint, message *C.char) {
	state := cgo.Handle(handle).Value().(*unitState)
	if severity >= 3 {
		state.unit.Diagnostics = append(state.unit.Diagnostics, C.GoString(message))
	}
}

//export scryerClangEdge
func scryerClangEdge(handle C.uintptr_t, kind C.uint, file *C.char, line, column C.uint, target *C.char, targetLine, targetColumn C.uint, definition C.int, name, usr *C.char) {
	state := cgo.Handle(handle).Value().(*unitState)
	source := Location{File: state.path(C.GoString(file)), Line: uint(line), Column: uint(column)}
	destination := Location{File: state.path(C.GoString(target)), Line: uint(targetLine), Column: uint(targetColumn)}
	edge := Edge{Unit: state.unit.ID, Site: source, Target: destination, Name: C.GoString(name), SymbolUSR: C.GoString(usr)}
	if kind == 503 {
		edge.TargetKind = "include"
		state.includes = append(state.includes, edge)
	} else {
		edge.TargetKind = "declaration"
		if definition != 0 {
			edge.TargetKind = "definition"
		}
		state.calls = append(state.calls, edge)
	}
}
