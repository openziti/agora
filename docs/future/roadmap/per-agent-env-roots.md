---
title: per-agent env roots
state: horizon
created: 2026-07-24
tags: [enhancement]
milestone: v0.2.x
---

An `environment.LoadRootAt(path)` so `NewStandalone` (and SDK consumers) can honor a per-agent `EnvRoot` without mutating the process-global `environment.SetRootDirName`. Enables multiple isolated agents (distinct env roots) in one process — the one global mutation `NewStandalone` currently makes.
