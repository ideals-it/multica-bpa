---
name: multica-bpa-investigation-workflow
description: "Use for evidence-first, read-only diagnosis before deciding whether a separate fix task is needed."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA investigation workflow

Keep a focused investigation in **one root issue**:

```text
Lead → Investigator → Quality → Lead
```

Lead moves the root to `In Progress` and mentions the Investigator on the root
with a clickable action mention such as
`**[@AT Builder](mention://agent/<agent-id>)**`.
The Investigator is read-only against logs, code, configuration, and run
history. It posts one evidence report on the root and mentions Team Lead.
Lead then mentions Quality on the same root to test the evidence, not a fix.

```text
**Симптом:** <confirmed fact>

**Докази:** <links or exact observations>

**Гіпотези:** <tested causes and result>

**Висновок:** <confirmed cause or "недостатньо даних">

**Наступне:** <bold owner and exact action>
```

Quality returns one verdict to Lead. Lead closes the root as `Done` when the
answer is evidenced, or sets it `Blocked` with the exact missing data. If a fix
is needed, Lead creates a separate Standard or Production root task; an
investigation не створює implementation-child.

Child issues are optional only for independent evidence streams that need their
own readable deliverable. Do not create one for the Investigator or Quality
handoff.
