---
name: multica-bpa-standard-workflow
description: "Use when Team Lead coordinates a small, non-production task through one specialist and one Quality review using Multica child stages."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA standard workflow

Use this only for a small, non-production task with one clear deliverable.
The flow is:

```text
Lead → specialist → Quality → Lead
```

This is a native Multica flow. Do not create a scheduler, an autopilot, an
event protocol, or `@`-mentions to move work between agents. Child completion
and the stage barrier wake the root assignee automatically.

The backend behavior behind this workflow is traced in
`references/standard-workflow-source-map.md`.

## Ownership

- Team Lead owns the root issue from start to finish. Only Team Lead creates
  children, promotes a parked stage, and closes the root.
- A specialist owns one child and one durable result. Use Documentation Writer,
  Builder, n8n Prod, or GitHub Ops only when its specialty matches the work.
- The agent that changes a repository owns one focused local commit after its
  logical part is complete. Its handoff includes the commit SHA and checks.
  If it made no repository change, it says `no repo changes` and why.
- GitHub Ops owns push, PR, merge, and GitHub-only publication. It does not
  take over another agent's local commit merely for ceremony.
- n8n Prod owns n8n workflow diagnosis, change preparation, and n8n-specific
  verification. It does not deploy or activate production changes without the
  Production workflow's approved execution stage.
- EventCatalog owns service and workflow documentation, event contracts, and
  catalog verification. It does not silently change runtime behavior.
- Quality is a separate child issue. It reviews the result; it is never an
  assignee swap on the specialist child.
- The root must be assigned to Team Lead before this workflow starts.

## Create the two-stage plan

On the root issue, Team Lead first writes the success criterion in the issue
description. Then create exactly these two children. Replace the placeholders
with real IDs and a result-focused title.

```bash
# Stage 1 starts now.
multica issue create \
  --title "Prepare <deliverable>" \
  --parent <root-id> \
  --assignee <specialist-id> \
  --stage 1 --status todo

# Stage 2 is created now but stays parked.
multica issue create \
  --title "Quality check <deliverable>" \
  --parent <root-id> \
  --assignee <quality-id> \
  --stage 2 --status backlog
```

After creating the children, Lead stops work. Do not add a progress comment and
do not tag the specialist. Stage 1 finishing wakes Lead through Multica.

## Specialist completion

The specialist completes only its own child. Before marking it `done`, verify
the deliverable against the root's success criterion and post exactly one
handoff comment:

```text
Результат: <what is ready, with a link or file when relevant>

Перевірка: <how it was checked>

Commit: <SHA, or "no repo changes: <reason>">

Ризик: <remaining risk, or "немає відомого">

Наступне: Team Lead запускає Quality-перевірку.
```

Do not create another child, change the root assignee, or comment merely to say
that work started. Then mark the specialist child `done`.

## Question or blocker

Normal handoff never needs a tag: completion wakes Lead. If work cannot
continue or needs a decision, write one short child comment and mark the child
`blocked`. Multica wakes Lead automatically. Never tag another specialist or
Quality to route work.

## Lead promotes Quality

When the Stage 1 barrier closes, Multica wakes Team Lead on the root. Lead reads
the result and either records one concrete blocker or promotes the existing
Quality child:

```bash
multica issue status <quality-child-id> todo
```

Do not create a duplicate Quality child and do not manually run Quality before
the specialist result exists.

## Quality completion and Lead closeout

Quality checks the stated criterion, then posts one comment on its own child:

```text
Результат: <accepted result, or the single concrete defect>

Висновок: Прийнято / Потрібні зміни

Перевірка: <what Quality checked>

Ризик: <remaining risk, or "немає відомого">

Наступне: Team Lead закриває root-задачу.
```

Quality marks its child `done`. The Stage 2 barrier wakes Lead. Lead reads both
child results and either closes the root or creates one explicitly named
follow-up child. Never close the root only because a child has changed status.

## Boundaries

This template does not authorize deployment, production configuration or data
changes, IAM changes, secret changes, or an external write. For any such work,
stop and ask the human owner for approval before creating an execution child.

If work cannot continue, use `blocked` and name the owner plus the exact action
needed. Do not leave an in-progress issue silent.
