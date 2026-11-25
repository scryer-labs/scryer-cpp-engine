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

## Workflow triggers

Classify the candidate by its actual effect, not its filename or directory.
The full analysis → coding → independent review workflow applies when changes
affect repository build, test, execution or runtime behavior, including:

- implementation, tests or test data;
- executable scripts, dependencies, build or CI configuration;
- scan/provider plans, executable configuration or machine-consumed contracts;
- executable or generation logic located inside documentation directories.

Mixed behavior-changing and prose changes use the full workflow. Coding and the
final independent reviewer run the existing complete acceptance entries for the
final candidate. The two independent source/scan tracks below remain required
for that path. Do not treat every configuration file as lightweight.

Ordinary prose, `AGENTS.md`, subagent role descriptions/prompts and working
agreements, and other content without repository executable-behavior changes use
the [lightweight path](#lightweight-path). Role TOML prompt wording alone is not a
product implementation change. A `.md`, `.toml`, `.yaml` or `.json` suffix does
not decide the path. Inspect ambiguous effects before classifying; if executable
behavior is discovered, promote the candidate to the full workflow. Report the
classification and checks actually performed.

## Sequential subagent workflow

For candidates that trigger the full workflow above, use the following stages.
Lightweight changes do not require these subagents, scans or complete plans.

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
3. **Code review** is independent of coding. Build source-review observations and
   fresh Scryer facts in two independent tracks for the same candidate, then
   reconcile their factual inventories as below. Directly inspect implementation
   and tests against requirements, contracts, responsibilities and regression
   risks in every comparison branch. Send actionable findings to coding rather
   than silently rewriting implementation. After fixes, review again and refresh
   both affected tracks. Once review passes, rerun the complete acceptance entry
   against the final candidate. Only with user commit authorization and passing
   final review/acceptance, make small commits and report hashes, key changes,
   evidence and remaining limits. Explicit user authorization is separately
   required for push.

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

## Reviewer source and scan tracks

This section applies only to full-workflow candidates. Lightweight changes do
not require a reviewer or Scryer scan.

Use two independent evidence tracks bound to the same candidate. Neither an
AI/source inventory nor scanner output is presumed correct. Self-scanning is
supporting evidence, not a correctness certificate for the scanner or its code.

1. **Source track**: Read the actual diff, relevant source, contracts and tests.
   Record a code-derived factual inventory, review findings, checked scope and
   supporting source references. Include definitions, component/file ownership,
   imports/references, calls, relationships, source positions and classifications.
   When the tracks run sequentially, record this inventory and findings before
   viewing this candidate's scan output. The scanner must never supply the sole
   inspection checklist or determine the initial source-review scope.
2. **Scan track**: Resolve the configured Scryer executable and installed engine
   inventory/capabilities. Automatically detect implementation languages: an
   engine repository's target language is not necessarily its own implementation
   language. Collect the full relevant advertised fact set, including dependencies,
   architecture, files, call graphs and communication where supported. Do not
   force the repository's engine ID or a permanently fixed small fact whitelist.
   Retain original JSON, stderr, exit status, command/mode, candidate identity,
   scope/include/exclude policy, per-engine/per-fact outcomes and scan/run identity.
   Record CLI path/version when available (or executable hash), engine versions,
   package/library identities and hashes, provider provenance if used, and whether
   collection uses installed releases or candidate-built packages.

Safe parallel tracks are optional; independence is required. If sequential,
source observations come first. Parallelism must not mix outputs between tracks
before their inventories are recorded or conflict with clean/build/package paths
or installation homes. Do not install, upgrade or rebuild tools implicitly to make
a scan pass. Semantic resolution is appropriate when applicable, available and
within scope; record its prerequisites and static/semantic modes separately.
A static scan is not compiler or runtime binding.

For direct collection, use runtime-resolved `REPO_ROOT`, `REVIEW_DIR` and
capability-derived comma-separated `FACTS`, retaining evidence outside the
reviewed repositories. For example:

```sh
scryer engine list --json > "$REVIEW_DIR/engines.json"
scryer project scan "$REPO_ROOT" --facts "$FACTS" --json --compact \
  > "$REVIEW_DIR/facts.json" 2> "$REVIEW_DIR/scan.stderr"
```

Record the exit status even when nonzero. A configured provider workflow is also
valid if it preserves the same raw facts and provenance; an architecture-only
projection is insufficient. Do not use `-o report.json` or silently substitute old
facts. TOML/Markdown workflow semantics, native/cgo and unsupported languages may
not be modeled; their absent facts do not establish absent behavior.

### Factual reconciliation

Compare factual inventories, not correctness verdicts. Verify discrepancies
against actual source, tests and applicable contracts, correcting whichever
observation is wrong. Keep the distinction between source candidates, compiler
bindings and runtime observations throughout reconciliation.

| Comparison | Required action |
| --- | --- |
| Both tracks agree | Record agreement within the checked scope, then continue normal source/requirements and test review. Matching inventories can share omissions. |
| Source-only fact | Verify the source observation and requested contract. Report a confirmed scan omission only when real code evidence establishes that the fact should have been collected. Otherwise correct the mistaken AI judgment, record a coverage limit, or leave it unconfirmed. |
| Scan-only fact | Verify it in code and tests. If correct, add it to the source-review scope and review the related paths; if disproved, report a confirmed scan false positive. Insufficient evidence remains unconfirmed. |
| Ownership, target, direction, classification or position differs | Trace code and contract evidence, correct the wrong side and retain the evidence explaining the discrepancy. Neither track wins by default. |
| Evidence is insufficient | Retain an unconfirmed item and its reason; do not force agreement or label an unknown as a confirmed scanner error. |

Inspect actual code and tests in every branch, including relevant paths missing
from or unresolved by the scan. Iterate the expanded review scope and
reconciliation until differences are explained or explicitly unknown. Never
invent an edge or turn a scanner defect into an application defect. On scan
failure, continue source review and state that scan-to-source verification is
incomplete; failure cannot establish agreement.

Retain the checked scope, source references, matched observations and discrepancy
log with fact identifiers where available. Keep five report buckets separate:

- **Code findings**: independent requirement, contract, responsibility and
  regression findings from direct source/test review.
- **Confirmed scanner omissions/false positives**: source-backed differences
  under the requested contract, not merely missing or unresolved facts.
- **Corrected AI judgments**: mistaken source observations corrected using real
  code and contract evidence.
- **Unconfirmed items and coverage limits**: unsupported, uncollected, partial,
  fallback, unresolved, failed scans and other insufficient evidence, with reasons.
- **Final candidate acceptance**: exact candidate identity, final review scope and
  validation/plan outcomes, independently of scanner accuracy.

If no discrepancy is found, say **“在已核对范围内一致”** and list that scope; do not
claim whole-repository correctness. Agreement can hide shared omissions.
Confidence comes from verified scope and repeatable regression evidence, not
repeated matching verdicts. Confirmed scanner or projection defects whose fixes
are authorized and in scope require a minimal reproducible example and meaningful
regression coverage. Report out-of-scope or cross-repository follow-ups without
silently expanding implementation ownership.

After fixes, refresh both affected source and scan tracks against the final
candidate and reconcile again. Preserve raw facts and provenance for handoff.
Impact analysis is not an additional reviewer duty yet. This does not waive
impact regression steps already included in an established complete acceptance
plan.

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

## Lightweight path

For the non-executable changes defined in Workflow triggers, the primary agent
may make the edits directly. Check content consistency, document references,
relevant configuration syntax and diff formatting (`git diff --check`). Confirm
the diff contains no implementation or executable-configuration change, preserve
unrelated work and report the actual changed files and checks.

Do not start a reviewer, Scryer self-scan, full acceptance pipeline or mandatory
three-agent sequence solely for these edits. No independent review or source/scan
reconciliation is required for this lightweight path. With user authorization,
commit and push after the required lightweight checks pass; do not invent a full
pipeline verdict for those checks.

This is not an exception for all configuration files. Changes to actual commands,
dependencies, tests/fixtures, build/package/CI logic, scan/provider plans, runtime
configuration, machine-consumed contracts or generated/executed behavior require
the full workflow when they affect repository behavior, even in a documentation
directory. Mixed candidates use the full path.

## Repository acceptance entries

Both coding and final independent review use the following complete entries for
changes that trigger the full workflow. Review reruns acceptance on its final accepted candidate.

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
