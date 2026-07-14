---
name: multica-bpa-standard-workflow
description: "Use for one clear non-production task owned end-to-end by its assigned agent."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA standard workflow

One assigned agent owns the full lifecycle of one clear, non-production root
issue: understand it, implement or investigate, verify, make a focused commit
when repository files changed, and post the result. Do not invent Lead,
Builder, Quality, or GitHub Ops handoffs for normal work.

## Status contract

- `Backlog`: planned work. Autopilot-created assigned tickets remain parked
  until a human promotes or explicitly starts them. Ordinary Backlog tickets
  retain native Multica comment and mention behavior.
- `Todo` / `In Progress`: the agent is actively working.
- `In Review`: only when a human must decide a production-impacting action.
- `Done`: the requested non-production result is complete and verified.
- `Blocked`: a concrete missing input, access, or external dependency prevents
  progress. State exactly what is needed and mention the human owner when a
  reply is required.

Keep work in the root issue. Create a child only for a separately useful,
independently readable deliverable: genuinely parallel work, a different
repository or system, or a substantial investigation. Never create a child to
represent an internal phase, a review, a commit, or a handoff.

## Communication

Post concise, human-readable progress while a task runs for a long time. A
short update every 3–5 minutes is enough; do not post commands, logs, or a
step-by-step transcript. Use normal Markdown paragraphs, bold only for useful
labels, and code formatting for identifiers, commands, paths, and error text.

Use a real member mention when an answer or decision is needed. Use an agent
mention only for a concrete delegated task; it starts work and must never be a
polite acknowledgement. The final result should explain in plain language what
changed, how it was checked, and any remaining risk. Do not force a template
when a short clear result is better.

When the result is a file inside the current task work directory, make its
visible absolute path clickable in Desktop:

```markdown
[**/absolute/workdir/audit-results/report.json**](local-artifact://task/${MULTICA_TASK_ID}/open/audit-results/report.json)
```

The URI path is relative to the task work directory. Use `reveal` instead of
`open` to select the file in its folder. Never use `file://`, link outside the
work directory, or invent a link for a file not produced by this task.

## Boundaries

This workflow does not authorize deployment, production configuration or data
changes, IAM changes, secret changes, or other external writes. Move the root
to `In Review` and use the Production workflow before such an action.

Runtime behavior is mapped in `references/standard-workflow-source-map.md`.
