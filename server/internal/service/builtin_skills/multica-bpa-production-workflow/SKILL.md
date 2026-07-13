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
a reversible plan. Кожен агент не виконує production-дію.

Use the normal staged children: specialist in Stage 1, Quality in Stage 2. Do
not use tags, scheduler jobs, duplicate children, or progress-only comments to
move the work.

Each completed child has один короткий коментар:

```text
Результат: <готовий результат>

Перевірка: <що перевірено>

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

When the task shows **Потрібне погодження**, Lead stops. No agent tags another
agent, starts an execution child, deploys, or changes production while this
state is pending. A changed plan requires a new approval.

Only after the human owner explicitly approves the current plan may Lead create
the single execution child. The specialist reports the result, Quality checks
it when applicable, and Lead closes the root with one short summary.

If approval is rejected or work is unsafe, set the root to `blocked` and name
the exact decision or input needed. Do not leave an in-progress task silent.

The backend contract is mapped in
`references/production-workflow-source-map.md`.
