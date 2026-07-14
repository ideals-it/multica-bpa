---
name: multica-bpa-investigation-workflow
description: "Use for an evidence-first, read-only diagnosis before a separate fix task is decided."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA investigation workflow

The assigned agent investigates one root issue end-to-end, using logs, code,
configuration, run history, and other read-only evidence. Do not make a fix,
deploy, or production change as part of the investigation.

Keep the issue `In Progress` while evidence is being gathered. Post concise
progress only for long-running work; write it as one Markdown blockquote
beginning with `> ` and immediately continue the investigation. It is not a
handoff or a request for acknowledgement. Avoid technical noise. The final
comment must distinguish confirmed facts from hypotheses and state the symptom,
evidence, conclusion, and recommended next action in language useful to the
human owner.

Move the issue to `Done` when the question is answered with adequate evidence.
Move it to `Blocked` when a named missing data source or access requirement
prevents a conclusion, and mention the human owner only when their action is
needed. If a fix is warranted, create or request a separate Standard or
Production issue; do not turn an investigation into implementation silently.

Create child issues only for independent evidence streams that need their own
readable result. Do not create one for a review or internal workflow phase.

For evidence produced as a file inside the current task work directory, use a
Desktop-safe link with the visible absolute path:

```markdown
[**/absolute/workdir/audit-results/evidence.json**](local-artifact://task/${MULTICA_TASK_ID}/open/audit-results/evidence.json)
```

The URI path must be relative to the task work directory. Use `reveal` to show
the file in its folder; never use `file://` or link outside the work directory.
