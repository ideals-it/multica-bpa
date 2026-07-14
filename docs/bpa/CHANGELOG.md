# BPA fork changelog

## 2026-07-14 — Legacy production-review recovery

An owner or admin comment on a legacy pending production-review ticket now
repairs the missing review marker and reaches the assigned agent. The agent,
rather than a server keyword parser, reads the complete conversation and
decides whether the human message clearly approves the already bounded scope.
Only then can that exact task resume the ticket; an ordinary question or
ambiguous reply keeps it in `In Review`.

No UI, board, predefined status, production service, IAM, secret, or remote
deployment behavior changed.

## 2026-07-14 — Simple task-board agent contracts

The active BPA workspace now uses four roles only: **AT Codex**, **AT
EventCatalog**, **AT n8n Prod**, and read-only **AT Archivist**. Historical
Lead → Builder → Quality, GitHub Ops, and role-handoff behavior below is kept
only as migration history and is not an active contract.

One shared BPA skill owns scope, evidence, secret handling, commit discipline,
verification, and visible-comment style. Agent instructions now contain only
their role, status semantics, and production boundary. Visible comments use
short natural Markdown prose without fixed `Результат / Перевірка / Ризик /
Наступне` sections; production approval remains the same-ticket `In Review`
decision with a real mention and a natural-language human response.

EventCatalog no longer has a daily build-check workflow: it builds after each
article change. Daily significance triage and the weekly deploy reminder create
assigned `Backlog` tickets only. n8n production work follows the same
ticket-level `In Review` approval rule. No UI, board column, predefined status,
production service, IAM, secret, or remote deployment changed in this contract
cleanup.

## 2026-07-13 — Archivist is fully background

Automatic BPA events now update only small server-owned knowledge markers. They
never queue an Archivist AI run, use a local runtime/directory, change a ticket
status, or wake Team Lead. A detailed archive summary remains available only
when Archivist is explicitly assigned or mentioned for that purpose.

No UI, board, predefined status, production service, IAM, secret, or remote
deployment behavior changed.

## 2026-07-13 — Archivist yields to delivery and cannot wake Lead

Archivist no longer queues or occupies the shared local project directory while
Lead or a delivery specialist has an active task on the same BPA root. Comment
triggering now happens before the archive scheduling check, so a new agent run
is visible to that check. The Archivist refresh is queued only after the
delivery chain becomes idle. Archivist completion is never treated as a
specialist handoff, so it cannot start a Lead → Archivist loop.

No UI, board, predefined status, production service, IAM, secret, or remote
deployment behavior changed.

## 2026-07-13 — Emoji approval for Production review

On the assigned Lead's root-level Production approval comment, a member's 👍
or 👌 now records the current ticket scope as approved and queues Lead to
resume the same ticket. ❤️, ✅, 👎, reactions on other comments, and all agent
reactions remain ordinary reactions with no approval effect.

No UI, board, predefined status, production service, IAM, secret, or remote
deployment behavior changed.

## 2026-07-13 — Visible specialist handoff to Lead

For a task-scoped specialist comment on an agent-owned main ticket, the server now
adds one compact `Наступне` mention to the assigned Lead when the specialist
did not name any next owner. An explicit agent, squad, or member mention is
preserved unchanged. This keeps the readable handoff and the wake-up target in
the same comment without creating a child ticket or an extra system comment.
The rule applies even before a BPA template is started.

No UI, board, predefined status, production service, IAM, secret, or remote
deployment behavior changed.

## 2026-07-13 — Native approval for agent-created In Review

An agent moving an untemplated, agent-owned main ticket to `In Review` now
starts the native Production approval contract automatically. The server stores
one pending fingerprint for the ticket title and description, then records a
matching human approval directly against that scope. An already approved,
unchanged scope remains approved when an agent later updates its status, so a
repeat approval comment cannot create an approval loop. Starting the Production
template while a ticket is already `In Review` has the same behavior.

Verified with focused live-PostgreSQL handler tests. No UI, board, predefined
status, production service, IAM, secret, or remote deployment behavior changed.

Pending Production review can no longer be moved manually to another status:
the ticket remains visibly `In Review` until a member writes a clear approval
directive. The parser accepts contextual confirmations such as `Погоджую`,
`Деплой`, `Роби`, or `Виконуй`, including after a Lead mention, but rejects
questions and negations.

## 2026-07-13 — Lead wake-up after concurrent specialist result

Fixed a root-ticket race where a specialist could finish while the assigned
Lead was already running. The server still avoids concurrent duplicate Lead
tasks, but when that Lead completes it now checks for specialist evidence that
arrived during its run and queues one normal Lead follow-up. This prevents a
ticket from remaining `In Progress` without a next agent after a valid handoff.

Verified with a focused live-PostgreSQL handler regression test and a local
backend health check. No UI, board, predefined status, production service,
IAM, secret, or remote deployment behavior changed.

## 2026-07-13 — Plain agent mentions

