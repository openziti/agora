---
title: removeServeActor loses desired serve when persistence fails
state: inbox
created: 2026-10-20
tags: [defect]
source: docs/current/layer-1/agent.md#bulk-local-connect-removal
log: docs/journal/2026-10-20.md
---

When `persistNetwork` fails in `removeServeActor` (`sdk/agent/tunnel_runtime.go`), the actor goes back into `a.serves`, but `removeServe` has already replaced `a.network.Serves`. The serve keeps running or retrying, yet `ListServes` iterates `a.network.Serves` and no longer shows it, while the row remains in `network.json`, so a rerun of `agora tunnel delete serve` cannot find it. Restore the previous `a.network.Serves` slice on failure, matching the connect-side fix in `removeConnectActor`, and add a regression test like `TestAgentRemoveConnectPersistFailureKeepsConnect` (`failingPersistRoot` in `sdk/agent/agent_test.go` already injects the write failure).

## background

Terminus raised this on 2026-10-20 (review `1802372e5888`, finding `bulk-connect-persist-rollback`) against the connect path during the bulk connect-removal work. The connect side was fixed in that work; the serve side was left unchanged by direction.
