# Session Cleanup on Environment Retire (Layer 2)

**Status: deferred, tracked.** This is a pre-existing gap, surfaced (not caused) by the
account-owned-tunnels work (now implemented; see
[`../../current/layer-1/spec.md`](../../current/layer-1/spec.md)), which deliberately left it out of
scope and flags it here.
**Not yet implemented.** This doc exists so the gap is tracked rather than implicit.

## The problem

A Layer 2 session's backing tunnel is created under its **provider's** environment:
`AcceptSession` (`internal/controller/acceptSession.go`) inserts the `tunnels` row with the
provider's `environment_id` and links it from the `sessions` row via `tunnel_id`.

When that environment is retired, `disableEnvironment`
(`internal/controller/disableEnvironment.go`, `disableEnvironmentWithLocks`) enumerates the
environment's tunnels (`ListByEnvironment`, `:64/:85`), deprovisions each from Ziti (`:112-122`),
and deletes the tunnel rows (`:152`). It does **not** look at `sessions`. So:

- the `sessions.tunnel_id` FK (`migrations/0006_layer2_sessions.sql:11`, `on delete set null`)
  nulls the link, and
- the session row is left **in its non-closed state with `tunnel_id = NULL`** — a session that
  still looks live but has no tunnel behind it.

`teardownSession` (`internal/controller/closeSession.go`) — which would mark the session closed and
do the orderly cleanup — never runs, because nothing in the retire path invokes it.

This is the same family as
[`session-teardown-on-tenant-deletion.md`](./session-teardown-on-tenant-deletion.md): a
Ziti-bearing parent delete that isn't session-teardown-aware. There the trigger is account/org
deletion and the residue is a live service for a gone session; here the trigger is environment
retirement and the residue is a stranded session row for a gone tunnel.

## Why it surfaces now

Before account-owned-tunnels, the retire path deleted *every* tunnel the environment created, so
this was one effect among several. After account-owned-tunnels, standalone tunnels are excluded
from `ListByEnvironment` (they carry a `NULL` environment_id) and survive retirement — which means
**every tunnel the retire path still deletes is a session tunnel.** The dangling-session effect is
no longer incidental; it is the entire remaining tunnel-deletion behavior of that path. That
concentration is why it is worth writing down even though the behavior itself is unchanged.

## The fix (sketch)

Make the retire path session-teardown-aware rather than relying on a bare tunnel delete:

1. For each session tunnel the environment hosts, find its session
   (`Sessions.GetByTunnelID`) and run the orderly `teardownSession` path (close the session,
   deprovision the service / bind policy / consumer dial policies) **before** the tunnel row is
   deleted.
2. Mind the transaction shape: `teardownSession` opens its own `WithTx`, while
   `disableEnvironmentWithLocks` already runs inside one — the teardown logic would need to accept
   the ambient transaction (or the close be sequenced around it) rather than nest.

This is a Layer 2 deletion-lifecycle concern (the session state machine, the provider↔consumer
relationship, ordering against `teardownSession`), not a Layer 1 ownership concern — which is why
it is carved out of the account-owned-tunnels work rather than bolted onto it.

## Related

- [`session-teardown-on-tenant-deletion.md`](./session-teardown-on-tenant-deletion.md) — the
  sibling gap (account/org deletion); the broader "every Ziti-bearing parent delete must be
  cascade-aware or guarded" rule lives there.
- [`../../current/layer-1/spec.md`](../../current/layer-1/spec.md) — the account-owned-tunnels work
  that surfaced this and deliberately scoped it out (now implemented).
