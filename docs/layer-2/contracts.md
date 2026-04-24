# Layer 2: Contracts

Tier: A. Implementation-ready. See [foundation.md](foundation.md) for cross-cutting decisions.

## Purpose

A contract defines the negotiated terms under which a session operates. It bounds the session on dimensions the platform can observe — duration, envelope count, allowed message types, workgroup and trust preconditions, and the access mode that gates acceptance. Contracts exist so providers can publish engagement terms declaratively and so the controller can evaluate consumer requests against those terms without the provider needing to be online to decide.

Contracts are declarative in MVP: a fixed set of fields the controller understands and evaluates. Programmatic contracts (arbitrary code or DSLs evaluated per engagement) are explicitly deferred.

## Relationship To Other Layer 2 Concepts

- **Advertisements** reference a contract as their `contract_requirements` — the terms a consumer must satisfy to engage. Advertisements gain a nullable `contract_id` column via this slice's migration; the workgroups and catalog/advertisements slices that shipped earlier did not carry the column.
- **Sessions** carry a **frozen snapshot** of the contract at engagement time. The snapshot is the authority for session enforcement; later changes to the source contract do not affect in-flight sessions. The sessions slice reserved a nullable jsonb `contract_snapshot` column on the sessions table; this slice populates and reads it.
- **Workgroups** may be listed in a contract's `required_workgroup_memberships` above and beyond the scoping of the advertisement itself. A consumer must hold non-deleted membership in **every** listed workgroup to satisfy the contract.
- **Envelopes** are subject to contract enforcement: disallowed message types will be rejected and envelope-count bounds tracked once envelopes flow. Those two dimensions are **persisted by the contracts slice but enforced by the envelopes slice**, because they require envelope-level observability the contracts slice does not wire.

## Data Model

- **Contract** (MVP shape, `schema_version = 1`)
  - `id` (`con_...`)
  - `account_id`, `organization_id` — the owning identity (the account that created the contract)
  - `name` (case-insensitive, unique within `account_id`)
  - `description` (optional)
  - `schema_version` (integer, MVP `1`)
  - `max_duration` — session-level cap, expressed as an integer number of seconds; `0` means "no cap"
  - `max_envelope_count` — session-level cap; `0` means "no cap". Persisted by this slice; enforced by the envelopes slice.
  - `allowed_message_types` — list of strings. **Empty list means "no types allowed"** (the defensive default). To allow every type, include a `"*"` wildcard. Persisted by this slice; enforced by the envelopes slice.
  - `required_workgroup_memberships` — list of `wg_...` IDs. Consumer must hold non-deleted membership in all of them.
  - `maturity_requirements` — structured object. MVP shape: `{min_account_age_days: int}`. The field is optional; absent means no maturity gating. Richer trust dimensions (prior-session success counts, organization trust flags, external attestations) are post-MVP.
  - `access_mode` — enum `open` | `approval_required`. (`policy_evaluated` is post-MVP.) MVP-default: `approval_required`. `access_mode` is recorded on the snapshot and surfaced to the provider's session handler; the controller does **not** differentiate behavior on this field in MVP — every session accept still runs through the provider's handler. `open` vs `approval_required` is a handler-level hint that the handler can read from the snapshot.
  - `created_at`, `updated_at`

Contracts are first-class persisted resources with their own IDs and API. One contract may be referenced by many advertisements.

A contract referenced by an active advertisement cannot be deleted; the advertisement must be updated to remove the reference (or retracted) first.

## Lifecycle / State Machine

Contracts themselves are largely static. Their lifecycle is:

- **created** — a provider-side account creates the contract via `POST /v1/contracts`.
- **updated** — the owning account may mutate fields via `PATCH /v1/contracts/{conId}`. Updates do **not** flow into in-flight sessions; the session's frozen snapshot is authoritative for its lifetime.
- **deleted** — the owning account may delete a contract via `DELETE /v1/contracts/{conId}`, **but only when no active advertisement references it**. Sessions that snapshotted the contract continue to run under the snapshot after delete.

There are no `active/closed` states; contracts are either extant or deleted.

## Authorization

