# Scryer C++ Engine

A macOS Scryer engine that embeds the Tree-sitter C++ parser and implements
Scryer's C ABI v1. The package contains a shared library and an engine manifest.

## Build and install

Build with Go and a local C compiler with cgo enabled. From this repository:

```sh
./scripts/build
scryer engine install cpp --from "$PWD/build/install/scryer-cpp-engine"
scryer engine list --json
scryer project scan /path/to/project --facts dependencies --json
```

The installed package embeds Tree-sitter. Scanning requires neither a C++
compiler on the user's machine nor a build of the target project.

## Facts

The `dependencies` fact returns source-level include directives under
`includes`, with a repository-relative file, a one-based line number and the
literal include target. Directives in conditional branches may be present.
Targets are unresolved, and this scan does not collect a CMake target graph.
