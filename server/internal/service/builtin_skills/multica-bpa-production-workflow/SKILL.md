---
name: multica-bpa-production-workflow
description: "Use when Team Lead coordinates a production-impacting task through preparation, Quality, and one explicit human approval."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA production workflow

Use only when the root task can change production, data, configuration, IAM,
secrets, deploy a revision, or make an external write.

```text
Lead → specialist → Quality → Lead
```

Team Lead owns the root task. Specialists own one child result. Quality checks
the stated criterion independently. Lead writes the only root-level summary.

## Prepare safely

The first children may investigate, prepare a change, test locally, or prepare
a reversible plan. На цьому етапі жоден агент не виконує production-дію.

Use the normal staged children: specialist in Stage 1, Quality in Stage 2. Do
not use tags, scheduler jobs, duplicate children, or progress-only comments to
move the work.

Each completed child has один короткий коментар:

```text
Результат: <готовий результат>

Перевірка: <що перевірено>

Commit: <SHA, або "no repo changes: <причина>">

Ризик: <залишковий ризик>

Наступне: Team Lead виконує наступний етап.
```

## Human approval

After Quality accepts the preparation, Team Lead creates one concise approval
request. Write it for the human owner, not for an engineer:

```text
Дія: <що буде зроблено>

Вплив: <що зміниться для сервісу або даних>

Відкат: <як повернути попередній стан>

Ризик: <короткий реальний ризик>
```

Lead moves the root ticket to **In Review** and posts **Потрібне погодження**.
The human replies with `Approve` or `Погоджую`. No agent starts an execution child,
deploys, or changes production before that reply. A changed ticket scope
requires a new approval.

Only after the human owner explicitly approves the current ticket scope may
Lead create the single execution child. The specialist reports the result,
Quality always performs a postflight check, and Lead closes the root with one
short summary.

The agent that changes a repository makes its own focused local commit before
handoff. If the approved task needs publication, Lead creates a separate
GitHub Ops child for push, PR, or merge. GitHub Ops never decides scope or
approval itself.

n8n Prod is the execution specialist for approved n8n production work.
EventCatalog documents the resulting service or event-contract change only
when Lead includes that deliverable in the ticket scope.

If approval is rejected or work is unsafe, set the root to `blocked` and name
the exact decision or input needed. Do not leave an in-progress task silent.

For a blocker before approval or during execution, mark the child `blocked`.
Multica wakes Lead on the root. Do not tag a peer agent to transfer work; Lead
owns all routing.

The backend contract is mapped in
`references/production-workflow-source-map.md`.
