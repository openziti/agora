# Account-Owned Tunnels — Work Order

Implementation translation of [`account-owned-tunnels.md`](./account-owned-tunnels.md). This is the plan an implementation agent executes; it grounds the spec in the code as it exists on the `standalone-tunnel-lifecycle` branch and records the slicing, integration points, and decisions the spec leaves open.

## What grounding changed about the model

The spec's framing is mostly accurate, with three corrections that shape the work:

1. **Tunnels are already account-owned at the schema level.** `tunnels.account_id` exists with an FK, and `canManageTunnel` (`internal/controller/tunnel_helpers.go`) already keys off `AccountID`. The defect is the environment **binding**, not ownership. Four mechanisms bind a tunnel to one environment; this work removes/loosens them:
   - `tunnels.environment_id NOT NULL` (`migrations/0001_core_layer1.sql:58`)
   - `tunnels_environment_organization_fk ... ON DELETE CASCADE` (`0001:75-76`) — the DB-level cascade that deletes tunnels when their environment is deleted
   - the serve-time env-match rejection (`tunnelServeLifecycle.go:49-61`)
   - the `disableEnvironment` deletion loop (`disableEnvironment.go:112-155`)

2. **Account-wide hosting is a reuse, not new machinery.** Every environment identity already carries an `agora-account:<id>` Ziti role attribute (`tunnel_helpers.go:20-38`, assigned at enrollment in `enableEnvironment.go`). Today the bind policy targets the single creating env identity (`@<envIdentity>`, `automation/tunnel.go:81`). Pointing it at the account role attribute (`#agora-account:<id>`) makes any of the account's environments eligible to host — and is what lets a standalone tunnel be re-hosted after its creating environment is retired.

3. **`--takeover` is the one genuinely new piece.** The controller has no terminator list/delete capability today (`automation/client.go` wraps Services/Identities/Policies only). Takeover requires a net-new `TerminatorManager` over the OpenZiti edge-management API. The spec's "subtraction, not new machinery" line is true for hosting but not for takeover; the spec has since been aligned to say so (see Decisions and deferrals).

## Decisions locked

- **Full takeover lands in this work order**, including the new fabric terminator eviction.
- **`tunnels.environment_id` becomes nullable**: `NULL` = standalone (account-owned), set = a Layer 2 session tunnel. The null/set split *is* the standalone-vs-session discriminator. No new column.

## Change set (sliced)

### Slice 1 — Schema migration (`migrations/0012_account_owned_tunnels.sql`)

```sql
-- +migrate Up
alter table tunnels alter column environment_id drop not null;

-- enforce the org -> account -> environment boundary in the schema (finding C1): a tunnel that
-- references an environment must reference one owned by the tunnel's own account, not merely one in
-- the same organization. this needs a matching unique target on environments.
alter table environments add constraint environments_id_account_organization_unique
    unique (id, account_id, organization_id);

alter table tunnels drop constraint tunnels_environment_organization_fk;
alter table tunnels add constraint tunnels_environment_account_organization_fk
    foreign key (environment_id, account_id, organization_id)
    references environments(id, account_id, organization_id);
-- no ON DELETE CASCADE (NO ACTION). a standalone tunnel has NULL environment_id and (MATCH SIMPLE)
-- is not checked by this FK at all; a session tunnel keeps environment_id set and is torn down by
-- the retire handler before the environment row is deleted, so the constraint never blocks retire.
```

- The env FK is now **account-scoped** (`(environment_id, account_id, organization_id)`), so the org → account → environment boundary is enforced in the schema, not just in controller code (finding C1). Standalone tunnels (NULL env_id) are exempt via MATCH SIMPLE; only session tunnels, which carry an env, are checked.
- Keep `ON DELETE CASCADE` on the env FKs of `tunnel_serves` (`0001:129-130`) and `tunnel_attachments` (`0001:109-110`) — a serve/attachment *is* that environment's participation and should die with it. This is what auto-clears an environment's serve records on the standalone tunnels it was hosting when the environment is retired.
- Org-unique-name index (`idx_tunnels_org_name_unique_active`) unchanged — names stay org-unique.
- A composite FK cannot use `ON DELETE SET NULL` (it would null `organization_id`, which is `NOT NULL`); `NO ACTION` is the correct choice and is cascade-safe because tunnels are independently cascade-deleted via `tunnels_account_organization_fk`/org FK when an account/org is removed.

