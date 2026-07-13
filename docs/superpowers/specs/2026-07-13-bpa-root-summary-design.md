# BPA root summary design

## Goal

Make the BPA root ticket the human-readable source of truth. A person should
understand the delivered result without opening agent handoff or child tickets.

## Scope

- No UI, board, status, or new-ticket changes.
- Applies only to BPA root tickets coordinated by the root Team Lead.
- Keeps detailed implementation evidence in existing agent comments and task
  runs; the root contains short plain-language summaries.

## Options considered

1. Prompt only: cheapest, but repeats the failure in BPA-176 when an agent
   finishes without following its prompt.
2. Server-generated summary: reliable but produces technical and potentially
   misleading prose because the server cannot interpret a change safely.
3. Hybrid (chosen): Team Lead writes summaries; server prevents closing a BPA
   root until its final human summary exists. Existing handoff/mention routing
   wakes Lead after material work, while templates make the root summary part
   of the normal loop.

## Contract

For a single-ticket BPA flow, Team Lead posts a short root update after each
material specialist or Quality result. It says only what changed in plain
language and who acts next; it does not copy logs, commands, or full child
comments.

Before moving a BPA root to `Done`, Team Lead posts a final root comment with
these separate Markdown paragraphs:

```text
**Що було не так:** <user-visible cause>

**Що змінили:** <plain-language change or no-change conclusion>

**Що перевірили:** <concise evidence>

**Результат:** <current outcome>

**Ризик / наступне:** <remaining risk, or no known risk>
```

The final comment is authored by the root Team Lead and must be newer than the
latest material specialist/Quality handoff. `In Review` uses the same concise
summary plus the existing approval fields; approval is not duplicated.

## Server behavior

When a BPA root is changed to `Done`, the server checks its recent comments.
If no qualifying final root summary exists, it rejects the transition with a
clear message naming the missing summary. It does not generate text or create
another ticket.

Existing child completion and blocked handoff behavior remains unchanged. In
the new default workflow, those handoffs happen on the root through explicit
mentions, so Lead is responsible for mirroring the meaningful outcome there.

## Current BPA-176 repair

Add one concise final root summary after the successful deployment:

- cause: temporary Google API failures exhausted retries and fell through to
  the generic 500 handler;
- change: those transient statuses/timeouts now produce 503 with
  `Retry-After` after retries;
- evidence: 14 automated tests and Cloud Run revision 00017 ready at 100%
  traffic;
- remaining risk: external Google API incidents can still occur, but now have
  the correct transient response.

Then reconcile the obsolete staged children to their factual terminal statuses
without creating new work.

## Tests

- root `Done` without a qualifying Lead final summary is rejected;
- a summary missing any required heading is rejected;
- a qualifying newer Lead summary allows `Done`;
- a specialist/Quality comment alone cannot satisfy the gate;
- non-BPA roots and non-`Done` transitions retain current behavior.
