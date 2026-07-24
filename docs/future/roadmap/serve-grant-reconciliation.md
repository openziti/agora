---
title: serve grant reconciliation
state: horizon
created: 2026-07-24
tags: [enhancement, spike]
milestone: v0.1.x
---

`tunnel.EnsureServed` is additive on `GrantEmails` — it adds entries but never revokes, so a grant dropped from a later call survives. Full reconciliation (list existing → diff against spec → add new, revoke removed) needs a revoke primitive on the controller/runtime path. Operators revoke via the agora CLI in the meantime.
