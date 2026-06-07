# Work Order: Go-Native Layer 1 Primitives (`Listen` / `Dial`)

This is the implementation work order for [`sdk-l1-primitives.md`](./sdk-l1-primitives.md). It translates that spec into concrete code changes, slices the work, names integration points, and folds in the implementation-correctness items that the spec-phase review deferred here. Read the spec first for the *why*; this document is the *how* and *where*.

## Architecture: the thin path

The single most important shape, confirmed against the code: **the thin primitives do not touch the gRPC runtime or the managed-actor lifecycle.** A managed `EnsureServed`/`EnsureConnected` call goes `tunnel.EnsureServed` → `runtimeFromAgent` → `networkpb` gRPC → managed actor (`sdk/agent/tunnel_runtime.go`) → heartbeat/retry loops. The thin `Listen`/`Dial` path is far shorter:

- **Provisioning / resolution** uses the controller API client directly — `agent.Controller()` returns the `*api.Client` (`sdk/agent/app.go:228`), which already has `CreateTunnel`, `DeleteTunnel`, `GetTunnel`, `ListTunnels`, `ConnectTunnel`, `DeleteTunnelAttachment`, `RemoveTunnelGrant` (`internal/api/oas_client_gen.go`).
- **The overlay object** is built directly — `tunnelruntime.OpenZitiFactory{}.New(identityPath)` returns an `OverlayContext` (`internal/network/tunnelruntime/overlay.go:10-35`), and `OverlayContext.Listen(serviceName)` / `Dial(serviceName)` return the raw `net.Listener` / `net.Conn`. The service name passed is the tunnel's ID, exactly as the managed runtime does (`sdk/agent/runtime_host.go` passes `tunnel.ID` into `tunnelruntime.StartServe*`).
- **Identity** comes from `agent.EnvRoot().ZitiIdentityNamed(...)` (the same path the managed runtime resolves).

So the new SDK functions need only the `Agent`'s controller client, environment, and identity path — **no started runtime, no `networkpb`, no managed actor state.** This is why `Listen` can be thin: provisioning persists a tunnel (bind policy on the tunnel, which the serve reaper never removes), and `Listen` just opens the overlay on it.

## Slicing

Two phases, listener first (it unblocks the motivating `mcp-gateway` case and depends on nothing new beyond its own serve-side schema):

- **Phase 1 — Provider (`Create` / `Delete` / `Listen`).** Ships independently.
- **Phase 2 — Consumer (`Attach` / `Detach` / `Dial`).** Depends on Phase 1's `kind`-discriminator pattern and carries the heavier controller-lifecycle work (reaper skip, cross-org cleanup, cancellable dial).

Within each phase the slices are: (a) schema migration, (b) persistence model/repo, (c) API spec + regen, (d) controller behavior, (e) Ziti automation, (f) SDK surface, (g) tests.

---

## Phase 1 — Provider: `Create` / `Delete` / `Listen`

### 1a. Migration — tunnel `kind`

New file `internal/persistence/migrations/0010_tunnel_kind.sql` (next number after `0009_audit_events.sql`; embedded via `//go:embed`, run by `sql-migrate` in `internal/persistence/migrate.go`):

- `alter table tunnels add column kind text not null default 'proxy';`
- `alter table tunnels add constraint tunnels_kind_valid check (kind in ('proxy','direct'));`
- `alter table tunnels alter column backend_target drop not null;`
- Drop `tunnels_backend_target_not_empty`; add the conditional invariant:
  ```sql
  constraint tunnels_backend_target_kind check (
    (kind = 'proxy'  and backend_target is not null and btrim(backend_target) <> '') or
    (kind = 'direct' and backend_target is null)
  )
  ```

The `NOT NULL` on `kind` is load-bearing (a Postgres `CHECK` passes a NULL `kind` as UNKNOWN). The existing unique index `idx_tunnels_org_name_unique_active(organization_id, lower(name)) where not deleted` (`0002_core_indexes.sql`) already enforces the "one tunnel per name" conflict between `EnsureServed` and `Create` — no change needed there.

### 1b. Persistence

- `internal/persistence/models.go:126-141` — add `Kind TunnelKind \`db:"kind"\`` to `Tunnel`; change `BackendTarget string` → `BackendTarget *string`. Add a `TunnelKind` type (`proxy`/`direct`) beside the existing `TunnelMode`/`TunnelState`.
- `internal/persistence/repositories.go` — `Tunnels.Create` (398-443) binds `kind` and the nullable `backend_target`, and **defaults an empty `kind` to `proxy`** (exactly as it already defaults `Mode`/`State`), so direct-construct call sites such as `acceptSession.go` (which builds a `persistence.Tunnel` by hand for Layer-2 sessions) remain `proxy` without per-site edits. Every `SELECT` in the repo (`GetByID`, `GetByName`, `GetByNameGrantedToAccount`, `ListBy*`) adds `kind` to its column list and scans the now-nullable `backend_target`. The `*string` ripple is mechanical but touches each read site.

### 1c. API spec + regen

- `internal/api/specs/tunnels/schemas.yml`:
  - `tunnel` (1-47): add `kind` (enum `[proxy, direct]`, required); make `backendTarget` optional (drop from `required`).
  - `createTunnelRequest` (53-76): make `backendTarget` optional — *absent-only, not `nullable`* (drop from `required`; keep `minLength: 1` when present); kind is inferred from presence (omitted → `direct`), and an explicit JSON null is rejected (clients omit the field). Do **not** add a `kind` field. Tests cover omitted / non-empty / whitespace-only (no null case, since the contract is absent-only).