- **Create / update / delete**: the owning account (`account_id`) only.
- **Describe**: the owning account may always describe. Non-owners may describe a contract if they share a workgroup with at least one active advertisement that references it. This mirrors the advertisement visibility rule — contracts "inherit" visibility from the advertisements that point at them.
- **List-mine**: `GET /v1/contracts` returns only the caller's own contracts.

Admin moderation of contracts (force-delete by platform admin) is out of MVP scope.

## Visibility Rules

Consistent with advertisements:

- Owner always sees their own.
- A non-owner sees a contract iff they hold a non-deleted membership in at least one workgroup referenced by an active, non-retracted advertisement whose `contract_id` equals the contract's ID.
- Non-visible contracts return `404 not_found` on direct describe attempts. Existence is not disclosed.
- `required_workgroup_memberships` in a contract describe response is **always returned in full** (it is a contract term, not a membership boundary to hide). Consumers use it to understand what they need in order to engage.

## Controller API Surface

Module: `internal/api/specs/contracts/`. All endpoints use `accountTokenAuth`. Property names are camelCase; timestamps are RFC3339; the error response schema is the existing `error` from `common/schemas.yml`.

### Resource shape

**Contract** (response):

```
{
  "id": "con_...",                           // pattern: ^con_[a-z0-9]{12}$
  "accountId": "ac_...",
  "organizationId": "org_...",
  "name": "<string>",
  "description": "<string>",                 // optional
  "schemaVersion": 1,
  "maxDurationSeconds": <int>,               // 0 = no cap
  "maxEnvelopeCount": <int>,                 // 0 = no cap
  "allowedMessageTypes": ["<string>", ...],  // empty = no types allowed
  "requiredWorkgroupMemberships": ["wg_..."],
  "maturityRequirements": {
    "minAccountAgeDays": <int>               // optional; absent = no gate
  },
  "accessMode": "open" | "approval_required",
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>"
}
```

### Endpoints

**`POST /v1/contracts`** — create a new contract.

- request body:
  ```
  {
    "name": "<string>",                      // required
    "description": "<string>",               // optional
    "maxDurationSeconds": <int>,             // optional; defaults to 0 (no cap)
    "maxEnvelopeCount": <int>,               // optional; defaults to 0 (no cap)
    "allowedMessageTypes": ["<string>"],     // optional; defaults to empty
    "requiredWorkgroupMemberships": ["wg_..."], // optional
    "maturityRequirements": {"minAccountAgeDays": <int>}, // optional
    "accessMode": "open" | "approval_required"  // optional; defaults to "approval_required"
  }
  ```
- response `201`: the created `Contract`.
- errors:
  - `400 invalid_request` — malformed field shapes, unknown `accessMode`
  - `400 unknown_workgroup` — a workgroup ID in `requiredWorkgroupMemberships` does not exist
  - `403 not_a_workgroup_member` — the caller is not a member of one or more of the listed workgroups (same policy as advertisements: you cannot reference workgroups you cannot see)
  - `409 name_in_use` — the caller already owns a contract with the same case-insensitive name
  - `401`, `500`

**`GET /v1/contracts`** — list the caller's own contracts.

- response `200`: array of `Contract`, ordered by `updated_at` descending.

**`GET /v1/contracts/{conId}`** — describe a contract (visibility-enforced).

- response `200`: `Contract`.
- errors: `401`; `404 not_found` when not visible.

**`PATCH /v1/contracts/{conId}`** — update a contract (owner only).

- request body: any subset of the create fields; omitted fields are unchanged.
- response `200`: the updated `Contract`.
- errors:
  - `400` (same as POST)
  - `403 not_owner`
  - `404 not_found`
  - `409 name_in_use`
  - `401`, `500`

**`DELETE /v1/contracts/{conId}`** — delete a contract (owner only).

- response `204` on success. Idempotent on non-existent IDs when the caller is known-owner (but not on IDs owned by someone else — those return `404`).
- errors:
  - `409 contract_in_use` — an active advertisement still references the contract; delete blocked.
  - `401`; `403 not_owner`; `404 not_found`; `500`.

### Advertisement integration (retrofit)

