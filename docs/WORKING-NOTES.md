# Scryer C++ Engine working conventions

This file owns collaboration and acceptance rules. Product contracts, supported
facts and known limitations remain in the references below; this is not a roadmap.

## Responsibilities and references

Read [fact extensions](FACTS.md) and the [README](../README.md), including CMake,
libclang, IPC, deterministic test registration, coverage and evidence boundaries.
Use the affected canonical [Core contracts](https://github.com/scryer-labs/scryer/blob/main/docs/contracts/README.md)
for ABI, project selection, impact and communication.

The implementation is Go with cgo/native parsers. Keep C/C++ syntax collection,
the CMake declaration model and optional libclang resolution modular. The CMake
collector must not depend on C++ parsing or execute project commands during static
collection. Preserve source ranges, explicit membership versus provider candidates,
conditional declarations, selection receipts and unknown boundaries.

Keep macro-body source scopes, struct callback initialization and dynamic-loader
requests as bounded evidence; do not promote candidates to compiler binding,
concrete object dispatch or observed execution. IPC facts remain OS/mechanism
observations; Core owns language-independent project/workspace matching. Test roles
require deterministic registration evidence, not names or directory guesses.
Keep native coverage/scan projections independent of Core report resources.

## Coding and commit conventions

Use clear names/branches, named intermediate results and readable boundaries.
Prefer named arguments where useful; keep lambdas/pipelines when they improve
readability. Keep code comments in English. Add meaningful regression tests for
new behavior rather than tests that merely repeat the implementation.
Go tests use `Test_should_xxx_when_xxx` where applicable. Commit subjects use
`<type>: <summary>` with `feat`, `fix`, `refactor`, `tests` (not `test`), `docs`,
`chore` or `revert`. Choose the primary purpose, keep summaries concise, preserve
unrelated work and do not rewrite history without explicit user instruction.

## Sequential subagent workflow

The primary agent owns user discussion, requirement boundaries, coordination,
questions, ownership assignments and the final report. It coordinates the roles
below in order. These instructions are an orchestration convention, not an
automatically enforced pipeline. Do not parallelize dependent phases.

1. **Analysis** reads the latest conversation, source, Git state and applicable
   contracts. Stay read-only: do not modify implementation or commit. Hand off the
   problem, scope, constraints, affected repositories/boundaries, acceptance
   criteria, unresolved questions and an executable coding plan.
2. **Coding** implements that handoff within the authorized scope, adds meaningful
   regression tests and runs this repository's complete acceptance entry below.
   Deliver the diff, candidate identity, exact plan/run evidence and limitations.
   Do not commit before independent review, and do not push.
3. **Code review** is independent of coding. First collect fresh self-scan evidence
   as below, then directly inspect implementation and tests against requirements,
   contracts, responsibilities and regression risks. Send actionable findings to
   coding rather than silently rewriting implementation. After fixes, review
   again and refresh affected scans. Once review passes, rerun the complete
   acceptance entry against the final candidate. Only with user commit
   authorization and passing final review/acceptance, make small commits and
   report hashes, key changes, evidence and remaining limits. Explicit user
   authorization is separately required for push.

Shared workspaces need one writer per code area. Cross-repository work needs
explicit file/repository ownership and coordinated final integration acceptance.
Do not revert unrelated edits, rewrite history, change plans, restart user-owned
services or expand permissions implicitly. Do not overlap scans or complete plans
that clean/rebuild the same build/package directories or use the same installation
home. Read prerequisites and effects before scheduling them.

The root `AGENTS.md` is the persistent entry. Role definitions are in
`.codex/agents/`; shared settings are in `.codex/config.toml`. If custom roles are
not discoverable in the current client, the primary reads the relevant role file
and passes its instructions to an available subagent. If no subagents are
available, report the limitation; do not claim an independent review happened.
Do not pin a model, credentials, personal paths, permissions or runtime history in
shared role/settings files. Track these files; do not ignore all of `.codex/`.

## Candidate identity and evidence

At each handoff retain repository/branch/HEAD, staged and unstaged binary diffs,
relevant untracked file contents and hashes, scope/configuration and dependency
identities. HEAD alone does not identify a dirty candidate, and hashes alone
cannot restore its content. Bind review and acceptance to that candidate; any
later edit invalidates affected conclusions and requires refreshed evidence.
After an authorized commit, retain the resulting SHA. Keep raw facts, patches,
logs and run metadata in a task-specific local evidence directory outside the
reviewed repositories, not in shared role/configuration files or committed history.
A workspace-local `.orrery/reviews/` directory is one possible location.

## Reviewer self-scan and source reconciliation

Before reviewing the candidate, resolve the configured Scryer executable and
installed engine inventory/capabilities. Record CLI path/version when available
(or executable hash), engine versions, package/library identities and hashes,
provider/run identity if used, and whether facts came from installed releases or
candidate-built packages. A scan of a scanner is supporting evidence, never a
correctness certificate for that scanner.

Use automatic project language detection: the engine's target language is not
necessarily the implementation language of its own repository. Discover the full
relevant advertised scan-fact set from the actual participating capabilities,
including dependencies, architecture, files, call graphs and communication where
supported. Do not permanently restrict review to a small fixed fact whitelist or
force the repository's engine ID as its implementation language. Preserve scope
include/exclude policy and per-engine/per-fact outcomes. Do not install, upgrade
or rebuild tools implicitly to make a scan pass.

For a direct CLI scan, use the supported JSON stdout interface; for example,
with runtime-resolved `REPO_ROOT`, `REVIEW_DIR` and capability-derived comma-separated
`FACTS` (not literal placeholders):

```sh
scryer engine list --json > "$REVIEW_DIR/engines.json"
scryer project scan "$REPO_ROOT" --facts "$FACTS" --json --compact \
  > "$REVIEW_DIR/facts.json" 2> "$REVIEW_DIR/scan.stderr"
```

Record the exit status even when nonzero. Retain original stdout JSON, stderr,
exact command/mode, candidate identity, scanner inputs/provenance and any run ID.
A configured provider workflow is also valid if it retains the same raw facts
and provenance; an architecture-only projection is insufficient. Do not use
`-o report.json`. Use semantic resolution when applicable, available and within
scope; record toolchain/build prerequisites and static/semantic modes separately.
Do not overlap scanning with clean/build steps. A static scan must not be
represented as compiler or runtime binding.

Compare facts with real source in the checked scope: definitions, component/file
ownership, imports/references, calls, relationships, source locations and
classifications. Always read source and tests directly, including paths missing
from or unresolved by the scan. TOML/Markdown workflow semantics, native/cgo and
unsupported languages may not be modeled by installed engines; inspect them
rather than treating absent facts as absence of behavior.

Report these separately:

- Confirmed scanner omissions/false positives, with fact identifiers, source
  paths/positions and code evidence. Continue source review despite scanner errors.
- If no discrepancy is found, explicitly say **“在本次已核对范围内一致”** and list the
  checked scope. Never claim unverified whole-repository agreement.
- Unsupported, uncollected, partial, fallback, unresolved and failed collection.
  These are evidence limits, not automatically confirmed scanner bugs. If a scan
  fails, continue source review and report scan-to-source verification incomplete;
  never silently substitute old facts or claim agreement from failed checks.
- Independent correctness/contract/regression findings and final acceptance
  evidence. Matching facts do not replace source/test review.

After fixes, refresh affected facts for the final candidate. Impact analysis is
not an additional reviewer duty yet. This does not waive impact regression steps
already included in an established complete acceptance plan.

## Running acceptance plans

Use the existing Orrery plan-run API, not separately executed local test/build
commands as a substitute. Resolve the workspace root from current registration
or explicit configuration; do not assume every checkout's parent is its workspace.
Find this repository's registered project and effective plan target at runtime.

Read `.orrery/plans/definitions/<plan-id>/manifest.json`, its current revision and
frozen `revisions/<revision-id>/plan.yaml`. Verify active state, target, checks,
prerequisites and side effects. Invoke the configured Orrery server with
`POST /api/workspace/plan-definitions/<plan-id>/runs` and JSON
`{ "root": workspace_root, "revisionId": current_revision }`.
Retain `rootRunId`, frozen definition/effective target, step results, logs and
artifacts together with candidate identity. Inspect which checks actually ran;
a green summary or build alone does not prove tests/vet executed. Local platform
and toolchains can differ from hosted CI.

If registration, a required plan, prerequisite or server is unavailable, report
missing acceptance evidence. Do not invent a replacement plan, change its scope,
start services or silently substitute ad hoc tests. Apply the user's existing
authorization to known effects; request clarification only for a necessary effect
that is not already authorized. Ordinary read-only inspection and formatting are
allowed. Release publication remains separate from local package verification.

## Documentation and agent-role configuration only

For changes limited to documentation and `.codex` agent-role/settings files,
validate TOML syntax/reference schema, role names and duties, local links,
acceptance IDs/targets against current definitions, Git visibility and the
changed-file allowlist; run `git diff --check`. Independently review the actual
text/configuration and preserve candidate/validation evidence. Reviewer self-scan
still applies when available, with its inability to validate workflow prose
reported explicitly. No unconditional full product pipeline is required for this
slice; do not claim these checks as code-pipeline acceptance.

This exception does not cover product/runtime configuration, manifests such as
`engine.json`, `.scryer` behavior, build/package scripts, CI or executable behavior.
Those changes require their applicable complete acceptance plans. If a slice
mixes documentation with behavior changes, use the implementation workflow.

## Repository acceptance entries

Both coding and final independent review use the following complete entries for
implementation changes. Review reruns acceptance on its final accepted candidate.

| Entry | Existing plan ID | Effective target |
| --- | --- | --- |
| Full repository checks | `scryer-lab--cpp-impact` | workspace |
| Core/engine integration | `scryer-lab--scryer-cpp-engine-integration` | workspace |

The repository entry currently covers: The complete cpp-impact plan: engine-wide Go race tests/vet, ABI build and package check, followed by its configured impact, coverage and reflection fixtures.
Read the current frozen definition rather than copying these summaries into
commands or assuming they remain exhaustive. Hosted checks are described in
[CI](../.github/workflows/ci.yml); hosted upload/publication is not a local step.

If Core changes too, run its `scryer-lab--daily-development` entry first, then this
repository's complete entry, then the integration. Record Core and engine
candidate/package identities for cross-repository acceptance. Integration may
consume already-built outputs; verify their freshness against the candidates.

There is no dedicated C++ daily project definition. `scryer-lab--cpp-impact`
is the established full-check workspace entry; running only its first engine
check step is not completion of the plan. Existing scan/compiler regressions
remain in `scryer-lab--scryer-cpp-engine-integration`. CMake-model changes can
also require `scryer-lab--cmake-project-model`, and source-selection changes
`scryer-lab--project-scope`. Preserve their configured input/fixture boundaries.

Official package installation/upgrade changes use
`scryer-lab--all-release-installation` with isolated homes. Runtime, coverage and
mixed-language boundaries may require additional existing feature plans selected
from the current registration; do not omit relevant integration merely because
unit/package checks pass. Refer to each plan's frozen definition for effects and
prerequisites rather than assuming all integration is isolated.
