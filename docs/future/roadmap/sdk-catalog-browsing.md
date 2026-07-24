---
title: sdk catalog browsing
state: horizon
created: 2026-07-24
tags: [feature]
milestone: v0.1.x
---

A read-only catalog browse surface in `sdk/agent/catalog` — e.g. `ListVisible(ctx, a, filter) ([]Advertisement, error)` returning advertisements visible to the caller across orgs. Today `List` returns only the calling account's own advertisements. Small sibling follow-ons noted alongside it: a catalog `Update` helper (PATCH already exists on the controller) and `LookupWorkgroupByName`.
