# BPA fork changelog

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
