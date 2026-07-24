---
title: session teardown on tenant deletion
state: horizon
created: 2026-07-24
tags: [feature]
milestone: v0.1.x
log:
  - stamp: 2026-07-24
    note: spec — docs/future/layer-2/session-teardown-on-tenant-deletion.md
---

Full teardown of the Layer 2 sessions (and the provider-owned backing tunnels of consumed sessions) that a tenant participates in, so account/org deletion can *proceed* instead of being refused. The interim guards shipped (account delete refuses while it participates in an active session or owns a standalone tunnel; org delete refuses while accounts remain). This is the deferred cascade that replaces the refusal — needs the session state machine, not just the L1 cleanup matrix. Spec carries the detail.