The contracts slice's migration adds a nullable `contract_id` column to `advertisements`. The advertisement publish/update endpoints are extended:

- `POST /v1/advertisements` — optional `contractId` field in the request body.
- `PATCH /v1/advertisements/{advId}` — optional `contractId` (set to `null` to clear).
- `GET /v1/advertisements/{advId}` response includes `contractId` (nullable string).

On publish/update, the controller validates that (a) the contract exists, (b) the caller owns the contract, and (c) the caller is a member of every workgroup in the contract's `required_workgroup_memberships` **at publish time**. Ongoing membership churn is not re-validated until the advertisement is updated again.

### Session integration (snapshot)

On `POST /v1/sessions/{id}/accept`, the controller evaluates admission:

1. If the advertisement has a `contract_id`, load the contract.
2. Evaluate admission-time terms against the **consumer**:
   - For each workgroup in `required_workgroup_memberships`: the consumer account must hold a non-deleted membership. Failure → `403 contract_violation_memberships`.
   - If `maturityRequirements.minAccountAgeDays` is set: consumer account's `created_at` must be at least that many days in the past. Failure → `403 contract_violation_maturity`.
   - `access_mode` is recorded on the snapshot; it does not gate this API call.
3. If admission passes, the controller snapshots the contract (frozen JSON) into the session's `contract_snapshot` column alongside the existing tunnel provisioning.
4. Duration enforcement is driven by a **session-duration reaper** that checks `active` sessions against their snapshot's `max_duration_seconds`. On expiry, the reaper transitions the session to `closed(contract_violation)` and deprovisions the backing tunnel via the same teardown path as participant close.

The reaper runs alongside the existing tunnel-attachment and tunnel-serve reapers, under the same lifecycle. MVP cadence: check every 15 s, no grace period beyond the contract's own `max_duration_seconds`.

`GET /v1/sessions/{id}` response is extended with a `contractSnapshot` field when populated. The field is a nested object shaped like `Contract` (minus the resource timestamps and account IDs) — exactly what was frozen at accept time.

## Agent / Runtime Surface

None. Contracts are entirely controller-evaluated for admission and duration in MVP. The provider's session handler may read `sess.ContractSnapshot` (added to `*session.Session`) and branch on `access_mode` or any other snapshot field if it wants handler-side logic; the controller neither requires nor enforces that the handler does so.

## CLI Surface

Per [foundation.md](foundation.md), contracts do not get a standalone top-level command in MVP; contract inspection nests under `agora session describe` and `agora advertise describe`.

The contracts slice therefore ships **no new top-level verb**. Contract management lives under `agora advertise`:

`agora advertise publish ... --contract <name|con_...>`

- optional flag; if supplied, references a contract owned by the same account (resolved by name via `ListContracts`-equivalent, or passed as `con_...` directly).

`agora advertise update ... --contract <name|con_...|-none->`

- optional; `--contract -none-` clears the reference.

`agora advertise describe <name|adv_...>` — the describe output gains a "Contract" section when the advertisement has one.

`agora session describe <ses_...>` — the describe output gains a "Contract snapshot" section when the session has one.

Contracts **are** manageable via REST (create/update/delete/list/describe) without a CLI wrapper. A future slice may add `agora contract` subcommands if demand warrants; the REST surface is the authoritative interface in MVP.

## SDK Surface

