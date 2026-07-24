---
title: session, contracts, envelopes public sdk types
state: horizon
created: 2026-07-24
tags: [enhancement, spike]
milestone: v0.1.x
---

Give `sdk/agent/session` (and contracts/envelopes) the same public-types treatment `sdk/agent/catalog` already got: external Go modules can't name `internal/api` types, so `Propose`/`Accept`/`Close`/`RegisterHandler` — which expose `*api.Session`, `*api.Envelope`, `*api.Contract` — are unreachable from outside the repo. Apply the catalog pattern (public types + boundary translation) when a driving external consumer for L2 sessions appears.
