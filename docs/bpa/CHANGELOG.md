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
