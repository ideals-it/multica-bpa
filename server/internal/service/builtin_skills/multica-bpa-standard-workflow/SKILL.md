---
name: multica-bpa-standard-workflow
description: "Use when Team Lead coordinates one small, non-production deliverable in a single Multica ticket."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA standard workflow

Use this for one clear, non-production deliverable. Keep all work in **one root issue**:

```text
Lead → specialist → Quality → Lead
```

The same root issue contains the task description, agent handoffs, commit,
verification, risk, and final decision. A different agent is a new run on the
same issue, not a reason to create a child issue.

The backend behavior behind this workflow is traced in
`references/standard-workflow-source-map.md`.

## Status contract

- Before Lead routes work: `Todo`.
- Once Lead starts or routes work: `In Progress`.
- When only **Vitaliy Ustymenko** must decide or approve: `In Review`.
- After Lead accepts the Quality result: `Done`.
- When a named outside action is required: `Blocked`.

Do not leave an active root in `Todo` or `In Progress` without a named next
owner.

## Single-ticket handoff

1. Lead writes one short plan on the root, moves it to `In Progress`, and
   names the specialist with a clickable action mention, for example
   `**[@AT Builder](mention://agent/<builder-id>)**`.
2. The specialist works only on the root. It makes a focused local commit when
   repository files changed, then posts one final result on the root. The
   server automatically wakes the root Lead after the last specialist run, so
   a missing mention cannot strand the task. A clickable Lead mention remains
   useful only when the comment also asks a specific question.
3. Lead reads that result and, on the same root, mentions
   `**[@AT Quality](mention://agent/<quality-id>)**` for an independent check.
4. Quality posts one verdict on the root and mentions Team Lead.
5. Lead either closes the root as `Done`, sets a named `Blocked` state, or
   moves it to `In Review` when a human decision is genuinely required.

Every material comment uses separate Markdown paragraphs:

```text
**Результат:** <what is ready>

**Перевірка:** <how it was checked>

**Ризик:** <remaining risk or "немає відомого">

**Наступне:** <bold owner and exact action>
```

When a result is a local file inside the current task work directory, make the
visible full path clickable in Desktop. `MULTICA_TASK_ID` is available during a
daemon run; the target path is relative to that task work directory:

```markdown
[**/absolute/workdir/audit-results/report.json**](local-artifact://task/${MULTICA_TASK_ID}/open/audit-results/report.json)
```

Use `reveal` instead of `open` to select the file in its folder. Do not use
`file://`, do not link paths outside the work directory, and leave a plain path
when the result was not produced by the current task.

## Root summary before Done

After each material specialist or Quality handoff, Lead posts one short
plain-language progress update on this root before mentioning the next owner.
Before `Done`, Lead posts this final root summary. Do not paste logs, commands,
child comments, or a task transcript:

```text
**Що було не так:** <user-visible cause>

**Що змінили:** <plain-language change or no-change conclusion>

**Що перевірили:** <concise evidence>

**Результат:** <current outcome>

**Ризик / наступне:** <remaining risk or "немає відомого">
```

## When child issues are optional

Child issues are optional. Create one only when it has an independently useful
deliverable: parallel work, a different repository/system with its own
acceptance, or a substantial investigation that must remain separately
readable. Do not create a child merely for a specialist, Quality, GitHub Ops,
or a workflow stage.

## Boundaries

- Team Lead owns the root issue from start to finish.
- A specialist owns one scoped result. GitHub Ops owns only publication of an
  existing focused commit; it does not take over implementation.
- n8n Prod does not deploy or activate production changes through this
  standard template.
- This template does not authorize deployment, production configuration/data
  changes, IAM changes, secrets, or external writes. Use the Production
  workflow when any of those actions are in scope.
