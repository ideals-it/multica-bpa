---
name: multica-bpa-production-workflow
description: "Use when Team Lead coordinates a production-impacting task with one explicit human approval."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA production workflow

Use only when the root can change production, data, configuration, IAM,
secrets, deploy a revision, or make an external write. Keep preparation,
Quality, approval, execution, and postflight on **one root issue** unless work
has independently deliverable parallel streams.

```text
Lead → specialist → Quality → Lead → human approval → specialist → Quality → Lead
```

## Status contract

- `In Progress` while preparation, execution, or Quality work is active.
- `In Review` only while **Vitaliy Ustymenko** must approve or reject the
  current production scope.
- `Done` after the approved action and required postflight are accepted.
- `Blocked` with a named missing decision or input when work cannot continue.

## Same-root flow

1. Lead moves the root to `In Progress` and mentions the preparation
   specialist on the root. Preparation may investigate, test locally, prepare
   a reversible plan, or make a focused local commit. It never changes
   production. An action owner always uses a clickable agent mention, for
   example `[@AT Builder](mention://agent/<agent-id>)`.
2. The specialist posts one result on the root and mentions Team Lead. Lead
   mentions Quality on that same root; Quality checks the plan/evidence and
   returns one verdict to Lead.
3. Only after Quality accepts, Lead moves the root to `In Review` and posts a
   concise approval request for **[@Vitaliy Ustymenko](mention://member/7c237dcc-c29c-4c66-8ed0-bbae2339e58e)**.
   For an untemplated agent-owned root, that transition initializes the native
   Production contract and records one pending ticket scope automatically:

```text
**Дія:** <what will be done>

**Вплив:** <service or data effect>

**Відкат:** <how to reverse it>

**Ризик:** <real residual risk>
```

4. A matching human approval is recorded server-side for the current ticket
   scope. Lead resumes the same root, moves it back to `In Progress` when
   execution starts, and mentions the execution specialist. The server blocks
   unapproved runs. Do not request approval again for an unchanged approved
   title and description; a new approval is needed only after that scope
   changes. Ask Vitaliy for a clear standalone confirmation such as
   `Погоджую`, `Деплой`, `Роби`, or `Виконуй`. An agent mention may accompany
   that directive; a question or a negation is not approval. On the Lead's
   root-level approval comment, Vitaliy may instead react with 👍 or 👌. Moving
   the card manually is not approval.
5. The execution specialist and Quality post their result/postflight on the
   root. Lead closes it as `Done` or records the exact blocker.

## Root summary before Done

After each material handoff, Lead posts a short plain-language progress update
on the root before mentioning the next owner. Before `Done`, Lead writes this
final root summary. Do not paste logs, commands, child comments, or a task
transcript:

```text
**Що було не так:** <user-visible cause>

**Що змінили:** <plain-language change>

**Що перевірили:** <concise evidence and postflight>

**Результат:** <approved action outcome>

**Ризик / наступне:** <remaining risk or "немає відомого">
```

Child issues are optional only for independent parallel deliverables. Do not
create a child merely to transfer work between Lead, specialist, Quality, or
GitHub Ops.

The agent that changes a repository creates a focused local commit before its
handoff. GitHub Ops is mentioned on the same root only for an approved
publication action. n8n Prod is the specialist for n8n production work.

The backend contract is mapped in
`references/production-workflow-source-map.md`.
