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
