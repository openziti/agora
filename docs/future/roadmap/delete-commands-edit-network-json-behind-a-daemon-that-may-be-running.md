---
title: Delete commands edit network.json behind a daemon that may be running
state: inbox
created: 2026-10-20
tags: [defect]
source: docs/current/layer-1/agent.md#bulk-local-connect-removal
log: docs/journal/2026-10-20.md
---

`agora tunnel delete serve`, `agora tunnel delete connect --listen`, and `agora tunnel delete <name|tt_...>` (through `drainManagedTunnelState`) in `cmd/agora/tunnelDelete.go` fall back to editing `network.json` whenever `openNetworkClient` returns `daemon.ErrNotRunning`. `normalizeDialError` (`internal/network/daemon/client.go`) also assigns that classification to timeouts, cancellation, and gRPC `Unavailable`, so a live daemon that is slow or wedged gets its desired state edited behind its back: the removed actor keeps running, and the daemon's next save writes the row back. Route these commands' offline fallback through `confirmNetworkAbsent`, which the `--all` bulk path already uses, so only a missing or refusing socket permits the offline edit. Decide separately whether `agora disable`, which deletes `network.json` after a drain that silently skips on the same classification, needs the same gate.

## background

Terminus raised this on 2026-10-20 (review `1802372e5888`, finding `bulk-connect-ambiguous-daemon`) against `--all`. Only the bulk path was fixed, by direction, so the existing commands' behavior is unchanged.
