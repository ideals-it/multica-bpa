# Natural-language approval and task discussion

## Goal

Replace BPA's phrase-only production approval with an agent-mediated,
context-aware conversation while preserving a human gate for every
production-impacting task.

## Behaviour

1. A ticket remains in `in_review` while a production action is pending.
2. Every top-level comment from a workspace owner or admin on that ticket
   creates one continuation for the assigned agent. It is a conversation
   event, not a server-side approval decision.
3. The resumed agent reads the entire ticket and the new comment. It decides
   whether the human clearly approved the already described scope.
4. When approval is clear, the agent may execute only that scope. When it is
   ambiguous, conditional, a question, or a refusal, the agent must not make a
   production change; it replies concisely and, when needed, mentions the
   human owner for clarification.
5. A material title or description change invalidates a pending decision.
   After work for an approved scope starts, a material scope change is rejected
   and requires a follow-up ticket.

## Communication

- Agents may use comments to discuss the task, ask for details, surface
  blockers, and explain decisions.
- A real human mention is required when the agent needs a human decision or
  clarification. It must not be used for acknowledgement or routine progress.
- An agent-to-agent mention is permitted only for a concrete handoff; it must
  not be used as a conversational acknowledgement.
- Comments remain concise, formatted Markdown with normal paragraphs and no
  runtime/log noise.

## Safety boundary

The server no longer decides approval from a hard-coded list of words or
emoji. It guarantees that only owner/admin comments can wake a production
review continuation. The assigned AI agent makes the semantic interpretation
of the human's full reply and is accountable for failing closed on ambiguity.
The agent must record its interpretation in the ticket before a production
action so the decision remains auditable.

## Non-goals

- No board, status, or UI change.
- No new approval dialog.
- No automatic production action from a comment authored by an agent or an
  ordinary workspace member.
