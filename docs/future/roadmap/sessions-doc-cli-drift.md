---
title: Sessions doc CLI drift
state: inbox
created: 2026-09-18
tags: [documentation]
source: docs/future/layer-2/persisted-session-connects-investigation-findings.md
---

Reconcile the CLI Surface section of `docs/current/layer-2/sessions.md` with `cmd/agora/session*.go`. At `ee1597d` the doc specifies `agora session close -y/--yes` and `agora session propose --json/--timeout`, none of which exist, and says `session propose` waits for `active` and prints the tunnel id and attach endpoint, while the command returns at `proposed`. Decide per item whether the doc or the command is wrong; `docs/current/` is supposed to be verifiable against running code.
