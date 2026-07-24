---
title: connect kernel-assigned ports
state: horizon
created: 2026-07-24
tags: [enhancement, spike]
milestone: v0.2.x
---

Support kernel-assigned connect ports (`…:0`) for `tunnel.EnsureConnected` / managed connect. Blocked today because the runtime status snapshot serializes the *desired* listen address, not the post-bind resolved one — so a caller can't learn which port was chosen. Needs a `ResolvedListenAddress` on the connect status plus runtime state to capture it after bind.
