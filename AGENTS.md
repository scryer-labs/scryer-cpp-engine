# Scryer C++ Engine agent instructions

Before starting work, read [docs/WORKING-NOTES.md](docs/WORKING-NOTES.md).
It owns this repository's collaboration workflow, responsibilities, evidence
principles and complete acceptance entry. Read its linked contracts as affected.

Classify changes by actual effects as described in the working notes. Changes
that affect build, test, execution or runtime behavior use analysis → coding →
independent review subagents. Ordinary prose, AGENTS.md and subagent instruction
edits without repository executable-behavior changes use lightweight checks;
do not start a reviewer, Scryer scan, full pipeline or mandatory agent sequence
solely for them. Mixed changes use the full workflow regardless of file suffix.
The primary agent owns user discussion and handoffs. Role definitions live in
`.codex/agents/`.
For full-workflow tasks, if the client cannot discover custom roles, the primary
reads their files and passes the instructions to available subagents. If subagents are unavailable,
report that limitation; do not claim independent review.

Do not implicitly change plans, restart user services or expand permissions.
Commit and push only within user authorization after the selected path's required
checks pass. Independent review and complete acceptance are required for the full
workflow, not for lightweight prose/agent-instruction edits. Configuration that
changes repository executable behavior still takes the full path.
