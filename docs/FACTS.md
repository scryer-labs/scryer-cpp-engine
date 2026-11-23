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
