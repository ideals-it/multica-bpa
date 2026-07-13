---
name: multica-bpa-investigation-workflow
description: "Use for evidence-first, read-only diagnosis before deciding whether a separate fix task is needed."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA investigation workflow

Keep a focused investigation in **one root issue**:

```text
Lead → Investigator → Quality → Lead
```

Lead moves the root to `In Progress` and mentions the Investigator on the root
with a clickable action mention such as
`[@AT Builder](mention://agent/<agent-id>)`.
The Investigator is read-only against logs, code, configuration, and run
history. It posts one evidence report on the root. The server wakes Team Lead
after the final specialist run; use a clickable Lead mention only for a
specific question that needs attention before the normal handoff.
Lead then mentions Quality on the same root to test the evidence, not a fix.

```text
**Симптом:** <confirmed fact>

**Докази:** <links or exact observations>

**Гіпотези:** <tested causes and result>

**Висновок:** <confirmed cause or "недостатньо даних">

**Наступне:** <bold owner and exact action>
```

For a local evidence file produced in the current task work directory, use a
Desktop-safe artifact link:

```markdown
[**/absolute/workdir/audit-results/evidence.json**](local-artifact://task/${MULTICA_TASK_ID}/open/audit-results/evidence.json)
```

The target path must be relative to the current work directory. Do not use
`file://` or expose a path outside that directory.

Quality returns one verdict to Lead. Lead closes the root as `Done` when the
answer is evidenced, or sets it `Blocked` with the exact missing data. If a fix
is needed, Lead creates a separate Standard or Production root task; an
investigation не створює implementation-child.

## Root summary before Done

After each material handoff, Lead posts a short plain-language progress update
on the root before mentioning the next owner. Before `Done`, Lead writes this
final root summary. Do not paste logs, commands, child comments, or a task
transcript:

```text
**Що було не так:** <confirmed symptom or cause>

**Що змінили:** <"нічого" for read-only investigation, or the recommended next step>

**Що перевірили:** <concise evidence>

**Результат:** <confirmed conclusion or insufficient data>

**Ризик / наступне:** <remaining uncertainty and owner>
```

Child issues are optional only for independent evidence streams that need their
own readable deliverable. Do not create one for the Investigator or Quality
handoff.