### Slice 2 — Persistence model/repo (`models.go`, `repositories.go`)

- `Tunnel.EnvironmentID` → `*string`; adjust `Tunnels.Create` insert and row scans for nullable.
- `ListByEnvironment` query (`where environment_id = $1`) is unchanged and now naturally returns only session tunnels (+ any legacy env-bound rows) — `NULL` env_id is excluded. This is the hinge the retire behavior turns on.
- Audit log call sites that print `tunnel.EnvironmentID` (e.g. `createTunnel.go:41,149`) need nil-safe formatting (`optionalStringValue`).

### Slice 3 — Account-scoped bind policy + env-optional create

- **Parameterize the bind identity roles** so standalone and session tunnels differ. In `automation/tunnel.go` (`TunnelSpec` / `Provision`), replace the hardcoded `IdentityRoles: []string{"@" + spec.EnvironmentIdentityID}` (`:81`) with a caller-supplied `BindIdentityRoles []string`. Rename the bind policy from `<envIdentity>-<svc>-bind` to a non-env name (e.g. `<svc>-bind`).
- **`createTunnel.go`** (standalone path): make `environmentId` optional; the standalone tunnel is never bound to it. **A supplied `environmentId` is treated only as caller context** (validate ownership if desired, then discard) — the created row always has `EnvironmentID = nil` regardless of what the caller sent (finding R2-A1). This matters because a persisted `environment_id` would make a standalone tunnel look like a session tunnel under the null/set discriminator and silently reintroduce env-binding. Skip `lockEnvironmentScope`/`requireOwnedEnvironment` when absent. Pass `BindIdentityRoles: ["#" + accountRoleAttribute(principal.AccountID)]`.
- **`acceptSession.go`** (session path) is unchanged in behavior: it keeps `EnvironmentID = env.ID` and passes `BindIdentityRoles: ["@" + env.ZitiIdentityID]` (env-scoped), preserving session-tunnel environment ownership.
- `accountRoleAttribute` already exists in `tunnel_helpers.go:20-38`; reuse it.

### Slice 4 — Serve eligibility (`tunnelServeLifecycle.go`)

- Delete the env-match rejection (`:49-61`). Replace with an owning-account check on the serving environment: reject if `env.AccountID != tunnel.AccountID` (or org mismatch). Hosting is owning-account-only — grants govern *reach*, not *host*. (`requireManagedTunnel` + `requireOwnedEnvironment` already constrain to the principal's account; the explicit check closes the admin-managing-another-account's-tunnel edge.)
- Result: any environment of the owning account may serve; clean moves work unchanged because clean stop already deletes the serve record (`DeleteTunnelServe`, `:158-188`) and frees the unique-active constraint.

### Slice 5 — Fabric terminator capability (`automation/`)

- Add a `TerminatorManager` modeled on `ServicePolicyManager` (`service_policy.go` is the template): `Find(filter)` via `terminator.ListTerminators`, `Delete(id)` via `terminator.DeleteTerminator`. `Find` is used with two filters: `service = "<svcId>"` (takeover, Slice 6) and `identity = "<identityId>"` (retire, Slice 8; with the client-filter fallback in R2-A2). Register a `Terminators` field on the `Client` struct (`client.go`). `edge-api v0.27.5` already exposes both operations.

### Slice 6 — Takeover (controller + API)