No new SDK package. Contract snapshots surface through the existing session handle: `*session.Session` gains an optional `ContractSnapshot` field (populated when the session's advertisement referenced a contract). Provider handlers and consumers can read the snapshot directly; there is no separate "contracts" helper package in this slice.

## Failure Modes

All user-visible failures emit a structured `df/dl` log entry with `contract_id` (when known), `session_id` (when applicable), and the failure reason.

- **Missing or invalid auth token** → `401 unauthorized`.
- **Contract name collision within owner** → `409 name_in_use`.
- **Contract references unknown workgroup** → `400 unknown_workgroup`.
- **Contract references workgroup the caller is not a member of** → `403 not_a_workgroup_member`.
- **Delete a contract still referenced by an active advertisement** → `409 contract_in_use` (caller must update/retract the ad first).
- **Advertisement publish/update referencing a non-owned or non-existent contract** → `400 unknown_contract`.
- **Session accept fails admission — missing required workgroup membership** → `403 contract_violation_memberships`. Session transitions to `closed(contract_violation)` with a structured `close_detail`.
- **Session accept fails admission — maturity** → `403 contract_violation_maturity`. Same close path.
- **Session duration exceeds contract's `maxDurationSeconds`** → session transitions to `closed(contract_violation)` via the duration reaper. `close_detail = "max_duration_exceeded"`.
- **Update or delete by non-owner** → `403 not_owner`.
- **Describe by non-visible caller** → `404 not_found`.
- **Controller store error** → `500 internal`.

## Acceptance Criteria

- **Create.** Account can `POST /v1/contracts` with all supported fields and receive a `con_...` ID.
- **Create with unknown workgroup** → `400 unknown_workgroup`.
- **Create with workgroup the caller is not a member of** → `403 not_a_workgroup_member`.
- **Name uniqueness.** Two contracts owned by the same account cannot share a case-insensitive name → `409`.
- **List-mine.** `GET /v1/contracts` returns only the caller's contracts.
- **Describe by owner.** Owner can describe their own contracts.
- **Describe by non-owner with shared workgroup.** A non-owner who is a member of a workgroup scope of an advertisement referencing the contract can describe it.
- **Describe by non-visible caller** → `404`.
- **Update.** Owner can update supported fields; response reflects updates; `updated_at` advances.
- **Update by non-owner** → `403 not_owner`.
- **Delete without references.** Owner can delete a contract not referenced by any active advertisement → `204`.
- **Delete while referenced** → `409 contract_in_use`.
- **Advertisement publish with contract.** Publishing an advertisement with `contractId` referring to a contract owned by the caller succeeds.
- **Advertisement publish with non-owned contract** → `400 unknown_contract`.
- **Session admission: required workgroup membership.** Propose + accept for a session whose contract requires membership in `wg_x` fails with `403 contract_violation_memberships` if the consumer does not hold membership; the session transitions to `closed(contract_violation)`.
- **Session admission: maturity.** If `minAccountAgeDays = 365` and the consumer's account is 1 day old, accept fails with `403 contract_violation_maturity`; the session transitions to `closed(contract_violation)`.
- **Session admission: pass-through.** If the contract imposes no admission terms the consumer fails, the session reaches `active` exactly as it would without a contract.
- **Snapshot populated.** `GET /v1/sessions/{id}` for an active session returns a populated `contractSnapshot` matching the contract at accept time; subsequent `PATCH /v1/contracts/{id}` on the source contract does not change the snapshot.
- **Duration reaper.** A session with `maxDurationSeconds = 2` proposed, accepted, and left running for ~4 seconds observes the duration reaper close it with `close_reason = contract_violation` and `close_detail = "max_duration_exceeded"`. The backing tunnel is deprovisioned.
- **Zero max_duration is unbounded.** A session with `maxDurationSeconds = 0` is never reaped for duration.
- **Persistence schema.** New migrations cover the `contracts` table and the `advertisements.contract_id` nullable column.
- **Tests.** `go test ./...` passes including new persistence and controller HTTP integration tests.

## Out Of Scope For First Slice

- **Programmatic contracts** (arbitrary code or DSL evaluated per engagement) — post-MVP.
- **`access_mode = policy_evaluated`** — post-MVP.
- **Mid-session contract renegotiation** — post-MVP.
- **Contract content versioning** (historical versions of a single contract) — post-MVP; only `schema_version` is tracked.
- **Cross-organization shared contract templates** — post-MVP.
- **Controller-enforced envelope count and `allowed_message_types`** — persisted by this slice; enforced by the envelopes slice.
- **Advanced maturity/trust dimensions** beyond account age — post-MVP.
- **CLI `agora contract` standalone verb** — REST-only in MVP.
- **Per-envelope contract-violation rejection** (returning an error envelope rather than closing the session) — post-MVP.

## Open Questions

None. All concept-level design questions are resolved. The contracts slice is implementation-ready.
