# Scryer C++ Engine

A Scryer engine for C++, C and CMake facts. It embeds Tree-sitter and ships a
C ABI v1 shared library and an `engine.json` manifest, with no standalone CLI.
Scryer Core calls the engine, aggregates its JSON facts, compares impact
snapshots, and renders reports. CMake extraction and C/C++ extraction remain
separate modules.

| Operation | Collection | Target-project requirements |
|---|---|---|
| Static scan | Tree-sitter and CMake source declarations | Source files; no build or external parser installation |
| Scan with `--resolve` | Static facts enriched with libclang observations | Local libclang and usable compiler arguments; CMake/toolchain/dependencies if a database must be generated |
| Coverage | Import an LLVM export, or build and run instrumented tests | Automatic collection currently requires macOS, a buildable CMake project and Apple LLVM tools |
| Impact | Shared scan evidence for immutable before/after snapshots | Git repository; `--resolve` or selecting test targets/binaries also requires compiler resolution and after-snapshot coverage |

## Install and scan

Engine ID: `cpp`. Official packages cover macOS arm64 and Windows amd64.
Install a missing engine or upgrade an installed one with the current Scryer CLI:

```sh
scryer engine install cpp
scryer engine upgrade cpp
scryer project scan /path/to/project --facts dependencies,architecture,files,call-graph,ipc --json --compact
```

For engine development, build with the Go version in `go.mod`, cgo and a C
compiler. Install the resulting local package into Scryer:

```sh
./scripts/build
scryer engine install cpp --from "$PWD/build/install/scryer-cpp-engine"
```

The local `install --from` command expects `cpp` to be absent. To replace an
existing engine with a development package, run `scryer engine uninstall cpp`
first. Remote `upgrade cpp` installs the official release instead of rebuilding
local source.

Static scanning uses the embedded parser. Compiler resolution and automatic
coverage have the additional requirements described below. The package does not
bundle libclang, CMake, a target-project compiler or LLVM coverage tools.

Engine facts are JSON. Core supplies terminal and Markdown scan views; scan HTML
is not provided. Analyze can write `.md` or offline `.html` with `-o`; structured
JSON uses `--json` on stdout, not `-o report.json`.

## Project file selection

With current Core, optional `.scryer/project.yaml` selects Core stack/LOC and
C++/CMake inputs together:

```yaml
schemaVersion: 1
scan:
  include: ["CMakeLists.txt", "cmake/**", "src/**", "include/**", "tests/**"]
  exclude: ["src/generated/**", "tests/fixtures/**"]
```

