# Session Teardown on Tenant Deletion (Layer 2)

**Status: deferred design.** The *interim mitigation* below has shipped — the account/org deletion guards landed with the Layer 1 `Listen`/`Dial` primitives work and the account-owned-tunnels work. The *full teardown* described here is the deferred follow-up and is **not yet implemented**. This doc exists so the gap is tracked rather than implicit.

## The problem

A Layer 2 session's backing tunnel is created under its **provider**. `AcceptSession` (`internal/controller/acceptSession.go`) provisions the Ziti service + bind policy (`:107`) and inserts the `tunnels` row (`:147`) with the *provider's* organization / account / environment (`:133-135`), and grants the consumer its dial policy (`:151`). The `sessions` row links the two parties (`consumer_account_id`, `provider_account_id`).

The tenant foreign keys are `ON DELETE CASCADE` end to end (`internal/persistence/migrations/0001_core_layer1.sql`). So deleting a **consumer** account cascades that account's `sessions` rows (and its grants) at the database layer — but the session's backing tunnel lives under the **provider**, in a *different* account and environment, and is therefore **not** cascaded. The result:

- the provider-owned tunnel row and its Ziti **service + bind policy stay live**, and
- `teardownSession` (`internal/controller/closeSession.go`) — which would have deprovisioned that tunnel — can never run, because the `sessions` row it keys on is already gone.

A live overlay bind policy and service for a session that no longer exists: a resource leak and a zero-trust residue. The symmetric hazard exists for any deletion path that removes a `sessions` row without first tearing down the backing tunnel app-side.

This is one instance of a general rule (L1 work order, cross-cutting rule 3): because the tenant FKs cascade, **every Ziti-bearing parent delete must be either cascade-aware or guarded** — a bare delete that leans on the FK cascade strands the overlay objects its children owned. The L1 work order makes the *attachment* and *owned-tunnel* paths cascade-aware; the **session backing tunnels of a deleted participant** are the piece left for here.

## Interim mitigation (shipped with the L1 work order)

To keep deletion **safe** without yet building full teardown, account and organization deletion are **guarded** rather than allowed to strand:

- **Account delete** refuses (`ErrConflict`) while the account participates — as consumer *or* provider — in any active session with a backing tunnel. Race-safe via lock-then-check: `SELECT … FOR UPDATE` the account row, then check `sessions` under the lock; the `sessions` / grant → `accounts` FK serializes a concurrent `proposeSession` / `AcceptSession` against the delete (an accept that loses the race fails its grant insert and deprovisions its just-made tunnel).
- **Org delete** is guarded the same way one level up — it refuses while any account remains, so a tenant is emptied account-by-account first.

The cost is **ergonomic, not correctness**: an operator must end (or let close) a tenant's active sessions before the tenant can be deleted. Nothing is stranded.

## Deferred: full session teardown

The complete fix lets deletion *proceed* by tearing sessions down first, instead of refusing:

1. Before deleting an account (and, by composition, an org), **enumerate the sessions the account participates in** that have a backing tunnel — both as consumer and as provider.
2. For each, run the existing whole-service tunnel cleanup (deprovision the Ziti service / bind policy and every consumer's dial policy, cross-org) **under the tunnel lock**, and close the session, **before** the FK cascade removes the `sessions` row.
3. Only then delete the account row; the cascade now finds nothing live to strand.

This is deliberately a **Layer 2 deletion-lifecycle** work item, not an extension of the L1 cleanup matrix. It has to reason about the session state machine (proposed / accepting / active / closed), the provider↔consumer relationship, and the ordering against `teardownSession` — concerns that belong to Layer 2, not to the Layer 1 `Listen`/`Dial` primitives that merely surfaced the gap. Doing it properly is why it was carved out rather than bolted onto the L1 work order.

## Related: account-owned standalone tunnels

Account-owned standalone tunnels (see
[`../../current/layer-1/spec.md`](../../current/layer-1/spec.md)) join this same
family. Once a standalone tunnel carries no `environment_id`, environment retirement no longer
deprovisions it, so account/org deletion becomes the parent delete that must not strand its OpenZiti
service, bind policy, and terminators. The interim mitigation is the same shape as above: account
deletion is **guarded** — it refuses while the account still owns a standalone tunnel — so the
operator deletes the tunnels first (each deprovisioned via the normal `deleteTunnel` path) and
nothing strands. The deferred full teardown is the same as for session backing tunnels: enumerate
the account's standalone tunnels, deprovision their fabric, then delete — folded into this doc's
deferred work rather than tracked separately.

## Related: the account-delete / org-delete asymmetry

Account deletion **cascades** (it tears down the account's owned environments and their tunnels); organization deletion is **guarded** (it refuses while accounts remain). That asymmetry is a conscious choice — the destructive one-call cascade is permitted at account scope but withheld at the tenant (org) scope, where a single call would otherwise destroy an entire multi-account tenant.

It is **flagged for possible future revisit**: an enterprise posture might prefer guarding account deletion too (symmetry — require an account be emptied before it can be deleted). Not a blocker today; recorded here so the decision stays visible rather than implicit.
