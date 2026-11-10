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

The `dependencies` fact contains two independent collections:

- `includes` records C and C++ include directives with a repository-relative
  file, one-based line number and literal include target.
- `cmake` records library and executable targets, aliases, link declarations,
  external dependency declarations and subdirectories from `CMakeLists.txt`
  and `*.cmake` files. Each record includes its file and line. Conditional
  declarations carry their textual control-flow context. Dynamic names, link
  items and other supported arguments that require evaluation are recorded in
  `unresolved`.

The scan reads declarations without configuring a CMake build or fetching
dependencies. Link operands can name targets, libraries, paths or flags.
Include targets and link operands remain as written, and the scanner does not
connect the two datasets. Declarations may appear in inactive branches or in
files that a particular build does not load.