Removed bold formatting around actionable agent mentions in the Standard,
Investigation, and Production contracts. Agent names remain plain text or a
plain clickable `mention://agent/...` link; bold is reserved for comment
labels. The same rule was applied to the live BPA agents.

## 2026-07-13 — Reliable root handoff and local artifact links

- A completed specialist run on a root ticket now queues its assigned Lead
  after the final parallel specialist finishes. The queue uses normal task
  deduplication and adds no technical system comment.
- Desktop comments can open or reveal a task-produced artifact using a
  task-bound `local-artifact://` link. The server checks task/workspace access;
  Desktop blocks traversal and symlink escape before the OS receives a path.

No board, predefined status, layout, production service, IAM, secret, or
deployment behavior changed.

## 2026-07-13 — Active handoff status guard

The server now rejects an agent attempt to set a ticket to `Blocked` while a
different agent task on that ticket is still `queued`, `dispatched`,
`waiting_local_directory`, or `running`. A member may still block a ticket,
and an agent may still do so when no delegated task is active.

This keeps `waiting_local_directory` as a normal local-directory mutex state,
not a false workflow blocker. No UI, board column, predefined status,
production service, IAM, secret, or deployment behavior was changed.

## 2026-07-13 — Lead-owned main-ticket summary

Kept the existing UI, board columns, and statuses unchanged while making the
main BPA ticket the readable source of truth:

- after every material specialist or Quality handoff, **AT Team Lead** posts a
  short plain-language progress update on the main ticket before routing the
  next owner;
- before `Done`, the assigned Lead must publish five separate paragraphs:
  `Що було не так`, `Що змінили`, `Що перевірили`, `Результат`, and
  `Ризик / наступне`;
- the server rejects completion of a BPA main ticket until that Lead-authored
  summary exists, regardless of whether the update uses the normal issue API,
  batch update, or GitHub merge completion;
- raw logs, commands, and child-ticket transcripts remain out of the main
  summary.

Verified by focused handler and built-in-skill regression tests. No UI, board
column, predefined status, production service, IAM, secret, or deployment
behavior was changed.

## 2026-07-13 — Single-ticket BPA execution and truthful board state

Simplified the BPA templates without changing the UI, board columns, or
predefined statuses:

- Standard, Production, and Investigation now use one root ticket by default:
  Lead → specialist → Quality → Lead;
- a child ticket is allowed only for independently useful parallel work or a
  separately readable deliverable, never merely for a role handoff;
- a queued agent task moves `Todo` or `Backlog` to `In Progress` only after the
  task is persisted; `In Review`, `Done`, and `Blocked` are preserved;
- `In Review` is reserved for a real current decision by Vitaliy, not a future
  safety guardrail such as “do not deploy without approval”;
- live instructions for **AT Team Lead**, **AT Builder**, **AT Quality**,
  **AT Documentation Writer**, **AT n8n Prod**, and **AT GitHub Ops** now use
  same-ticket handoffs with explicit mentions.

Verified with focused regression tests and the complete Go backend test suite.
No UI, board/status definition, remote deployment, production service, data,
IAM, or secret was changed.

## 2026-07-13 — Review hardening for BPA dispatch and authority

Closed the independent review findings without changing the UI, board, or
predefined statuses:

- every child dispatch now reads the BPA state from its main task, so reruns,
  mentions, and assignment cannot bypass Production `In Review` approval;
- before approval, only the actual root Team Lead may prepare a Production
  plan; specialists cannot be woken through a root mention;
- the `In Review` transition writes all approval-pending metadata atomically;
- Team Lead ownership is enforced for BPA child creation, routing,
  reassignment, reparenting, and root closure, including batch updates;
- `AT Archivist` is read-only across issue creation, update, deletion, batch
  updates, and comments;
- commit evidence now requires a valid Git SHA, while no-change evidence needs
  a substantive explanation;
- Autopilot's once-per-day guard is based on the planned occurrence rather
  than the time a run happens to finish.

Follow-up independent review found and closed three deeper paths:

- Archivist now uses the normal Production dispatch gate, so its read-only run
  cannot start while a ticket awaits human approval;
- BPA root resolution traverses the complete issue ancestry with cycle guards,
  covering nested child tasks without imposing a new depth limit on non-BPA
  work;
- dispatch recomputes ticket scope from the current root title and description,
  fail-closing if a write could not persist the matching approval metadata;
- Archivist read-only enforcement also covers generic metadata writes/deletes.

Verified with the complete Go backend test suite on the local test database.
No production service, data, configuration, IAM, secret, UI, or deployment was
changed.

## 2026-07-13 — Local runtime Autopilot recovery

Added opt-in recovery for scheduled `run_only` Autopilots when a local runtime
was offline. Recovery is bounded, limited to one successful run per Autopilot
per scheduled day, and does not change board or UI behaviour.

## 2026-07-13 — Standard workflow template

Added the first fork-specific workflow asset:

