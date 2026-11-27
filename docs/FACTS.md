# C++ fact extensions

These additive fields use report schemaVersion 1 and C ABI v1. Consumers must
retain unrecognized fields. Evidence candidates must not be promoted to confirmed
ownership, compiler bindings or runtime execution. Historical reports omit these
fields; omission means unavailable evidence, not a negative finding.

## File ownership

`files.files[].componentIds` retains its original meaning: explicit CMake source
membership. `searchComponentIds` is an include/compilation context, not ownership.
Each file additionally has `ownership`:

- `status`: `declared`, `candidate`, `ambiguous`, or `unassigned`.
- `candidateComponentIds`: possible providers identified independently of explicit
  source membership. A unique candidate is still a candidate.
- `evidence[]`: `basis`, `componentId` and applicable `declaration`, `definition`
  source ranges or `viaFile` source path. Current bases are `cmake-source-list`,
  `declaration-definition-name-candidate` and `included-header-provider-candidate`.
- `exposures[]`: PUBLIC/INTERFACE include-root declarations that expose this
  header, including component ID, visibility, directory, file and line. Exposure
  is not ownership: a target can re-export another target's public headers.

The source comparison initially covers free-function declarations/definitions,
using namespace-qualified spelling and excluding static helpers and pointer
variables. It is not compiler overload binding. Mock and implementation providers
remain ambiguous. Header umbrellas can inherit common provider candidates from
observed local header dependencies. Type-only headers, templates, generated and
unobserved declarations can remain unassigned. No directory-name or target-name
heuristic silently assigns a provider. Candidates never modify `componentIds`.

Consumers should display declared, candidate and unknown ownership separately.
Do not reject a clean layering view on the basis of candidate-only ownership or
substitute the nearest CMakeLists directory as confirmed ownership.

## File-internal C function candidates

Bare calls in lowercase `.c` files can prefer one same-file free-function
implementation with an explicit AST `static` storage specifier. Storage order,
comments and newlines do not change classification. The existing optional
`candidateBasis` is `file-internal-definition`, with `resolution: name-candidate`;
this never supplies a compiler `targetSymbolId` or proves runtime invocation.
No ABI or report schema version changes are required.

This refinement is blocked by duplicate/conditional or malformed definitions,
known parameter/local-name shadowing, and selected-source macro definitions of
the callee (including empty replacements). Boundary resolutions include
`ambiguous-name`, `conditional-definition`, `unresolved-definition`,
`local-binding-boundary` and `macro-name-boundary`. Local binding collection is
conservative across the entire enclosing function, not full block/order lookup.
Macro observation does not execute preprocessing or interpret `#undef`/conditions.

A `.c` filename does not establish the configured compiler language. Visible C++
constructs and additional same-file declarations disable the refinement; `.C`
and C++ extensions do not use it. This is not overload resolution or complete
translation-unit visibility. Other-file static-inline header candidates and
ordinary external-name candidates are retained. Class-static methods are not
classified as file-internal free functions. Ownership collection uses the same
literal storage classifier to exclude static helpers regardless of modifier order.

## Macro callable source scopes and completeness

`call-graph.symbols[]` may contain `kind: "macro-body"` and
`projection: "syntactic-adjacent-block"`. A recovered top-level macro invocation
followed by a compound body supplies a source scope, with the same file/offset ID
convention and full source range. Calls inside the body receive its caller ID;
the macro invocation itself is not a recursive call to that scope. Nested ordinary
functions/lambdas keep their own scopes. Macro spelling never supplies a test role.
Impact can compare these bounded source-body projections as well.

Compiler binding to GoogleTest registration or the Catch2 `Catch::AutoReg`
constructor in its framework declaration can establish TEST/testContext at the
macro scope's start. An ordinary registration call inside a function does not
classify that function. Without usable compiler evidence, Catch2-looking and
custom test macros remain unknown-role source scopes. System-defined macros
sharing Catch2 names are not silently classified as Catch2.

`call-graph.completeness` reports `status` (`bounded` or `partial`),
`parseRecoveryFiles`, `macroBodyProjections`, `callsWithoutCaller` and limitations.
Tree-sitter recovery can come from valid macros or unsupported syntax; it is not
proof of a compile error. Global initializers can legitimately lack a caller.
Source grouping is neither full macro expansion nor execution-order evidence.

## Macro and function-pointer candidates

Calls may include `candidateSymbolIds[]` and `candidateBasis` (`macro-forwarding`
or `struct-field-initializer`). The original call site and expression remain.
Simple macro forwarding preserves its definition location in `macroExpansions[]`.
`symbolReferences[]` records function-valued arguments and struct initializer
bindings, with source range, expression, candidate IDs and applicable struct/field.

Typed struct field chains can use observed function-pointer initializers as
candidates. All compatible observed initializers remain possible; this is not
object-sensitive analysis. Conflicting struct layouts, unresolved typedefs,
complex macro expansion, conditional preprocessing and mutations can prevent or
broaden projection. Static impact preserves these candidates and their basis;
compiler-resolved impact continues to use compiler targets exclusively.

## POSIX communication and dynamic loader evidence (v0.4.0)

The `ipc` fact adds `posix.unixSocket` and `posix.sharedMemory`. Each mechanism
contains `observations[]` (including unresolved operations) and `endpoints[]`
(resource-bearing participation candidates). Existing Windows facts are unchanged.
Every observation retains `api`, `expression`, source file/line/column, enclosing
function information, `role`, `status`, `reason` and applicable `name`, `transport`
and `accessFlags`. `resolved-resource` means a literal/macro/unique local assignment
supplies a resource name, not that a resource exists or a message was exchanged.

The initial native source rules cover `socket(AF_UNIX/AF_LOCAL, ...)`, `bind`,
`connect`, `sendto`, `recvfrom`, `shm_open`, `shm_unlink`, `mmap` and `munmap`.
Socket address names come from bounded local `sun_path` assignments or copying a
known string. `SOCK_DGRAM` and `SOCK_STREAM` remain distinct. Shared-memory names
can be literal strings or unambiguous object-like macro constants. `O_CREAT`
marks a possible creator; it does not prove that the call created a new object.
Cleanup/view operations remain observations rather than communication endpoints.

Potential entrypoint traces reuse the named-call index, preserving source roots
and call chains. Only explicit executable source membership supplies component
IDs; a library is not a process. Indirect global arrays, aliasing, arbitrary
formatting, descriptor reuse, cross-function value flow and unresolved constants
remain boundaries. Lambda definitions alone are not execution roots. Native API
spelling is a source observation, not compiler binding; conditional branches can
coexist. The facts retain these limitations explicitly.

`call-graph.dynamicBindings[]` records `dlopen`/`dlsym` lookup requests, with call
site ID/range, handle expression, requested name when known, and source symbol
candidates. `resolved-name` is not an actual runtime target. In particular,
`RTLD_NEXT` and preload replacement depend on loader order and environment.
No ordinary call edge is inferred solely from a lookup request.

CMake source declarations involving `LD_PRELOAD` in `set`, `add_test`,
`add_custom_target` or `set_tests_properties` appear in `runtimeBindings[]` on
CMake dependency facts and architecture facts. They retain the original command,
expression, location and conditional context with `status: declared-environment`.
No host environment is read and no preload, build or test command is executed by
static scanning. General linker interposition and generated launchers are outside
this bounded collection.

Core normalizes POSIX endpoints using a language-independent contract and retains
raw engine observations. See the Core
[communication contract](https://github.com/scryer-labs/scryer/blob/main/docs/contracts/WORKSPACE-IPC.md).