- Regenerate: `bin/generate_rest.sh` (`go generate ./...`). Do not hand-edit `internal/api/oas_*_gen.go`.

### 1d. Controller

- `internal/controller/createTunnel.go:13-131` — infer kind at the edge from `backendTarget` presence (the exact absent-only rule: absent → `direct`; trimmed non-empty → `proxy`; present-but-empty/whitespace → reject; explicit JSON null rejected by the non-null optional OpenAPI field). Set `Tunnel.Kind` and the nullable `BackendTarget` accordingly. Provisioning (`tunnelLifecycle.Provision`, `automation/tunnel.go:54-112`) is unchanged: it already creates the Ziti service + bind policy and is mode-agnostic at the fabric, and it doesn't use `backend_target` — a direct tunnel provisions identically, it just stores no backend.
- `internal/controller/tunnelServeLifecycle.go` (`StartTunnelServe`) and any managed serve create-or-reuse path — **require `kind = proxy`**: a `direct` tunnel has no backend and is served only by `Listen`, so a managed serve actor must never latch onto one. Reject a serve start on a `direct` tunnel (the symmetric guard to "one name, one kind").
- `internal/controller/deleteTunnel.go:14-99` — no change needed for Phase 1; it already deprovisions by IDs and hard-deletes.
- Display: the dashboard/CLI should render a `direct` tunnel as "direct listener" where a `proxy` tunnel shows `backend_target`. (`kind` is in the REST response from 1c, so clients branch on it, not on a string.)

### 1e. Ziti automation

No change. `Provision` (`automation/tunnel.go:54-112`) already produces the service + bind policy + tags for any tunnel; `Mode` is metadata only.

### 1f. SDK surface (`sdk/agent/tunnel`)

New functions in the `tunnel` package (no stutter — package is `tunnel`):

- `tunnel.Create(ctx, a, Spec) (*Tunnel, error)` — wraps `a.Controller().CreateTunnel(...)` with a `direct`-kind request (omit `backendTarget`). `Spec` carries `Name`, `Mode`, `GrantEmails`; defaults the environment to the agent's enrolled environment when omitted.
- `tunnel.Delete(ctx, a, *Tunnel) error` — wraps `a.Controller().DeleteTunnel(...)`.
- `tunnel.Listen(ctx, a, nameOrID) (net.Listener, error)`:
  1. resolve `nameOrID` to a controller `Tunnel` via `a.Controller()` (same-org: `GetTunnel`/`ListTunnels`),
  2. validate: `kind == direct`, `mode ∈ {http, tcp}` (reject `udp`), `EnvironmentID == agent environment`, Ziti service metadata present,
  3. build the overlay: `tunnelruntime.OpenZitiFactory{}.New(identityPath)` (identity from `a.EnvRoot()`),
  4. `return overlay.Listen(tunnel.ID)` — the same service identifier the managed runtime uses (`runtime_host.go`).

No `ServeHandle`, no `Status()`, no heartbeat — `Listen` returns the raw `net.Listener` and an error. The package's `EnsureServed`/`EnsureConnected` and the gRPC `runtimeClient` are untouched.

**Public types** (small, hand-written — do **not** export generated `internal/api` types):
- `type Kind string` (`KindProxy`, `KindDirect`); `type AttachmentKind string` (`AttachmentProxy`, `AttachmentDialer`).
- `type Spec struct { Name string; Mode Mode; GrantEmails []string; EnvironmentID string }` — `EnvironmentID` is optional; empty defaults to the agent's enrolled environment (the spec's defaulting rule).
- `type Tunnel struct { ID, Name string; Kind Kind; Mode Mode }` — returned by `Create` and resolution; carries what `Listen` needs without leaking `api.Tunnel`.
- (Phase 2) `type Attachment struct { ID, TunnelID, EnvironmentID string; Kind AttachmentKind }` — returned by `Attach`.

### 1g. Tests (Phase 1)

- Migration: applies cleanly; `kind` defaults existing rows to `proxy`; the conditional check rejects `proxy`-without-backend and `direct`-with-backend; `NOT NULL` rejects a null `kind`.
- `createTunnel`: a request without `backendTarget` creates a `direct` tunnel (no backend, kind set); with `backendTarget` creates `proxy`; whitespace-only `backendTarget` is rejected; same-name conflict with an existing tunnel still errors.
- SDK: `Listen` resolves + validates (rejects non-`direct`, `udp`, wrong-env), and opens the overlay on `tunnel.ID`. Cover the `overlay.Listen(name)`-instead-of-ID mistake with a test that would catch it.

---

## Phase 2 — Consumer: `Attach` / `Detach` / `Dial`

Phase 2 carries the genuinely entangled controller work. Its design mirrors zrok's consumer posture (`unaccess` / `disable` / `unshare`): no consumer-side reaper, the Ziti dial policy is the security boundary, cleaned up explicitly + on environment teardown + by tag on provider teardown.

### 2a. Migration — attachment `kind`

New file `internal/persistence/migrations/0011_attachment_kind.sql`:

