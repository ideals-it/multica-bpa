# BPA continuation and local artifact links

## Goals

1. A completed specialist run on a single-ticket BPA workflow must wake the
   assigned Team Lead without relying on agent-authored mention syntax.
2. A Desktop user can open an agent-reported local artifact or its directory
   from a comment without allowing arbitrary filesystem access.

## Lead continuation

At daemon task completion, the server will identify a completed specialist run
on a root issue whose assignee is a different agent. This covers the existing
single-ticket Lead → specialist → Lead pattern without depending on BPA
metadata or the text of a comment. It will not wake for chat tasks, child
issues, the root assignee's own run, an active sibling specialist run, or a
root that is already terminal.

When the handoff is eligible, the server queues the root assignee through the
existing issue-task queue and its pending-task deduplication. It does not add a
comment, create a child ticket, or change a board status. The Lead decides the
next status and whether human approval is needed from the evidence already on
the root.

## Local artifact links

An agent comment may use an opaque `local-artifact://` link that includes the
comment source task ID, an action (`open` or `reveal`), and a relative artifact
path. The visible label is the full local path, so the person sees exactly
which file will open. The server authorizes the task in the current workspace
and returns that task's recorded `work_dir` plus the validated relative path.

Only Desktop makes the request through a narrow preload IPC API. Its main
process resolves the returned target with real paths and rejects absolute
paths, traversal outside `work_dir`, missing files, and symlinks that escape
the work directory. `open` uses the OS default application; `reveal` selects
the file in its containing folder.

Web and mobile render the label as non-clickable text. No `file://` URL is
allowed, and no arbitrary path reaches Electron shell APIs.

## Constraints

- No board, status, or layout change.
- Existing HTTP/HTTPS and mention link behavior remains unchanged.
- The server and Desktop main process are both authorization boundaries:
  the server proves task/workspace ownership, and Desktop proves filesystem
  containment on the local machine.
- Missing files and paths outside registered workspace repositories return a
  clear error and do not reveal filesystem structure.
