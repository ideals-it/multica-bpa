---
name: multica-bpa-production-workflow
description: "Use for one production-impacting task owned end-to-end by its assigned agent, with human approval in In Review."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA production workflow

One assigned agent owns preparation, execution, verification, commit, and
clear communication on one root issue. Keep the issue `In Progress` while
working. Before the first production-impacting action—deployment, production
configuration/data/IAM/secret change, or external write—post a concise plan and
move the issue to `In Review`.

The approval is for the task scope, not for every command. Describe the action,
its expected impact, rollback path, and material residual risk in plain
language. Do not perform the production action while the issue is in review.

Independent review of the prepared material change is ordinary task work, not a
human approval gate. Perform it, resolve actionable findings, and re-verify
before requesting production approval. If an independent reviewer cannot run,
perform and accurately label a separate self-review; do not block or ask the
human for approval solely because of that limitation.

## Human conversation

Read the whole human reply and its ticket context. Treat it as natural language,
not as a whitelist of words or an emoji.

- If its meaning clearly approves the described scope, state that
  interpretation briefly in a ticket comment, return the issue to `In Progress`,
  and perform only that scope.
- If it is conditional, a question, a refusal, or ambiguous, do not make a
  production change. Reply with the remaining question and a real member
  mention when a response is needed.
- If the title or description materially changes, treat it as new scope: update
  the plan and request a new review before production work.

After the approved action, verify the result, post a concise plain-language
outcome, and move the issue to `Done`. Use `Blocked` only for a named blocker.

For long-running preparation or verification, progress updates are non-blocking
heartbeats: use one concise Markdown blockquote beginning with `> ` and then
continue work without waiting for acknowledgement. Reserve a normal comment
and `In Review` for an actual human decision.

Do not create child issues merely to split preparation, review, deployment, or
GitHub publication. Create one only for independent work with its own useful
deliverable.

Runtime behavior is mapped in `references/production-workflow-source-map.md`.