Core owns parsing, Git-aware inventory and patterns. The Engine's independent
`internal/selection` module validates a temporary, checksummed path-list reference;
`internal/cmake.ScanFiles` and the C/C++ collector consume the same list without
interpreting Core configuration. The package declares `scanFileSelection: true`.
It accepts a `scan.fileSelection` reference (`path`, `sha256`, `fileCount`) and
returns a `selection` receipt (`sha256`, `fileCount`). Impact accepts matching
`beforeSelection`/`afterSelection` references and returns per-snapshot receipts.
No C ABI symbol changes are required. See Core's
[project scope contract](https://github.com/scryer-labs/scryer/blob/main/docs/contracts/PROJECT-SCOPE.md).

Without configuration existing traversal is unchanged. With configuration only
selected C/C++ and CMake files produce observations. Referenced external or
excluded targets may remain boundary references; libclang may still read headers
as compilation context. Include build files intentionally when restricting the
scope. Scope does not change configure/build commands, test execution or native
coverage instrumentation, and does not imply code/test classification.
Git-ignored untracked files are outside the candidate inventory. Empty include
selects no files. Core rejects missing selection receipts instead of accepting
an unfiltered result. Package language matching includes C++, C and CMake;
header-only projects without those detected languages remain a dispatch limitation.

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
in the scanned definitions can appear as a candidate.
The symbol table also preserves initialized variable templates as declaration
nodes and marks explicit `consteval` functions with `executionKind: "compile-time"`.
Semantic declaration/type references are retained in `semantic.references[]`,
separately from calls in `semantic.edges[]`. These can preserve dependencies
through variable-template instantiations and unevaluated `decltype` expressions.
Instantiations share a source node, so their dependency union is conservative.
With `--resolve`, semantic edges can link `callSiteId`, `callerSymbolId` and
`targetSymbolId` when compiler evidence and source locations allow. A compiler
reference or call expression does not establish runtime execution. Dependent
expressions, unobserved template instantiations, virtual dispatch and unprojected
types can still leave boundaries.

Request all three views with `--facts architecture,files,call-graph`. The project
ID is the local scan root. Within that project, join components by component ID,
files by relative path and callers by symbol ID. IDs derived from source locations
describe one scan snapshot. The scan emits all collected entries for the selected
facts and parses each source file once per request.

## Optional libclang resolution

With `--facts files,call-graph --resolve`, the engine loads libclang at runtime
from the selected Xcode toolchain or a local LLVM installation on macOS,
or from LLVM on Windows. Set `SCRYER_LIBCLANG` to prefer a specific library.
Static scans do not load LLVM. Windows release checks cover static scans.

The engine prefers a supplied `--compile-commands` path, then a database at the
project root, then a unique database under the project. When none is found, it
configures CMake in a temporary directory and removes the directory after the
scan. It passes compiler arguments to libclang without invoking the commands.
The JSON preserves Tree-sitter and CMake facts and adds semantic edges,
translation-unit target triples, which source units were parsed or lacked
commands, and diagnostic or skip details. This is compiler parsing, not test coverage.
The resolver does not build or run the target, although CMake configuration can
fetch declared dependencies or execute project configure commands.

```sh
scryer project scan . --facts files,call-graph --resolve --json --compact
scryer project scan . --facts files,call-graph --resolve --compile-commands build/compile_commands.json --json --compact
```

Scan reports `resolved`, `partial` or `skipped` resolution evidence. The selected
compiler arguments and platform conditions limit the observed graph; this does
not include all inactive platform implementations. Dynamic impact applies the
stricter failure rule described below.

C++ call sites have a project-scoped `callSiteId`. With semantic resolution,
`semanticEdgeIds` links a site to its observed libclang edges. Those edges retain
their translation unit and source evidence and include `callerSymbolId` and
`targetSymbolId` when they can be matched to the static symbols. Compiler-only
edges remain in the semantic list even when no syntactic call site matches.

## Deterministic test roles

Resolved call-graph and impact symbols can carry `role: "TEST"` and
`testContext` evidence from compiler-bound test registration. The standalone
`internal/testclassification` module keeps its API rules in `registrations.json`.
Its initial GoogleTest rule recognizes the registration API, including macro
wrappers, rather than requiring a macro to be named `TEST`. A matching compiler
USR and framework declaration at the macro-defined function's start are required.
An ordinary function calling the registration API, a test-like filename, linked
framework or helper call does not establish a test-only scope. Unknown roles
remain unknown. Catch2 and other C++ test frameworks currently have no automatic
test-role recognition rule. Their tests remain unclassified unless supported
registration evidence is available; a test-like name does not change that.
This limitation concerns role classification, not the ability to run selected
executables or collect their coverage. Other framework rules, build registrations
and user-defined rules are deferred and can extend this Engine module.

## Windows named-pipe facts

`--facts ipc` scans source for Windows named-pipe evidence without building the
target. Project JSON exposes it as the `ipc` engine fact under
`engineFacts[].data.windows.namedPipe`.

- `endpoints` records names recovered from literal strings or simple file-scope
  constants, with the endpoint role and its source evidence.
- `parameterEvidence` records a zero-based parameter index when the name
  flows from a function or constructor parameter into a native IPC call. This
  includes supported forwarding calls, member initializers and factory
  construction. The evidence chain records where each step was observed.
- `unresolved` records recognized IPC calls whose names could not be traced.

Alongside `windows.namedPipe`, `data.calls` records qualified calls with statically known string arguments and
candidate symbols. Core can join these observations to parameter evidence
from the project or a referenced library, then aggregate project/workspace
communication relationships. The calls themselves carry no endpoint
role. Named endpoints and pipe-name calls can include entrypoint evidence and
the CMake executable IDs associated with the entrypoint source.

These are Windows API source facts and can be collected on macOS without
running Windows code. The current IPC collector covers Windows named pipes and
shared memory; it does not provide general REST, RPC or all-OS IPC discovery.
See Core's [communication contract](https://github.com/scryer-labs/scryer/blob/main/docs/contracts/WORKSPACE-IPC.md)
for aggregation.

`CreateFile` is treated as a client observation when the argument is a named-pipe
path or the same source value reaches `WaitNamedPipe` in the function. Test
observations carry a legacy `sourceRole: test` hint based on the source
directory. This IPC-specific hint is not the deterministic `role: "TEST"` and
`testContext` described above. This is bounded source analysis, so dynamic names, arbitrary data flow and cross-project
matching are outside this scan.

## Windows shared-memory facts

The project-level `ipc` fact also includes `data.windows.sharedMemory` when
the scan observes a supported Windows file-mapping API. `mappings` groups
statically recovered object names and keeps each `create` and `open`
operation with its source location, declared access, and available backing
evidence. `views` records `MapViewOfFile` and `MapViewOfFileEx` calls.
`handleSourceCandidateId` points to a single preceding create/open observation
using the same handle spelling in the function, when available. Mapping
operations whose names could not be recovered appear under `unresolved`.

Bounded source traces can connect operations to `main` or a file-scope
initializer through unambiguous calls and supported factory patterns.
Initializer traces preserve enclosing `#ifdef`/`#ifndef` guards where
available. The IPC scan attaches CMake executable IDs to entrypoints with
declared source membership. Include `architecture` to get the component table. These observations describe
source and access capabilities. Actual memory reads and writes require
runtime evidence.

## Runtime coverage

On macOS, automatic coverage uses CMake, its Unix Makefiles generator, Apple
Clang and matching `llvm-profdata`/`llvm-cov` tools found through `xcrun`.
The source root must contain `CMakeLists.txt` and no in-source `CMakeCache.txt`.
The engine configures a temporary instrumented Debug build with
`BUILD_TESTING=ON`, builds, runs CTest or selected executables, merges profiles
and reads the LLVM export. It removes its temporary build afterwards. The
project must support building and running its selected tests on the host;
Windows automatic collection is not implemented.

No source edits or existing coverage export are required. Project-defined CMake
configuration and test commands still execute as part of this operation.

```sh
scryer project analyze . --operations coverage --json
scryer project analyze . --operations coverage --test-target unit_tests,integration_tests --json
scryer project analyze . --operations coverage --test-binary tests/unit,tests/integration --json
scryer project analyze . --operations coverage --coverage-file /path/to/llvm-cov.json --json
```

With no test selection, automatic coverage runs the project's CTest inventory.
A project without CTest registration can select executable targets instead.
`--test-target` matches exact CMake executable target names through the CMake
File API, builds them and runs their reported artifacts. It does not recursively
search for similarly named files; wildcards and fuzzy matching are not supported.
Paths passed to `--test-binary` start at the temporary build directory, not the
repository or an existing local build. Prefer target names when available.
Both flags accept repeated or comma-separated selections. Target selection,
binary selection and `--coverage-file` import are mutually exclusive.
The last command imports an existing v2 or v3
[llvm-cov export](https://llvm.org/docs/CommandGuide/llvm-cov.html#export-command).
Both paths report aggregate source-region evidence with an artifact hash.
Standalone coverage describes the supplied/current source, without an immutable
Git comparison. File paths and an artifact hash do not identify a source revision.
A collected profile does not identify which individual test exercised a region
or establish assertion effectiveness. Missing profiles are not zero coverage.

## Change impact

The `impact` engine operation reuses scan collection for two immutable
snapshots. It supplies named function and initialized variable-template
identities, token fingerprints, source ranges, and call/reference evidence.
Core owns snapshot selection, comparison, graph traversal, coverage matching
and presentation. It defaults to the `impact` analyze operation.

```sh
# Static impact: no tests are run.
scryer project analyze . --json -o impact.md
# Compiler-resolved impact plus after-snapshot coverage.
scryer project analyze . --test-target unit_tests --json -o impact.html
# Explicit revisions; both flags are required together.
scryer project analyze . --before HEAD~1 --after HEAD --test-target unit_tests -o impact.html
```

Core defaults to the latest two commits for a clean working tree, or HEAD and
an immutable working-tree snapshot when there are uncommitted changes. Explicit
`--before` and `--after` override that selection and must appear together.
Coverage alone does not accept comparison revisions. Use `--stack cpp` only when
an explicit engine selection is needed.

Comments and whitespace do not mark a function changed. File moves and signature
changes appear as removal and addition. Ambiguous identities and source context
outside the projected declarations remain explicit uncertainty. Candidate edges
express potential connections. Without test selection, impact reuses static scan
collection. Selecting
`--test-target` or `--test-binary` reuses scan's compiler-resolution path and
after-snapshot coverage. Runtime coverage is `NOT_APPLICABLE` for immediate
functions and variable-template declaration nodes; their references do not prove
runtime execution or constant evaluation. They are excluded from assessed runtime
coverage counts. The shared resolver joins cross-unit definitions by compiler USR.
A skipped/partial compiler resolution, failed build/test, missing coverage profile
or a mutated snapshot stops dynamic impact; there is no static fallback report.
Core verifies the after snapshot stayed clean before associating collected
coverage with its commit.

Impact alone includes matching coverage evidence. `--operations impact,coverage`
also includes the full coverage report. JSON keeps all evidence edges; Core's
HTML groups parallel evidence into inspectable connections. Neither these graphs
nor an execution percentage prove a unique runtime/test path or refactoring safety.
See the [impact contract](https://github.com/scryer-labs/scryer/blob/main/docs/contracts/IMPACT.md).

## Implementation and verification

| Module | Responsibility |
|---|---|
| `cmd/cpp-engine-abi` | Thin C ABI v1 entry point, execution/cancellation and response buffers |
| `internal/engine` | Request dispatch, fact assembly, source identities and scan-to-impact projection |
| `internal/cmake` | Independent static CMake declaration collection |
| `internal/clangscan` | Optional libclang loading, compilation databases and compiler evidence |
| `internal/ipc` | Windows named-pipe/shared-memory observations and bounded source traces |
| `internal/coverage` | Temporary instrumented builds, test execution and LLVM normalization |
| `internal/testclassification` | Compiler registration rules and narrow test-role evidence |
| `internal/process` | Subprocess cancellation support |

The [GitHub workflow](.github/workflows/ci.yml) runs Go tests and vet, builds the
macOS arm64 and Windows amd64 packages, and calls
[`scripts/check-package.py`](scripts/check-package.py) through the packaged ABI
for static scan and impact checks. A `vMAJOR.MINOR.PATCH` tag publishes both ZIP
assets after the matrix succeeds. Other workflow runs do not publish a release.
`SCRYER_ENGINE_VERSION` controls local package version metadata, defaulting to
`0.0.0-dev`; `SCRYER_ENGINE_PACKAGE` overrides its output directory.

Hosted package checks are not acceptance of arbitrary target builds or Windows
runtime coverage. Local development acceptance uses the configured Orrery plans
for the affected engine and Core integration. Engine facts and test-role rules
are independent of Core's Markdown/HTML report resources.

## Known gaps

- Static calls and includes retain candidates and unresolved boundaries. The
  compiler pass requires usable host build inputs; inactive platform code,
  unavailable template instantiations, indirect dispatch and dynamic wrappers
  are not guaranteed to resolve.
- Test-role registration currently supports GoogleTest evidence. Catch2 and
  other framework rules are deferred; their executables can still run for coverage.
- Runtime coverage currently uses the documented macOS CMake/Apple LLVM path;
  other build systems/compiler coverage formats and Windows runtime acceptance
  are not implemented. Test-target names are exact, without fuzzy matching.
- Coverage is aggregate, without per-test attribution or assertion-effectiveness
  proof. Test-effectiveness rules and C++ ownership/lifetime analysis remain planned.
- IPC supports the documented Windows named-pipe/shared-memory patterns and
  wrapper evidence, not arbitrary IPC APIs or REST/RPC/GraphQL.
