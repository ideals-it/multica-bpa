# BPA Multica operating guide

This document is the current operating contract for the local BPA Multica
fork. It describes the small BPA-specific layer on top of upstream Multica;
upstream product documentation and the repository README remain the source for
general installation and platform capabilities.

## Purpose

Use Multica as a task board and durable history for AI-assisted work. A ticket
normally has one accountable executor, not a multi-agent assembly line. The
board reflects the real state of that work, while comments retain the
human-readable context and decisions.

The fork deliberately keeps the existing Multica UI, board columns, and status
set. BPA behavior is added through focused server guards, live agent contracts,
and reusable skills so upstream updates remain manageable.

## Active roles

| Agent | Responsibility | Boundary |
| --- | --- | --- |
| `AT Codex` | Full lifecycle for ordinary BPA tasks: analysis, implementation, verification, commits, and any natural push/PR work. | Production-impacting work needs the ticket-level review flow. |
| `AT EventCatalog` | Service and event-catalog documentation. Builds the catalog after every article change and commits the result. | Cloud Run deployment needs ticket-level review. |
| `AT n8n Prod` | n8n production work on Google Compute Engine: diagnosis, upgrade, rollback, backup/restore, and worker topology. | Deployments and other irreversible production actions need ticket-level review. |
| `AT Archivist` | Read-only BPA knowledge agent for evidence, history, commits, decisions, and current work. | Cannot change tickets, repositories, production, IAM, secrets, agents, or queues. |

The retired Team Lead, Builder, Quality, Reviewer, and GitHub Ops choreography
is not an active workflow. Historical tickets keep that history, but new work
must not recreate those handoffs without a genuinely independent deliverable.

## Ticket lifecycle

| Status | Meaning |
| --- | --- |
| `Backlog` | Parked work. An Autopilot-created ticket is assigned but does not start until a human promotes or explicitly starts it. |
| `Todo` / `In Progress` | Work is ready or actively being performed. |
| `In Review` | A specific decision from Vitaliy is required, normally before a production-impacting action. |
| `Blocked` | A concrete missing input, access right, tool, or external dependency prevents progress. |
| `Done` | The requested outcome is complete and proportionately verified. |

Keep one clear task in its root ticket. Create a child only for an independent,
separately useful result, such as parallel work in another repository or an
independent investigation. Do not create children for internal phases, review,
Git, a handoff, or a summary.

## Production approval and verification

A production deployment, production-data change, IAM or secret change, or
other irreversible action stays in the current ticket. The assigned agent
prepares the work, moves the ticket to `In Review`, and uses a clickable
mention for Vitaliy with a concise explanation of the action, scope, risk, and
rollback.

The agent reads the complete human reply in ticket context. Approval is natural
language, not a fixed keyword or an isolated control. A clear approval resumes
only the already described scope. A conditional, ambiguous, or changed scope
requires one focused follow-up question and a new review when appropriate.

`Done` requires behavioral verification. A configuration readback, revision
check, or `/health` response only proves that a configuration was applied; it
does not prove the requested behavior. If the only meaningful verification is a
bounded production write, the ticket remains in `In Review` until Vitaliy
approves that controlled run. The agent must state its scope, safeguards,
expected evidence, and rollback before asking.

## Comments and collaboration

The shared `codex-soul-global-rules` skill is assigned to every active agent
and is the single source of truth for visible ticket communication.

- Visible comments, questions, progress updates, and final results are concise
  Ukrainian Markdown.
- Use natural paragraphs, not fixed `Result`, `Check`, `Risk`, or `Next`
  sections. The server also removes these retired labels from BPA agent
  comments before storage.
- Use `code` for exact identifiers, paths, commands, and errors. Use bold only
  for material emphasis, never for names.
- Mention Vitaliy only when a specific answer or decision is needed. Mention
  another agent only for a concrete delegated task.
- For work longer than a meaningful stage, leave one short progress update
  every 3–5 minutes without raw logs, command transcripts, JSON, or internal
  handoff noise.

## Skills and tools

`using-superpowers` is assigned to all active agents and is mandatory before
any response or action. The agent must then identify and invoke every relevant
skill before it inspects, plans, clarifies, or executes work.

`AT Codex`, `AT EventCatalog`, and `AT n8n Prod` receive their configured MCP
tools in their runtime sessions. They may use only tools that are actually
available and necessary for the assigned scope. If a required tool is missing
or fails, the agent must state the concrete blocker and its impact in the
ticket; it must never invent a tool result, access, verification, or external
state.

`AT Archivist` is intentionally read-only and has no external MCP capability.
It relies on native Multica history and evidence instead.

The `caveman` skill is not assigned by default. It is an explicit-request
response-style mode, not an execution or safety control, and would reduce
clarity in normal ticket work.

## Autopilots

Configured Autopilots create work only in `Backlog` when they create a ticket;
they do not silently start it. As of 2026-07-14, the active recurring flows
are:

- `n8n Stable Version Check`: creates one deduplicated `Backlog` ticket for a
  newly available stable n8n release; production deployment remains subject to
  review.
- `EventCatalog Weekly Deploy Reminder`: creates a `Backlog` reminder for
  review and approval of accumulated catalog changes.
- `EventCatalog Daily Significance Triage`: creates only justified,
  deduplicated `Backlog` documentation tickets for verified service changes.
- `Multica Doctor Daily Audit`: report-only health audit; it does not create or
  update tickets or live configuration.

`EventCatalog Daily Build Check` is paused. EventCatalog builds after each
article change instead.

Use the live state, not this list alone, when changing or debugging an
Autopilot:

```bash
multica --profile selfhost-local autopilot list --output json
multica --profile selfhost-local autopilot get <autopilot-id> --output json
multica --profile selfhost-local autopilot runs <autopilot-id> --output json
```

## Local operation and verification

The local CLI profile for this fork is `selfhost-local`. It targets
`http://localhost:8080` and the local workspace. Use it for live inspection:

```bash
multica --profile selfhost-local agent list --output json
multica --profile selfhost-local skill list --output json
multica --profile selfhost-local issue list --output json
```

The local Docker backend can be verified with:

```bash
curl --fail --silent --show-error http://localhost:8080/health
```

For source changes, run the narrowest relevant tests first and then broader
checks in proportion to the risk. Do not infer local runtime, production, or
secret state from documentation alone.

## Change records and historical material

[CHANGELOG.md](CHANGELOG.md) records BPA fork changes. Historical plans and
specifications under `docs/superpowers/` record how earlier designs evolved;
they are not the current operating contract. When they conflict with this
guide, this guide and the live agent/skill configuration take precedence.
