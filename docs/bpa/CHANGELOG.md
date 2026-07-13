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
