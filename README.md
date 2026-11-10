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

`architecture` returns the components and link declarations visible in CMake,
with source locations and project-scoped component IDs. Link endpoints use
canonical component IDs even when the original operand is an alias. Components
list their unambiguous aliases and alias declarations preserve source locations
and any candidate IDs. Conditional or unresolved CMake inputs remain declarations.

`files` returns C/C++ file paths, explicit CMake source memberships and include
references. Shared files can have multiple component IDs and unassigned files
have an empty list. The scanner first checks quoted paths relative to the source
file. It also checks source-tree include directories declared for the owning
target and propagated by the public/interface links of local CMake targets.
When an include can be matched in a target context, the scanner follows that
context through the included file. `searchComponentIds` identifies these
contexts, while `componentIds` records declared source membership.
The report preserves the directory declarations behind matches and lists
multiple candidates without selecting one. It handles literal paths and
build-interface paths based on `${CMAKE_CURRENT_SOURCE_DIR}` or
`${CMAKE_SOURCE_DIR}`. Other generator
expressions, generated sources, CMake file sets and compiler search paths remain
outside this static resolution.

`call-graph` returns syntactic function definitions, lambdas and call sites with
project-relative paths and one-based line/byte-column ranges (end exclusive).
Calls include the enclosing symbol when one is visible. A unique exact spelling
in the scanned definitions can appear as a candidate, while compiler-based
name lookup and overload resolution remain unavailable.

Request all three views with `--facts architecture,files,call-graph`. The project
ID is the local scan root. Within that project, join components by component ID,
files by relative path and callers by symbol ID. IDs derived from source locations
describe one scan snapshot. The scan emits all collected entries for the selected
facts and parses each source file once per request.

## Optional libclang resolution

With `--facts files,call-graph --resolve`, the engine loads libclang at runtime
from the selected Xcode toolchain or a local LLVM installation. Set
`SCRYER_LIBCLANG` to prefer a specific library. Static scans do not load LLVM.

The engine prefers a supplied `--compile-commands` path, then a database at the
project root, then a unique database under the project. When none is found, it
configures CMake in a temporary directory and removes the directory after the
scan. It passes compiler arguments to libclang without invoking the commands.
The JSON preserves Tree-sitter and CMake facts and adds semantic edges,
translation-unit target triples, coverage and diagnostic or skip details.
