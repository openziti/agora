---
title: orphan dial policy gc
state: horizon
created: 2026-07-24
tags: [enhancement]
milestone: v0.1.x
---

A periodic tag-sweep GC backstop that removes any Ziti policy left with no backing DB row (residue from a prior failed write). Revocation today is row-enumeration-driven (deprovision each policy by its stored id), so this is belt-and-suspenders, not the load-bearing path. Must be exhaustive — a one-shot tag delete stops at the `Find` page limit (defaults to 100), so it has to repeat-until-empty.