- **Eviction primitive** (controller helper, shape-agnostic): given a tunnel, (a) list terminators for `tunnel.ZitiServiceID` and delete each via the new `TerminatorManager`; (b) if an active `tunnel_serves` record exists (proxy), mark it `disconnected` and delete it. Blunt and unconditional per spec — no health check.
- **Operation**: `POST /tunnels/{tunnelId}/takeover` (new operationId `takeoverTunnel`), authorized by `requireManagedTunnel`. Shape-agnostic so it serves both proxy (serve record + terminators) and direct (terminators only) tunnels. This is required because direct tunnels have **no serve operation** — `StartTunnelServe` rejects non-proxy tunnels (`:31-37`) — so takeover cannot be a serve-only parameter.
- **Standalone-only (finding R2-C1):** `takeoverTunnel` must reject any tunnel with `EnvironmentID` set (a Layer 2 session tunnel) before any fabric or DB mutation — takeover applies only to account-owned standalone tunnels (`environment_id` NULL). The guard lives at the operation, so both the `--takeover` flag and the `tunnel takeover` command inherit it, and session-tunnel behavior stays untouched as the spec requires.
- **Why takeover is needed despite the reaper**: clean stop frees the serve slot immediately, and the reaper (`tunnelServeReaper.go`, 45s TTL) flips an *un*clean-death record to `stale` (freeing the constraint within ~60s). Takeover's unique value is the **wedged-but-still-heartbeating** host (reaper won't touch it: serve record stays `active`, terminator stays live and splits traffic) and **direct tunnels** (no record at all).

### Slice 7 — CLI (`cmd/agora/`)

- Add `--takeover` to `tunnel serve` (`tunnelServe.go`): when set, call `takeoverTunnel` before the serve flow (proxy). Thread through both the foreground `StartTunnelServe` path and the network-agent `EnsureServe` path (`sdk/agent/networkpb`).
- Add a shape-agnostic `agora tunnel takeover <name>` command calling `takeoverTunnel` directly — this is how a direct tunnel (hosted by the SDK runtime, not `tunnel serve`) is reclaimed. **Decided:** takeover is its own verb over a single eviction operation; direct-tunnel takeover is CLI-exposed via this command, and `--takeover` on `serve` is a convenience wrapper over the same operation for the proxy flow.

### Slice 8 — Retire handler (`disableEnvironment.go`)

Grounding result: **standalone-tunnel survival is a DB-side consequence of Slice 1** — with `environment_id` nullable, `ListByEnvironment` (`:64,85`) returns only session tunnels, so the deprovision+delete loop (`:112-155`) no longer touches standalone tunnels, and their serve records cascade-clear via the `tunnel_serves` env FK when the env row is deleted (`:156`). The account-scoped bind policy (Slice 3) keeps the service re-hostable by another environment.

- **Required (finding C2): explicitly evict the retiring environment's fabric terminators** via the new `TerminatorManager` (Slice 5), rather than relying on `envLifecycle.Disable` (`:128`) deleting the env identity to drop them as a side effect. Filter terminators by the environment's Ziti identity (`env.ZitiIdentityID`) and delete each before the identity is removed; if OpenZiti does not support server-side filtering of terminators by identity, list terminators and client-filter by the returned identity id (finding R2-A2). This is especially necessary for **direct** tunnels: they have no serve row, so the DB cannot enumerate which services the environment was hosting — only the identity-scoped terminator list can. This removes the "hope identity-deletion cleans up" assumption and guarantees no stranded residue on the standalone tunnels the environment was hosting.
- Required: a test proving a standalone tunnel survives retirement of the environment that created and/or hosted it, and that no terminator for the retired identity remains.
- Optional polish: transition an environment's serve records on standalone tunnels to `disconnected` (audit nicety) before the cascade, keyed by environment rather than by tunnel.
- Out of scope (flag only): session tunnels are still raw-deprovisioned+deleted here, leaving the session row dangling (`tunnel_id` SET NULL, still active) — a pre-existing gap, tracked in [`layer-2/session-cleanup-on-environment-retire.md`](./layer-2/session-cleanup-on-environment-retire.md). See Decisions and deferrals.

### Slice 8b — Account/org deletion guard (finding R2-C2)

Account-owning a tunnel makes the account the deprovision-responsible party. Standalone tunnels (NULL env_id) are not reached by environment-retire cleanup, so account/org deletion must not strand their OpenZiti service / bind policy / terminators after a bare DB cascade.

- **Guard, not teardown (matches the tenant-deletion precedent):** `deleteAccount` refuses (`ErrConflict`) while the account still owns any standalone tunnel; `deleteOrganization` already refuses while any account remains, so the org is emptied account-by-account. The operator deletes the standalone tunnels first, and the existing `deleteTunnel` path deprovisions each tunnel's fabric objects — so nothing strands.
- Race-safe via the same lock-then-check pattern used for the session guard in [`layer-2/session-teardown-on-tenant-deletion.md`](./layer-2/session-teardown-on-tenant-deletion.md).
- Full automatic teardown (enumerate standalone tunnels, deprovision their fabric, then delete) is **deferred** and recorded in that same doc, consistent with how session backing tunnels are handled.
- Test: deleting an account that still owns a standalone tunnel is refused; after the tunnel is deleted, the account delete succeeds with no fabric residue.

