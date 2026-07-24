---
title: single-active-host enforcement
state: horizon
created: 2026-07-24
tags: [feature, spike]
milestone: v0.1.x
log:
  - stamp: 2026-07-24
    note: spec — docs/future/single-active-listen.md
---

Guarantee exactly one live host per tunnel — so two healthy environments can't both serve and split traffic — plus the automatic failover that falls out of it. Account-owned tunnels shipped the manual, on-demand slice (`--takeover` / `tunnel takeover`); this is the continuous, controller-directed enforcement. The fabric can't refuse a second host, so it reaches into the thin-listener design. Spec carries the weight.