- `alter table tunnel_attachments add column kind text not null default 'proxy';`
- `check (kind in ('proxy','dialer'))`.
- `alter column listen_address drop not null;`
- Replace `tunnel_attachments_listen_address_not_empty` with the conditional invariant (`proxy` ⇒ non-empty address; `dialer` ⇒ null).
- Add the partial unique index for one **active** dialer grant per consumer env+tunnel (scoped to live rows):
  ```sql
  create unique index idx_tunnel_attachments_dialer_unique
    on tunnel_attachments (environment_id, tunnel_id)
    where kind = 'dialer' and state = 'active' and not deleted and dial_policy_id is not null;
  ```
  The index predicate must match the *authorization* predicate exactly (active + not deleted + has a policy). `state='active' and not deleted` is required because the delete path marks rows `disconnected` without setting `deleted`; `dial_policy_id is not null` is required so a half-broken active-but-policy-less row — invisible to `GetActiveDialerByEnvironmentAndTunnel` — doesn't block a fresh `Attach`. Test: reattach over an active dialer row whose `dial_policy_id` is null.

### 2b. Persistence

- `models.go:167-181` — add `Kind TunnelAttachmentKind`; `ListenAddress string` → `*string`. Add the `TunnelAttachmentKind` type.
- `repositories.go` — `TunnelAttachments.Create` (689-733) binds `kind` + nullable `listen_address`; reads add `kind`. New queries:
  - **Active-dialer-by-key:** `GetActiveDialerByEnvironmentAndTunnel(envID, tunnelID)` — the authorization predicate `kind='dialer' and state='active' and not deleted and dial_policy_id is not null`. Used by idempotent `Attach`, by `Dial`, and by `Detach`-by-natural-key.
  - **Consumer enumeration:** `ListConsumerAttachmentsWithPolicy(envID)` — *all* non-deleted attachments where this environment is the *consumer* and `dial_policy_id is not null` (proxy **and** dialer), for consumer-env teardown (2d). It covers proxy consumer attachments too, because account/org hard-delete cascade outruns the proxy reaper. Note this is NOT `ListByTunnel` (which is provider-org-scoped); it keys on the attachment's own `environment_id`.
  - **Reaper scope:** `ListExpiredActive` (801-813) gains `and kind = 'proxy'` so the reaper only ever loads proxy rows — dialer rows are never swept and never become permanent reaper work.

### 2c. API spec + regen

The SDK talks **REST via `api.Client`**, not repositories — and the existing attachment endpoints (`listTunnelAttachments`, `deleteTunnelAttachment`) are owner/manager-scoped, so a *consumer* cannot look up or remove its own attachment by key. The natural-key `Attach`/`Detach`/`Dial` contract therefore needs new consumer-scoped operations, not just repo methods. In `internal/api/specs/tunnels/`:

