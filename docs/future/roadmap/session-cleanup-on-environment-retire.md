---
title: session cleanup on environment retire
state: horizon
created: 2026-07-24
tags: [defect]
milestone: v0.1.x
log:
  - stamp: 2026-07-24
    note: spec — docs/future/layer-2/session-cleanup-on-environment-retire.md
---

Retiring an environment raw-deprovisions+deletes its Layer 2 session tunnels but leaves the `sessions` row active with `tunnel_id` NULL — a dangling session with a stranded state. Full fix: tear down the sessions the retiring environment participates in during retire, not just the L1 tunnel. A Layer 2 deletion-lifecycle concern; spec carries the detail.
