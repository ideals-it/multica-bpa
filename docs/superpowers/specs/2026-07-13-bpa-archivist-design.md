# BPA Archivist Design

## Goal

Give the BPA workspace a durable, concise index of every completed root task,
without adding UI, board statuses, or a second source of truth.

## Decision

Archivist is a separate, read-only specialist. Team Lead owns active work;
Archivist answers cross-project questions from Multica's existing issue,
comment, child, run, agent, project, and resource records.

The server maintains compact flat `bpa.knowledge_*` metadata on every BPA root
from its first material event until completion. Metadata is intentionally
primitive-only in native Multica, so it records a version, update time, latest
event, root status, and template. It does not duplicate child or discussion
content already available through native records.

A dedicated Archivist is automatically queued after a material root event. It
reads the current knowledge keys and the native root, child, comment, run,
agent, project and resource records, then updates only `bpa.archive_summary`
and `bpa.archive_updated_at`. The archive is an index, not a replacement for
the original data. Full descriptions, comments, run output, and agent history
stay in their existing tables and remain the source of truth.

For any task that changes a repository, its change-making agent creates a
focused local commit before its child can be accepted or its BPA root can close.
The delivery handoff records a commit hash; `no repo changes` is the only
alternative and must state why. GitHub Ops owns push, PR and merge operations.
Archivist reads and records commit and PR references but never creates them.

## Behavior

- Only a BPA root (`bpa.template` present, no parent) receives knowledge and
  archive records.
- The server refreshes `bpa.knowledge_*` on material root/child changes,
  comments, workflow metadata, and terminal task results. It does not create
  Archivist comments.
- Archivist receives at most one pending archive task per root. New material
  events merge into that pending task instead of creating a queue storm.
- Archivist can write only `bpa.archive_summary` and `bpa.archive_updated_at`;
  all other task, project, GitHub and production writes are denied.
- A code-changing child without a valid commit reference cannot be accepted.
  A non-code child records `bpa.no_repo_changes` and a concise reason instead.
- Root completion causes a final archive refresh; later re-saves do not create
  a new archive task unless a new material event changed the facts.
- A task cannot be closed while a BPA child is non-terminal. This keeps the
  archived record complete.
- No new board status, column, screen, comment, or agent trigger is created.
- Archivist has no write, routing, deployment, GitHub, or production authority.

## Metadata Contract

The server-owned facts use `bpa.knowledge_version`, `bpa.knowledge_updated_at`,
`bpa.knowledge_event`, `bpa.knowledge_status`, and `bpa.knowledge_template`.
Archivist's derived record uses
`bpa.archive_summary` and `bpa.archive_updated_at`.

```text
bpa.knowledge_version = 1
bpa.knowledge_updated_at = RFC3339 timestamp
bpa.knowledge_event = root_created | child_changed | root_changed | task_completed
bpa.knowledge_status = in_progress
bpa.knowledge_template = standard
bpa.archive_summary = Стан: ...\n\nРеалізація: ...\n\nCommit: ...
```

The root issue already stores title, description, project, assignee, comments,
and runs, so the snapshot deliberately does not duplicate them.

## Verification

Unit tests cover knowledge/archive serialization and terminal-child
recognition. Handler tests cover material-event upsert, one-pending-task
deduplication, Archivist namespace-only writes, final completion refresh, no
archive on a child, refusal to close a root with an open child, and refusal to
accept a code-changing child without a commit reference.

## Out of Scope

- a new archive table, search UI, label, or board column;
- a periodic watchdog for `waiting_local_directory`;
- any production, GitHub, or external-system action by Archivist.
