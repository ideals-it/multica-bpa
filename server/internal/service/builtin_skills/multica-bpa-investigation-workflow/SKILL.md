---
name: multica-bpa-investigation-workflow
description: "Use for evidence-first, read-only diagnosis before deciding whether a separate fix task is needed."
user-invocable: false
allowed-tools: Bash(multica *)
---

# BPA investigation workflow

```text
Lead → Investigator → Quality → Lead
```

Lead owns the root. Investigator works read-only against logs, code,
configuration, and run history. Quality checks the evidence, not a fix.

The Investigator posts one result:

```text
Симптом: <підтверджений факт>

Докази: <посилання або точні спостереження>

Гіпотези: <перевірені причини та результат перевірки>

Висновок: <підтверджена причина або "недостатньо даних">

Наступне: Team Lead створює окрему задачу або блокує з потрібними даними.
```

Investigation is read-only: it does not deploy, change production data, edit
configuration, and не створює implementation-child. If a fix is needed, Lead
creates a new Standard or Production root task after Quality accepts the
evidence. If evidence is insufficient, set the root to `blocked` and name the
exact missing data.

If the Investigator needs a decision before evidence is complete, it marks its
child `blocked`; Multica wakes Lead on the root. It never routes work directly
to another specialist.