- `connectTunnelRequest` (83-98): make `listenAddress` optional — *absent-only, not `nullable`* (drop from `required`); kind is inferred from presence (omitted → `dialer`), explicit null rejected (clients omit the field). `ConnectTunnel` remains `Attach`'s backend; a `dialer` request omits `listenAddress` and the response is the created-or-existing attachment (idempotent, 2d).
- `tunnelAttachment` (155-209): add `kind` (enum `[proxy, dialer]`, required); make `listenAddress` optional.
- **New: get active dialer attachment by key** — `GET /dialer-attachments?environmentId={ev}&tunnel={nameOrID}` (operationId `getActiveDialerAttachment`; `tunnel` is a name or ID, passed as a **query parameter** rather than a path segment since tunnel names aren't constrained to path-safe characters; resolved by the consumer resolver in 2d). Authorized for the consumer (can-connect), not just the tunnel manager. Responses: `200` with a dedicated `activeDialerAttachment` schema; `404` if none; `401`/`403` on auth. The `200` schema **requires** the `attachment` (a `tunnelAttachment`), `tunnelId`, and `tunnelMode` — `tunnelMode` is required *here* (unlike the optional field on `tunnelAttachment` itself) so `Dial` can enforce stream-only without a second lookup. Backs `Dial`'s validation and idempotent `Attach`'s existing-attachment check.
- **New: detach active dialer attachment by key** — `DELETE /dialer-attachments?environmentId={ev}&tunnel={nameOrID}` (operationId `detachDialerAttachment`), same query-parameter key, authorized for the consumer. Deprovisions the dial policy *then* removes the row (deprovision-before-delete, 2d). Responses: `204`; `404` if none; `401`/`403`. This is the natural-key `Detach` that survives a lost opaque ID.
- Both new operations enforce **two** checks, mirroring `ConnectTunnel`: `requireOwnedEnvironment(environmentId)` (the supplied environment must belong to the authenticated account) **and** can-connect to the tunnel. The *owned* environment ID from the first check is what keys the active-dialer lookup/delete — so a caller can only ever touch its own environment's attachment, never another's by guessing the ID. Negative tests: a same-org and a cross-org `environmentId` owned by a *different* account must both be rejected.
- Regen via `generate_rest.sh`.

### 2d. Controller

This is the heart of Phase 2.

**`connectTunnel.go:14-99` — becomes `Attach`'s backend (idempotent, kind-inferring, cross-org):**
- Infer kind from `listenAddress` presence (absent → `dialer`; trimmed non-empty → `proxy`; empty/whitespace → reject; explicit null rejected — absent-only).
- **Reject `mode = udp`** at attach time for a `dialer` request — the direct dialer path is stream-only (the overlay yields a `net.Conn`), so an unusable `udp` dialer grant must never be provisioned.
- Resolution must use can-connect semantics including **cross-org grants**, but **not** via the existing `lookupTunnelByName` (`tunnel_helpers.go:106-127`): that helper returns a same-org tunnel by name *before* authorization filtering, so an inaccessible same-org tunnel would shadow a cross-org grant, and an owned tunnel plus a same-named cross-org grant would resolve silently to the owned one. Specify a **new consumer resolver** that collects *all* accessible matches for the name — same-org accessible (owned or granted) **and** cross-org granted — dedupes by tunnel ID, and returns 404 on zero, the single match on one, and **fails ambiguous on more than one**. Replace the `limit 1` `GetByNameGrantedToAccount` lookup with a list query (e.g. `ListAccessibleByName`) that filters `not g.deleted` — the current helper omits that, so without it a *revoked* (soft-deleted) grant would still count as an accessible match. ID resolution likewise resolves over can-connect candidates (today `GetTunnel` is same-org). Tests: a same-org-inaccessible tunnel must not shadow a cross-org grant; owned-plus-cross-org same name must fail ambiguous.
- **Idempotency + race cleanup:** for a `dialer` request, first check `GetActiveDialerByEnvironmentAndTunnel`; if one exists, return it (idempotent recovery). Otherwise create the dial policy (`CreateAttachmentDialPolicy`) then insert the row — and on the live-dialer **unique conflict** after the policy was created, **deprovision the just-created policy** before re-reading and returning the existing attachment. This mirrors the existing create-policy-then-insert-then-deprovision-on-failure pattern already in this handler (`connectTunnel.go:~89`); the new wrinkle is treating the unique-conflict branch as a recoverable race, not a hard error.

**`Detach` — by natural key.** `DeleteTunnelAttachment` (`tunnelAttachmentLifecycle.go:37-74`) is by-ID today. The new consumer-scoped delete operation (2c) is keyed on (agent env + tunnel name/ID): it finds the active dialer via `GetActiveDialerByEnvironmentAndTunnel`, deprovisions its dial policy, and removes the row — so a process that lost the opaque ID can still clean up. The consumer-scoped get operation (2c) backs `Dial` validation and idempotent-`Attach`'s existing-row check.

The existing by-ID `DeleteTunnelAttachment` must stay consistent with natural-key `Detach`: when its target is a `dialer` attachment, it delegates to the same dialer-detach helper — deprovision → `detachTunnel` (audit) → `SoftDeleteByID` — so a by-ID delete and a natural-key `Detach` leave the row in the same soft-deleted state. `proxy` attachments keep the existing state-disconnect (history-preserving, reaper-managed) behavior unchanged.

**Reaper (the dialer skip):** `tunnelAttachmentReaper.go:33-64` — with `ListExpiredActive` now filtered to `kind='proxy'` (2b), the reaper inherently skips dialer rows. Proxy attachments keep today's deprovision-plus-`detachTunnel` (stale) path exactly.

**The cleanup paths — cross-org row enumeration, not tag-sweep.** Because the reaper no longer sweeps dialers, every revocation path must remove the dial *policy*. Agora stores `dial_policy_id` on every attachment row — unlike zrok, which only tags the policy and must `CleanupByTag` — so the authoritative and exhaustive cleanup is to **enumerate the affected attachment rows cross-org and deprovision each by its stored `dial_policy_id`**, then remove the rows. This needs no `Find`-limit pagination (you delete by IDs you already hold, not a filter query that defaults to `limit 100`), carries no tag-correctness dependency, and leaves no orphan rows. The enumeration must drop the provider-org filter and key on `tunnel_id` (globally unique, so no org filter is needed). Add to the attachment repository:
- `ListByTunnelCrossOrg(tunnelID)` — all non-deleted attachments for the tunnel, **regardless of organization**. Used by whole-service teardown.
- `ListByTunnelAndAccountCrossOrg(tunnelID, accountID)` — the de-granted account's attachments for the tunnel, cross-org. Used by grant removal. (Note: existing `TunnelGrants.Delete` and `ListByTunnelAndAccount` filter `organization_id = tunnel.OrganizationID`, so they *miss* cross-org consumers entirely — this query and matching grant-row cleanup must be org-agnostic on the consumer side.)
- `ListConsumerAttachmentsWithPolicy(envID)` — all non-deleted attachments where this environment is the consumer and `dial_policy_id is not null` (proxy + dialer). Used by consumer-env disable; covers proxy consumer policies that a hard account/org cascade would otherwise strand before the reaper.

The enumeration needs matching cross-org **deletion** methods, because the existing helpers don't fit — `DeleteByTunnel` and `TunnelGrants.Delete` are provider-org-scoped, and `detachTunnel` only updates state (it does not soft-delete the row):
- `TunnelAttachments.SoftDeleteByIDs(ids)` — soft-delete (`deleted = true`) the enumerated attachment rows regardless of org. Used by the multi-row cleanup paths.
- `TunnelAttachments.SoftDeleteByID(id)` — single-row soft-delete for natural-key `Detach`.
- `TunnelGrants.DeleteCrossOrg(tunnelID, accountID)` — org-agnostic grant-row delete on the consumer side, since `TunnelGrants.Delete` filters by the tunnel's org. (Account-scoped, for grant removal.)
- `TunnelGrants.DeleteByTunnelCrossOrg(tunnelID)` — org-agnostic *whole-tunnel* grant delete, for whole-service teardown, since the existing `TunnelGrants.DeleteByTunnel` is provider-org-scoped and misses cross-org grants.

Each cleanup path's sequence is: **enumerate → deprovision each policy by stored `dial_policy_id` → mark disconnected via `detachTunnel` (state + audit) → soft-delete the same enumerated IDs.** The `detachTunnel` audit/disconnect *is* appropriate on these paths — they are real revocations, distinct from the reaper's no-event stale (which dialer-kind no longer takes at all).

Wire the repository methods into **every** revocation and teardown path. This is the complete matrix — each path follows the deprovision-before-delete ordering above; "soft-delete" means `SoftDeleteByIDs`/`SoftDeleteByID`:

| Path (handler) | Consumer dialer attachments | Grant rows | Provider tunnel (service/bind) | Notes |
|---|---|---|---|---|
| `Detach` (consumer op, 2c) | the caller's one active dialer: deprovision → soft-delete | — | — | natural-key |
| Grant removal (`removeTunnelGrant.go`) | `ListByTunnelAndAccountCrossOrg` → deprovision each → soft-delete | `DeleteCrossOrg(tunnel, account)` | — | account-scoped only; remove→regrant test |
| Tunnel delete (`deleteTunnel.go`) | `ListByTunnelCrossOrg` → deprovision all → soft-delete | `DeleteByTunnelCrossOrg(tunnel)` | deprovision bind/service/SERP + soft-delete tunnel (existing) | whole-service |
| Provider-env disable (`disableEnvironment.go`) | per *owned* tunnel: as Tunnel delete | `DeleteByTunnelCrossOrg` per owned tunnel | per owned tunnel (existing) | dedupe IDs with the consumer row |
| Consumer-env disable (`disableEnvironment.go`) | `ListConsumerAttachmentsWithPolicy(env)` → deprovision → soft-delete | — | — | all the env's own attachments with a policy (proxy + dialer) |
| Session teardown (`closeSession.go`) | **add** `ListByTunnelCrossOrg` → deprovision → soft-delete (none today) | **add** `DeleteByTunnelCrossOrg` (none today) | deprovisions service/bind today; + soft-delete | new enumeration + grant delete |
| Account delete (`deleteAccount.go`) | via per-environment internal env-disable | via internal env-disable | via internal env-disable | **cascading** (iterate the account's envs → `disableEnvironmentInternal` → delete account) **but guarded on L2 sessions** — refuse while the account participates in any active session with a backing tunnel; see rule 2 |
| Org delete (`deleteOrganization.go`) | — | — | — | **made guarded** — today an *unguarded* FK cascade; change `Organizations.Delete` to lock the org row (`SELECT … FOR UPDATE`) → check `accounts` under the lock → `ErrConflict` if any, else delete. Empty it account-by-account first. See rule 3 |
| Reaper (`tunnelAttachmentReaper.go`) | **proxy only**: deprovision + `detachTunnel` (stale); **dialer-kind skipped entirely** | — | — | dialer rows never swept — no policy removal, no stale, no event |

Three cross-cutting rules:

1. **Dedupe within a path.** `DisableEnvironment` enumerates both an environment's owned-tunnel consumers *and* its own dialer attachments; these overlap when an environment dials a tunnel it owns. Collect attachment IDs from both enumerations, dedupe, and run the deprovision→soft-delete sequence once per ID (no double-deprovision, double-audit, or second-soft-delete not-found).
2. **Account delete composes the per-environment path through an internal helper, not the HTTP handler.** Account delete is changed from its current guarded form (it refuses today while environments remain) to a cascade. Factor the cleanup-aware teardown into an internal function — `disableEnvironmentInternal(ctx, tx, env)` or similar — that runs the enumerate→deprovision→delete and takes the environment-scope lock; the `DisableEnvironment` HTTP handler and the account-delete cascade both call *it*. The cascade must not invoke the handler method directly: that re-runs `requireAccountPrincipal` and the owner check (`disableEnvironment.go:15-34`) against the *deleting admin* rather than the environment's owner, and returns `api.*Res` response shapes instead of a plain error. The cascade follows zrok's `deleteAccount.go` pattern — `FindEnvironmentsForAccount` → internal env-disable per env → then delete — so it inherits the matrix automatically and never raw-cascades stranded Ziti resources. **But account delete is also *guarded* on L2 session participation** — the cascade only reaches the account's *owned* side. A session's backing tunnel is created under its **provider** (`acceptSession.go:133-135`), so deleting a *consumer* account would cascade its session rows via the `sessions` FK while leaving the provider-owned tunnel + bind policy live and unreclaimable (the handle `teardownSession` needs is gone). Until full session teardown lands (deferred — see Out of scope), account delete refuses (`ErrConflict`) while the account participates — as consumer **or** provider — in any *active* session (one with a backing tunnel). Make it race-safe with the same lock-then-check as org delete: `SELECT … FOR UPDATE` the account row, then check `sessions` under the lock; the `sessions`/grant → `accounts` FK makes a concurrent `proposeSession`/`AcceptSession` take a key-share lock on that row and serialize against the delete (an accept that loses the race fails its grant insert and deprovisions its just-made tunnel), with no change to the session paths. Test: deleting an account that consumes an active session returns conflict and leaves the provider tunnel + bind policy intact. **Org delete is made guarded** (not cascade-aware): it neither cascades nor enumerates accounts — but it is *not* guarded today. `Organizations.Delete` (`repositories.go:65`) issues a bare `delete from organizations where id=$1`, and because the tenant FKs are `on delete cascade` (rule 3) that silently deletes the whole tenant and returns success — it never raises `ErrConflict` (the existing `isForeignKeyConstraint` branch is dead code under a cascading FK). Replace it with a *transactional guard that locks first, then checks*: in one transaction, `SELECT … FROM organizations WHERE id=$1 FOR UPDATE` to lock the org row, then read `accounts` under that lock — if any exist, return `ErrConflict`; otherwise delete the org while still holding the lock. The single-statement `delete … where not exists (accounts)` is **not** sufficient: under READ COMMITTED the `NOT EXISTS` can be evaluated before the delete blocks on a concurrent `CreateAccount`'s FK key-share lock, and after that insert commits the delete may proceed via EvalPlanQual *without* re-checking the subquery, cascading the just-created account. The lock-first ordering forces the account check to run as a fresh read after the lock is held, so a racing `CreateAccount` is either seen (→ `ErrConflict`) or fails its own FK insert against the now-deleted org. `CreateAccount` needs no change. Do **not** rely on a foreign-key conflict or on single-statement subquery atomicity. So an org is emptied account-by-account first; the asymmetry is deliberate — the destructive cascade is allowed at account scope (per the requirement) but withheld at org scope, where a single call would otherwise tear down an entire multi-account tenant.
3. **`ON DELETE CASCADE` makes every parent-row delete dangerous.** The tenant FKs — `accounts→organizations`, and `environments`/`tunnels`/`grants`/`attachments`/`serves` → accounts/orgs/environments — are all `on delete cascade` (`0001_core_layer1.sql`). Deleting any parent row therefore deletes its children at the DB layer *without* running the app-side Ziti deprovisioning, stranding identities/bind-policies/services/dial-policies with no row left to drive cleanup. So every Ziti-bearing parent delete must be either **cascade-aware** — tear the children down app-side first (deprovision Ziti, then delete) so the FK cascade finds nothing left: account delete, env disable, tunnel delete — or **guarded** — atomically refuse while children exist: org delete (rule 2). A bare parent-row `DELETE` that leans on the FK cascade is never acceptable at a Ziti-bearing level. Regression test: deleting a populated organization returns conflict and cascades nothing (assert its accounts/environments/tunnels and a sentinel Ziti resource survive); deleting an emptied organization succeeds; and a `CreateAccount` racing a concurrent org-delete either is refused (org survives, account intact) or fails its insert against the deleted org — never a silent cascade.

**Ordering — deprovision before delete (retry-safety).** With the reaper no longer a backstop, every cleanup path must deprovision the Ziti dial policy *before* deleting (or soft-deleting) its DB row, so a failed deprovision leaves the row intact and the whole operation retryable. This is a real correction for grant removal especially: `RemoveTunnelGrant` (`removeTunnelGrant.go:30-57`) deletes the grant row *first* today, so a policy-deprovision failure would strand a live dial policy with no grant row left to drive a retry and — with no reaper — no other cleanup. Reorder all paths to: enumerate → deprovision each policy by its stored `dial_policy_id` → only then delete the grant/attachment rows. Add a fake-lifecycle test where deprovision fails and assert the grant and attachment remain present and retryable. **Session teardown needs particular care:** `teardownSession` (`closeSession.go`) marks the session closed first and returns early for already-closed sessions, so the dial-policy/attachment/grant cleanup must complete **before** the final `MarkClosed` — otherwise a failed deprovision is unretryable (retries and the duration reaper both skip an already-closed session). Add a deprovision-failure retry test for `CloseSession` / the duration reaper.

**Admission serialization (race-safety) — a lock hierarchy keyed to enumeration scope.** Deprovision-before-delete opens a window: between a teardown path *enumerating* the rows it will remove and *committing* their deletion, a concurrent admission can create a new resource the enumeration already passed — leaving a live Ziti object (dial policy, bind policy, or identity) with no row and no reaper to sweep it (a permanent zero-trust hole). The governing rule: **the lock must be scoped to whatever the teardown enumerates over.** A child created after the parent's enumeration but before the parent's deletion is invisible to that enumeration, so admission and teardown must rendezvous on the *parent* — the only thing both name, since the child has no id yet. The matrix has three teardown scopes, so three lock scopes, and they nest:

1. **Tunnel scope** — the dialer's own race. Grant removal, tunnel delete, session teardown, and per-owned-tunnel cleanup in env-disable all enumerate *attachments by `tunnelID`*. `Attach` and each of those take the same transaction-scoped lock keyed by `tunnelID` (`pg_advisory_xact_lock(hash(tunnelID))`, or `SELECT … FOR UPDATE` on the tunnel row): `Attach` re-checks can-connect *under the lock* before creating a policy; each teardown *enumerates* under the lock. An `Attach` racing a just-committed grant removal re-checks, sees the grant gone, and fails; an `Attach` that wins is caught by the teardown that follows. **This scope cannot fold up to the account**: the dial is cross-org — provider (account A) removes the grant, consumer (account B) attaches — so the two share no account, and the tunnel is the only object both name; an account lock would also serialize every attach to every tunnel a busy provider owns. The tunnel is the minimal correct rendezvous.

2. **Environment scope** — new tunnels/attachments vs. env teardown. `DisableEnvironment(E)` enumerates *tunnels by `environmentID`* (provider side) and *its own consumer attachments by `environmentID`* (consumer side). Any path that provisions an overlay object or inserts a tunnel/attachment row **bound to environment E** but commits after those reads (or after E is soft-deleted) is missed — its service/bind or dial policy strands, with no path keyed to the deleted environment left to reclaim it. The tunnel lock can't help: a new tunnel has no id at enumeration time, and a new consumer attachment rendezvouses on *E*, not on its (other-env) tunnel. So the **admitter set is every env-bound provisioning path, not just the Layer 1 handler.** Verified against the code (the only non-test callers of `Tunnels.Create` / `tunnelLifecycle.Provision` / `CreateAttachmentDialPolicy`), that set is exactly three: `CreateTunnel` and **`AcceptSession`** — both provision a Ziti service/bind and `Tunnels.Create` a row for `req.EnvironmentId` (`createTunnel.go:73,105`; `acceptSession.go:107,147`, the Layer 2 session-accept path, which bypasses the `CreateTunnel` handler) — plus the consumer side of `Attach` (`connectTunnel.go:57,76`). Each takes the lock keyed by `environmentID`, re-checks E is live/owned *under the lock*, and holds it through the external provision (or `CreateAttachmentDialPolicy`) **and** the row-writing transaction — for `AcceptSession`, through `Provision` plus the tx doing `Tunnels.Create`/grant/`MarkActive`; on a lost re-check it aborts, and `AcceptSession`'s existing failure path already deprovisions and marks the session closed. `DisableEnvironment` takes the same lock before enumerating — today its `ListByEnvironment` (`disableEnvironment.go:36`) runs before any lock and outside the tx, so it must move inside.

3. **Account scope** — the same race one level up, and the top of the ladder. Account delete (now cascading) enumerates *environments by `accountID`* (`FindEnvironmentsForAccount`); a `CreateEnvironment` in that account, committing in the same gap, strands a fresh environment identity. By the identical rule, `CreateEnvironment` and account delete take a lock keyed by `accountID`; the admitter re-checks the account is live, the teardown enumerates under it. **The hierarchy terminates here.** Organization is the top tenant boundary, and org-delete is *made guarded* (rules 2–3): a transactional lock-then-check delete (`SELECT … FOR UPDATE` the org row, then refuse if any account exists), so it never enumerates accounts and there is no org-scope race to serialize. The guard's own race against `CreateAccount` is closed by that row lock forcing a fresh post-lock account check, not by an admission lock — and `CreateAccount` provisions no Ziti object, so a refused org-delete strands nothing. If org-delete is ever made cascade-aware like account-delete, the identical rule adds an organization-scope lock above account; until then, account is the outermost scope.

**Order and holding.** The scopes are hierarchical (account ⊃ environment ⊃ tunnel), so locks are acquired **outer-to-inner — account, then environment, then tunnel** — a canonical global order that cannot deadlock (multiple keys within one scope are taken in sorted order). So `Attach` takes its consumer-environment lock then the tunnel lock; `DisableEnvironment` takes E's lock then each owned tunnel's in sorted order; account delete takes the account lock then composes the internal env-disable per environment, inheriting the inner locks. Every lock is transaction-scoped and held through the *whole* critical section — the external Ziti op **and** the matching DB mutation — so nothing interleaves between an overlay op and its row write. One concurrency test per scope: `Attach` vs. grant-removal/tunnel-delete (tunnel); `CreateTunnel`/`AcceptSession`/`Attach` vs. `DisableEnvironment` (environment); `CreateEnvironment` vs. account delete (account) — each must leave no stranded live Ziti object.

Because each path deprovisions before it deletes, a failed deprovision is retryable; and because each path deletes the rows it enumerates, nothing orphans. Tags are demoted to an **audit / backstop** role: a deferred periodic GC may tag-sweep for any policy that somehow has no row (a prior failed write), and the tag vocabulary (the existing `automation.AgoraTags` keys — `agoraTunnelId`, `agoraAccountId`, `agoraAttachmentId`, `agoraResourceKind`, plus an optional `agoraAttachmentKind`) is kept consistent for that GC and for debugging — but it is no longer the load-bearing revocation path.

### 2e. Ziti automation

No new primitive is needed for the revocation boundary: the existing `Deprovision(DeprovisionTunnelSpec{DialPolicyID: ...})` (`automation/tunnel.go:146-168`) removes a single dial policy by its stored ID, which is exactly what the row-enumeration cleanup (2d) calls per attachment. The only optional addition is a `DeprovisionByTag` for the *deferred* orphan-policy GC backstop — and if built, it must be **exhaustive**: `resources.go:deleteWithFilter` calls `Find` once and `FilterOptions.limit()` defaults to 100, so a one-shot tag delete silently leaves matches past the first page; the GC must repeat-until-empty. Keep the existing `automation.AgoraTags` keys (`agoraTunnelId`, `agoraAccountId`, `agoraAttachmentId`, `agoraResourceKind`) consistent for that GC and for audit.

### 2f. SDK surface + cancellable dial

- `tunnel.Attach(ctx, a, nameOrID) (*Attachment, error)` — wraps `ConnectTunnel` with a `dialer` request (omit `listenAddress`); idempotent by natural key (server-side, 2d).
- `tunnel.Detach(ctx, a, nameOrID) error` — wraps the consumer-scoped delete-by-key operation (2c), so a process that lost the `*Attachment` handle can still detach by (env + tunnel). (May also accept the `*Attachment` returned by `Attach` for ergonomics, but the key is what's sent.)
- `tunnel.Dial(ctx, a, nameOrID) (net.Conn, error)` — resolves cross-org (can-connect), validates an active dialer attachment exists via the consumer-scoped get (2c), rejects `mode = udp` (stream-only, using the `mode` on the get response), builds the overlay, returns `overlay.DialContext(ctx, tunnel.ID)` — a single `net.Conn` (zrok's `NewDialer` shape). The HTTP-transport use is the caller's trivial `DialContext` closure.
- **Cancellable-dial prerequisite:** `OverlayContext.Dial` is contextless today (`overlay.go:37-47`, `ziti.DialWithOptions` with no ctx). Add `DialContext(ctx, serviceName)` to the `OverlayContext` interface and `openZitiContext`. Prefer a context-capable ziti dial if the SDK version provides one (real cancellation to the fabric). Otherwise the **bounded fallback** is the contract: run the ziti dial in a goroutine and select on `ctx.Done()` vs. the result; on cancel/deadline, return `ctx.Err()` immediately, and any connection that arrives late is `Close()`d and discarded — never returned to the caller; the orphaned background attempt runs out its own `ConnectTimeout` and is dropped. The guarantee is therefore: *the caller never receives a late/leaked conn and the call returns promptly on cancel*, even though the underlying attempt may finish in the background. Tests: canceled and deadline-expired dials return promptly with `ctx.Err()` and leak no late connection. This is a Phase-2 gate before `Dial` ships.

### 2g. Tests (Phase 2)

- Migration: conditional `listen_address` invariant; partial unique index allows reattach after a row goes `disconnected`; `NOT NULL kind`.
- `Attach`: kind inference; idempotent return of an existing active dialer by env+tunnel; **the race test** — a losing concurrent `Attach` deprovisions its own just-created dial policy (assert no orphan policy survives); cross-org grant resolution; ambiguous-name failure.
- Cleanup: tunnel-delete and session-teardown remove a cross-org consumer's dial policy (row enumeration by stored id); grant-removal removes only the de-granted account's policy and leaves other consumers' intact; consumer-env-disable removes the environment's own dialer policies.
- Reaper: a stale dialer row is **not** swept (no deprovision, no detach event, stays `active`); a stale proxy row still is.
- `Dial`: cancellable — a canceled or deadline-expired dial returns promptly and leaves no late connection (requires the `DialContext` overlay primitive).

---

## Cross-cutting

- **The OpenZiti revocation dependency is stated, not tested.** The active-stream cut on policy removal rests on OpenZiti terminating established sessions when the authorizing policy is removed — a known, accepted overlay behavior (see the spec's normative Layer 1 dependency). No integration test for it is in scope, by decision.
- **Display/status semantics:** a `dialer`-kind attachment sits at `state=active` for life with no heartbeat; dashboards and `last_heartbeat_at`-based "live" counts must read it kind-awarely as an active *grant*, not a live process. Non-normative; UI's call on presentation.
- **Generated code:** never hand-edit `internal/api/oas_*_gen.go` or `sdk/agent/networkpb/*`; change the spec/proto and regenerate (`bin/generate_rest.sh`, `bin/generate_pb.sh`). The thin primitives need no proto change (they don't use the gRPC runtime).
- **UI type sync (per `CLAUDE.md`):** the dashboard's `ui/src/lib/api/types.ts` is hand-maintained and does **not** regenerate from the OpenAPI spec. Sync the new `kind` and optional `backendTarget`/`listenAddress` fields there manually alongside the REST regen, or the dashboard can't render direct tunnels / direct dialers even after the contract is correct.

## Verification (end-to-end)

- Unit/integration per slice above (persistence tests use the Postgres testcontainers harness).
- Phase-1 end-to-end: bring up the demo stack, provision a direct tunnel, `Listen`, serve an HTTP handler on it, dial it from a granted consumer — the `examples/macro-pulse` / demo-bootstrap path is the natural home for a worked example mirroring zrok's `http-server` example.
- `go build ./...` and `go vet ./...` after each slice; `go test ./...` when a slice touches shared wiring.

## Out of scope (carried from the spec)

- A *managed* direct lifecycle (heartbeated, supervised, auto-relisten direct serve) — a distinct heavier surface built on these primitives, not folded in. `EnsureServed`/`EnsureConnected` remain the managed proxy option.
- UDP on the raw listener (a packet-conn variant).
- Promoting tolerated orphaned-dialer-row cleanup into an active GC.
- **Full L2 session teardown on tenant deletion** — having account/org deletion *enumerate and tear down* the L2 sessions a tenant participates in (and the provider-owned backing tunnels of consumed sessions) under the tunnel lock, instead of the interim guard that refuses deletion while such sessions exist (rule 2). A Layer 2 deletion-lifecycle work item: it needs the session state machine, not just the L1 cleanup matrix.