### Slice 9 — API spec + regen (`internal/api/specs/tunnels/`)

- `createTunnelRequest` / tunnel schema: `environmentId` optional/nullable.
- Add the `takeoverTunnel` operation (`POST /tunnels/{tunnelId}/takeover`).
- Regenerate: `./bin/generate_rest.sh`. Do not hand-edit generated code.
- UI note: `ui/src/lib/api/types.ts` is hand-maintained; a nullable `environmentId` on the tunnel object is a UI-visible change and needs a manual sync (per CLAUDE.md footgun).

### Slice 10 — docs/current (synthesized at implementation time)

- `docs/current/layer-1/spec.md` lines "bound to the environment that serves them" / "serve is environment-bound" must change to reflect account-owned hosting and the new takeover operation.

## Tests

- **Persistence (testcontainers):** migration applies; `environment_id` nullable; a standalone tunnel (NULL env_id) **survives** deletion of the environment that created it; a session tunnel (env_id set) is still torn down; the env-FK no longer cascade-deletes standalone tunnels; the account-scoped env FK **rejects** a session tunnel whose account does not own the referenced environment (finding C1); the NO ACTION env FK does not block tenant teardown — deleting an owning **account** or **organization** whose tunnels are session tunnels (env set), and deleting one after its standalone tunnels have been removed, succeeds with no tunnel/serve/attachment row residue (finding C3). (The refuse-while-standalone-tunnels-exist guard and OpenZiti fabric-residue checks are controller/fabric tests, above — findings R2-C2 and C2.)
- **Controller:** serve from a *second* environment of the owning account succeeds; another account's environment is rejected; retiring an environment keeps standalone tunnels standing, clears that environment's serve/attachment participation, and leaves **no live terminator for the retired identity** (fabric-residue check, finding C2); `takeoverTunnel` deletes terminators (both shapes) and clears the active serve record (proxy); `takeoverTunnel` **rejects** a session tunnel (env_id set) without touching its terminators or serve record (finding R2-C1); deleting an account that still owns a standalone tunnel is **refused**, and succeeds with no fabric residue once the tunnel is deleted (finding R2-C2).
- **CLI:** `tunnel create` direct/proxy unchanged; `tunnel serve --takeover` and `tunnel takeover` wiring compiles and dispatches the right operations.
- `go test ./...`, `go vet ./...`, `go build ./...` (never `go build ./path/to/cmd`).

## Decisions and deferrals

1. **Spec framing accuracy.** *Resolved (round 1, finding A1):* the spec now states hosting is subtraction while `--takeover` deliberately adds controller-driven terminator eviction.
2. **Dangling sessions on retire.** Pre-existing: retiring an environment deletes a session tunnel and leaves the session row active with `tunnel_id` NULL. Spec says session behavior is unchanged ("as it is today"), so this work preserves it. **Decided: deferred and tracked** in [`layer-2/session-cleanup-on-environment-retire.md`](./layer-2/session-cleanup-on-environment-retire.md), not folded into this work.

Resolved during planning: takeover is exposed as a dedicated shape-agnostic operation (`takeoverTunnel`) with an `agora tunnel takeover <name>` command, and `--takeover` on `serve` as a wrapper — see Slices 6–7.

## Migration / rollout

Per spec: lab-only, recreate rather than migrate. The schema migration (Slice 1) both loosens (nullable `environment_id`, drops the env-FK cascade) and tightens (account-scoped env FK plus a new `environments` unique target). Existing dev/demo tunnels are env-bound with their environment owned by their account, so they already satisfy the account-scoped FK that `ADD CONSTRAINT` validates against current rows; any legacy mismatch is handled by a `bin/demo-down.sh [--attach] --purge` reset, which is also the clean way to exercise the new model end-to-end. No data-conversion logic is built.
