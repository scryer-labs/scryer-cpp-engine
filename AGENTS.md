# Scryer C++ Engine agent instructions

Before starting work, read [docs/WORKING-NOTES.md](docs/WORKING-NOTES.md).
It owns this repository's collaboration workflow, responsibilities, evidence
principles and complete acceptance entry. Read its linked contracts as affected.

Use analysis → coding → independent review subagents in sequence. The primary
agent owns user discussion and handoffs. Role definitions live in `.codex/agents/`.
If the client cannot discover custom roles, the primary reads their files and
passes the instructions to available subagents. If subagents are unavailable,
report that limitation; do not claim independent review.

Do not implicitly change plans, restart user services or expand permissions.
Commit only with user authorization after final review and acceptance; push
requires separate explicit authorization. Apply the documented exception only
to documentation and agent-role configuration, not product/build configuration.