- built-in `multica-bpa-standard-workflow` skill;
- native `Lead → specialist → Quality → Lead` flow using existing child stages;
- one specialist in Stage 1 and a parked Quality child in Stage 2;
- concise Ukrainian four-section handoff comments only at task completion;
- explicit boundary: no production, deployment, IAM, secret, or external-write action.

No scheduler, workflow engine, custom status, comment event protocol, runtime,
or production configuration was added or changed.

## 2026-07-13 — Isolated live routing verification

Verified the template on the fork's local backend and isolated database:

- Lead, specialist, and Quality tasks were dispatched by the local runtime;
- Stage 1 completion woke the root Lead task;
- Stage 2 remained parked until explicitly promoted to `todo`;
- Stage 2 completion woke Lead again and the root task closed as `done`.

No production service, data, configuration, IAM, or secret was touched.

## 2026-07-13 — System handoff comment formatting

Replaced verbose English child-stage system comments with concise Ukrainian
sections separated by blank lines: `Результат`, `Стан` (for staged work), and
`Наступне`. The stage barrier, parent wake, mentions, and dispatch behavior are
unchanged.

Verified live on the isolated fork backend with a root and Stage 1 child. No
agent, production service, data, configuration, IAM, or secret was touched.

## 2026-07-13 — Production approval policy foundation

Added a pure BPA policy layer based on the `Safe Outputs` gate pattern from
GitHub Agentic Workflows: deny an explicitly marked production action until a
human approves the exact plan fingerprint; a changed plan invalidates approval.
This policy does not introduce a workflow engine or dispatch work itself.

The full role and evidence template is adapted from the public Routa workflow:
Team Lead corresponds to its coordinator, specialist roles to its implementor,
and Quality to its independent gate. Multica's native stages remain the
orchestration primitive.

## 2026-07-13 — Production workflow API and dispatch gate

Added local BPA endpoints to start a template, request an approval, read its
state, and record a human decision. Direct assignment, mention-triggered work,
and rerun now check the same fail-closed policy before creating or cancelling a
task. No production action was dispatched.

## 2026-07-13 — Human approval card and role contract

Added a small issue-detail card for a pending production decision. It shows
only the Lead's short Ukrainian summary and explicit `Погодити` / `Відхилити`
actions; plan fingerprints and raw workflow metadata stay hidden.

Added `multica-bpa-production-workflow`, a Lead-centred contract adapted from
the Routa Coordinator → implementor → Gate pattern. It keeps native Multica
stages, requires one concise child handoff, and prevents agents from executing
a production action before the human decision.

## 2026-07-13 — Native review approval and investigation template

Removed the fork-specific approval card. BPA keeps the existing board and
statuses: a Production root enters native `In Review`, and a human `Approve` or
`Погоджую` comment approves the current title-and-description scope. Editing
that scope resets approval. No production command is approved separately.

Added a read-only Investigation template and explicit routing rules: normal
child completion wakes Team Lead through native stages; a blocked child wakes
Team Lead immediately. Specialists do not route work to each other. System
handoffs and role templates use concise Ukrainian paragraphs.

## 2026-07-13 — Stable Lead routing, commit gate, and autonomous Archivist

Moved the remaining BPA guarantees into server behavior:

- a BPA root cannot close while a child remains open;
- every completed BPA child records either its focused local commit SHA or a
  concise `no repo changes` reason;
- child completion and `blocked` transitions create one native system handoff,
  mention the root assignee, and wake Team Lead without agent-authored routing
  comments;
- the server queues one deduplicated read-only Archivist refresh after material
  issue, comment, workflow, metadata, and agent-run events;
- `AT Archivist` is discovered automatically per workspace, while a human may
  configure another Archivist explicitly;
- only the configured Archivist can update `bpa.archive_summary` and
  `bpa.archive_updated_at`; server knowledge and archive records cannot be
  deleted through the metadata API;
- Archivist completion output remains in task-run history and is not copied
  into issue comments, keeping the board discussion free of technical noise.

No UI, board column, predefined status, production service, IAM, secret, or
deployment behavior was changed.

## 2026-07-13 — Independent review hardening

Closed the server-side bypasses found by an independent review:

- removed legacy BPA approval API routes and the legacy plan-fingerprint
  dispatch path; only native `In Review` plus a human approval comment can
  approve a Production ticket scope;
- production children cannot dispatch before that ticket-scope approval; the
  root Lead remains able to prepare the plan;
- generic metadata cannot write or delete BPA workflow state; only an assigned
  BPA child agent may record commit evidence, and only Archivist may update its
  two archive keys;
- one completion guard now covers direct issue updates, batch updates, and
  GitHub merge completion;
- the configured Archivist cannot create issue comments.

The stage barrier remains intentional: parallel child completion wakes Lead
when its stage closes, while a `blocked` child wakes Lead immediately. No UI,
board/status, production, IAM, secret, or deployment behavior changed.
