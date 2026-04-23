# Layer 2: Advertisements

Tier: A. Implementation-ready. See [foundation.md](foundation.md) for cross-cutting decisions.

## Purpose

An advertisement is an agent's declaration of what it can do and how to engage with it. Advertisements let a participant publish capabilities to the catalog so other participants in shared workgroups can find and engage them without prior out-of-band coordination. The advertisement is the durable record of intent; the catalog is the query surface over the set of advertisements.

Advertisements are persistent and tied to the advertising **account**, not to a specific environment or session. If the advertising participant disconnects and reconnects from a different environment enrolled to the same account, its existing advertisements remain associated with its identity — they are not re-created on every reconnect. Any environment enrolled to the owning account may act as the advertising runtime; multiple environments may run simultaneously and serve sessions for the same advertisement.

## Relationship To Other Layer 2 Concepts

- **Workgroups** determine where an advertisement is visible. An advertisement declares one or more workgroup scopes; visibility to a given caller is the intersection of those scopes with the caller's workgroup memberships.
- **Catalog** queries return advertisements filtered by the caller's workgroup memberships. The catalog's `search` is the discovery entry point; the per-advertisement `describe` lives on the advertisement surface itself, accessible via both `agora advertise describe` and `agora catalog describe`.
- **Contracts** will reference advertisements as their requirements once the contracts slice ships. **In the workgroups + catalog/advertisements slice, advertisements have no `contract_requirements` field**; the contracts slice's migration will add it.
- **Sessions** are proposed against an advertisement. The advertisement's contract requirements (once contracts ship) are evaluated when a session is proposed.

## Data Model

- **Advertisement**
  - `id` (`adv_...`)
  - `account_id`, `organization_id` — the owning identity
  - `name` (case-insensitive, unique within `account_id`)
  - `description` (optional)
  - `capabilities` — list of `{name, description, metadata}` records (see below)
  - `interaction_patterns` — list of `{kind, custom_pattern}` records (see below)
  - `workgroup_scopes` — list of `wg_...` IDs the advertisement is visible in
  - `schema_version` (integer, MVP: `1`)
  - `status` (`active`, `retracted`)
  - `created_at`, `updated_at`, `retracted_at` (nullable)

Advertisements are soft-retracted (`status = retracted`) rather than hard-deleted, so in-flight references from sessions or audit logs remain resolvable.

### Capabilities shape

Each capability is a structured record:

```
{
  "name": "<string>",                       // required; identifies the capability
  "description": "<string>",                // optional human-readable description
  "metadata": {"<key>": "<value>", ...}     // optional opaque key-value pairs
                                            // (used for A2A-aligned metadata)
}
```

`metadata` is a flat map of strings to strings, opaque to the controller. This is where A2A-style structured metadata is carried at the per-capability level. The controller does not interpret or filter on metadata content in MVP; it is round-tripped to consumers as-is.

### Interaction patterns shape

Each interaction pattern is a structured record:

```
{
  "kind": "<enum>",                         // required; one of the values below
  "custom_pattern": "<string>"              // required iff kind=custom; otherwise omitted
}
```

`kind` enum values for MVP: `request-response`, `stream`, `broadcast`, `custom`. When `kind = custom`, `custom_pattern` carries a freeform identifying string; otherwise it is omitted. The enum can be extended in future slices without breaking existing data.

### Workgroup-scope constraint

Every workgroup ID listed in `workgroup_scopes` must reference an `active` workgroup that the owning `account_id` is a non-deleted member of. This is enforced at advertisement publish and update time. Once an advertisement is published, if the owning account loses membership of one of the scopes, the workgroup is silently dropped from the advertisement's effective visibility but the column is not edited; this avoids racy mutations during membership churn (the catalog query will filter it out anyway).

## Lifecycle / State Machine

States and transitions:

- `active` — published and visible to callers under workgroup-scope rules
- `retracted` — terminal; not visible in catalog queries; existing session references remain resolvable

Transitions:

- **publish** — `POST /v1/advertisements` creates the advertisement in `active` state with the supplied fields
- **update** — `PATCH /v1/advertisements/{advId}` mutates fields in place (name, description, capabilities, interaction patterns, workgroup scopes); status remains `active`. In-flight sessions that referenced the prior advertisement state are unaffected — sessions snapshot their relevant state at engagement time.
- **retract** — `DELETE /v1/advertisements/{advId}` transitions to `retracted`, sets `retracted_at`. Subsequent catalog queries do not return it.
- **reconnect re-association** — no explicit transition. Advertisements are tied to `account_id`, so any environment enrolled to that account can serve sessions for them. There is no per-environment claim or hand-off.

A retracted advertisement is not revivable; republishing the same `name` after retraction creates a new `adv_...` ID.

## Authorization

- **Publish**: any account may create an advertisement scoped to workgroups it is a member of. There is no `org-admin` or workgroup-admin gating in MVP.
- **Update / retract**: only the owning account (`account_id` on the advertisement) may mutate or retract.
- **Describe**: any account that is a member of at least one of the advertisement's `workgroup_scopes` may describe it. The owning account may always describe its own.
- **List-mine**: the `GET /v1/advertisements` endpoint returns only the caller's own advertisements; it does not return advertisements owned by others.
- **Catalog search**: any account may search; results are filtered to advertisements visible to them under the workgroup-scope rule.

Admin moderation surfaces (e.g. force-retract by platform admin) are out of MVP scope.

## Visibility Rules

The visibility rule is consistent across describe and catalog search: an advertisement is visible to an account if either (a) the account is the owner, or (b) the account holds a non-deleted membership in at least one workgroup listed in `workgroup_scopes`.

- The owner-or-member check happens at every read (describe, search). The catalog never returns advertisements outside this rule.
- Non-visible advertisements return `404 not_found` on direct describe attempts. The 404 deliberately mirrors the response for genuinely non-existent advertisement IDs; existence is not disclosed to non-entitled callers.
- A retracted advertisement is invisible to all callers except the owner (who sees it in their list-mine response with `status = retracted` for audit). Once retracted, a describe by ID returns `404` to non-owners.

Workgroup-scope intersection in catalog responses includes only the workgroups the caller can see; an advertisement scoped to `[wg_a, wg_b]` returned to a caller who is only a member of `wg_a` shows `workgroup_scopes: [wg_a]` in the response (the `wg_b` scope is redacted to avoid leaking workgroup membership of the owner across boundaries).

## Controller API Surface

Module: `internal/api/specs/advertisements/`. All endpoints use `accountTokenAuth`. Property names are camelCase; timestamps are RFC3339; the error response schema is the existing `error` from `common/schemas.yml`.

### Resource shape

**Advertisement** (response):

```
{
  "id": "adv_...",                           // pattern: ^adv_[a-z0-9]{12}$
  "accountId": "ac_...",                     // owning account
  "organizationId": "org_...",               // owning organization
  "name": "<string>",
  "description": "<string>",                 // optional; "" if unset
  "capabilities": [
    {
      "name": "<string>",
      "description": "<string>",             // optional
      "metadata": {"<key>": "<value>", ...}  // optional; opaque map
    },
    ...
  ],
  "interactionPatterns": [
    {"kind": "request-response"},
    {"kind": "stream"},
    {"kind": "custom", "customPattern": "<string>"},
    ...
  ],
  "workgroupScopes": ["wg_...", ...],
  "schemaVersion": 1,
  "status": "active" | "retracted",
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>",
  "retractedAt": "<RFC3339>"                 // null when status=active
}
```

### Endpoints

**`POST /v1/advertisements`** — publish a new advertisement.

- request body:
  ```
  {
    "name": "<string>",                       // required
    "description": "<string>",                // optional
    "capabilities": [{...}, ...],             // required; non-empty
    "interactionPatterns": [{...}, ...],      // required; non-empty
    "workgroupScopes": ["wg_...", ...]        // required; non-empty
  }
  ```
- response `201`: the created `Advertisement`
- errors:
  - `400 invalid_request` — missing required fields, empty arrays, capability without `name`, interaction pattern with `kind=custom` and missing `custom_pattern`, etc.
  - `400 unknown_workgroup` — a workgroup ID in `workgroupScopes` does not exist
  - `403 not_a_workgroup_member` — the caller is not a member of one or more of the listed workgroups
  - `409 name_in_use` — the caller already owns an active advertisement with the same name (case-insensitive)
  - `401`, `500`

**`GET /v1/advertisements`** — list the caller's own advertisements.

- query parameters:
  - `status` (optional, one of `active`, `retracted`; default returns both for owner)
- response `200`: array of `Advertisement`, ordered by `updated_at` descending

**`GET /v1/advertisements/{advId}`** — describe an advertisement (visibility-enforced).

- path: `advId` matches `^adv_[a-z0-9]{12}$`
- response `200`: `Advertisement` with `workgroupScopes` filtered per visibility rules
- errors: `401`; `404 not_found` when the advertisement does not exist or is not visible to the caller

**`PATCH /v1/advertisements/{advId}`** — update an existing advertisement.

- only the owner may call. Any subset of `name`, `description`, `capabilities`, `interactionPatterns`, `workgroupScopes` may be supplied; omitted fields are unchanged. Validation rules match `POST`.
- response `200`: the updated `Advertisement`
- errors:
  - `400` (same as POST)
  - `403 not_owner` when the caller is not the owning account
  - `404 not_found` when the advertisement does not exist or is not visible
  - `409 name_in_use` when the new `name` collides with another active advertisement of the same owner
  - `401`, `500`

**`DELETE /v1/advertisements/{advId}`** — retract an advertisement.

- only the owner may call.
- response `204` on success
- errors: `401`; `403 not_owner`; `404 not_found`; `500`. Retracting an already-retracted advertisement returns `204` (idempotent).

### Pagination

In MVP, list endpoints return the full result set without pagination, matching the existing Layer 1 list-endpoint convention. Cursor-based pagination is added later if owner-side advertisement counts warrant it.

## Agent / Runtime Surface

Advertisements are entirely controller-owned. The local agent runtime has no advertisement state, no advertisement-related gRPC methods, and no reconciliation logic. There is no per-environment claim handshake; once an account publishes an advertisement, every environment enrolled under that account is permitted to accept sessions for it. Coordination between concurrent advertising environments (if any) is the agent's own concern, not the controller's.

When an account loses access to a workgroup that one of its advertisements scopes to, the controller does not modify the advertisement; the catalog filter handles it. When the account is removed from all of an advertisement's workgroups, the advertisement effectively becomes invisible (only the owner sees it in list-mine) until the account regains membership in one of them.

## CLI Surface

Per [foundation.md](foundation.md): `agora advertise ...`. Output via `clioutput.NewTable()` / `RenderJSON()`.

`agora advertise publish <name>`

- flags:
  - `--description <string>` — optional
  - `--capability <name>[=<description>][:k1=v1,k2=v2]` — repeatable; CLI sugar that parses into the structured shape
  - `--interaction <kind>` — repeatable; `kind` is `request-response`, `stream`, `broadcast`, or `custom:<pattern>`
  - `--workgroup <name|wg_...>` — repeatable; required at least once
- positional args: advertisement name
- default output: confirmation with the new `adv_...` ID and a one-line summary
- `-j`, `--json` — emit the raw response

`agora advertise list`

- flags: `--status <active|retracted>` (optional), `-j` / `--json`
- default output: table with columns `name`, `id`, `status`, `workgroups`, `updated`

`agora advertise describe <name|adv_...>`

- flags: `-j` / `--json`
- positional args: advertisement identifier (name within caller's own ads, or full `adv_...`)
- default output: indented summary including capabilities, interaction patterns, visible workgroup scopes
- exits non-zero on `404`

`agora advertise update <name|adv_...>`

- flags: same shape as `publish`; only supplied flags trigger updates
- default output: confirmation with the updated fields

`agora advertise retract <name|adv_...>`

- flags: `-y` / `--yes` to skip interactive confirmation
- default output: one-line confirmation

The CLI resolves `<name>` to `adv_...` by listing the caller's own advertisements and matching on name.

## SDK Surface

No stable public Go SDK surface is committed in this slice. Consumers drive advertisements through:

- the CLI (`agora advertise ...`)
- the generated `ogen` REST client under `internal/api/`

This matches the workgroup slice's stance. An SDK helper layer can be extracted later once usage patterns stabilize.

## Failure Modes

Each row names a failure, its trigger, the expected HTTP status from the controller, and CLI behavior. All user-visible failures emit a structured `df/dl` log entry with `account_id`, `advertisement_id` (when known), and the failure reason.

- **Missing or invalid auth token** → `401 unauthorized`. CLI exits 1.
- **Missing required field on publish** (e.g. empty capabilities, missing name) → `400 invalid_request`.
- **Capability without `name`** → `400 invalid_request` with field detail.
- **Interaction pattern with `kind=custom` and missing `custom_pattern`** → `400 invalid_request`.
- **Unknown workgroup ID in `workgroupScopes`** → `400 unknown_workgroup`.
- **Caller not a member of one of the listed workgroups on publish/update** → `403 not_a_workgroup_member`.
- **Advertisement name collision** within owner → `409 name_in_use`. Retracted advertisements with the same name do not block reuse.
- **Advertisement not visible** (does not exist OR caller has no membership) → `404 not_found`. Same response for both cases by design.
- **Update or retract by non-owner** → `403 not_owner`.
- **Retracting an already-retracted advertisement** → `204 no content` (idempotent).
- **Updating a retracted advertisement** → `409 advertisement_retracted`.
- **Controller store error** → `500 internal`. Logs include the underlying error.

## Acceptance Criteria

The advertisements slice is implemented when all of the following are true. Each is verifiable by running CLI commands or inspecting controller state directly.

- **Publish.** An account that is a member of `wg_x` can `agora advertise publish my-ad --capability summarize=Summarizes text --interaction request-response --workgroup wg_x` and receive a `201` with a new `adv_...` ID.
- **Publish without workgroup membership rejected.** Same publish targeting a workgroup the caller is not a member of returns `403 not_a_workgroup_member`.
- **List-mine.** `agora advertise list` returns only the caller's own advertisements (active by default).
- **Describe-self.** Owner can describe their own advertisement and see all workgroup scopes.
- **Describe-cross-workgroup.** A non-owner account that shares a workgroup with the advertisement can describe it and receives only the intersected workgroup scopes in the response.
- **Describe-non-visible 404.** A non-owner account with no shared workgroup receives `404`; the response is indistinguishable from a request for a non-existent ID.
- **Update.** Owner can update name, description, capabilities, interaction patterns, and workgroup scopes; the response reflects the updates and `updated_at` advances.
- **Update by non-owner.** Returns `403 not_owner`.
- **Name uniqueness.** Two active advertisements owned by the same account cannot share a case-insensitive name; collision returns `409`.
- **Name reuse after retract.** After retracting an advertisement, a new advertisement with the same name can be published; the new advertisement gets a fresh `adv_...` ID.
- **Capabilities round-trip.** Capabilities with `metadata` survive publish→describe with byte-equal map content.
- **Interaction patterns round-trip.** All four enum values plus a `custom` pattern with `custom_pattern` survive publish→describe.
- **Retract.** Owner can retract; subsequent describe by non-owner returns `404`; subsequent describe by owner returns the record with `status=retracted` and a populated `retracted_at`.
- **Idempotent retract.** Re-issuing retract on an already-retracted advertisement returns `204`.
- **Catalog visibility integration.** The advertisement appears in catalog search results for accounts in shared workgroups; not for others.
- **Persistence schema.** A new persistence integration test covers name uniqueness, retraction state transitions, and workgroup-scope FK behavior.
- **Tests.** `go test ./...` passes including new persistence and controller HTTP integration tests.

## Out Of Scope For First Slice

- **`contract_requirements` field** — deferred to the contracts slice. The contracts slice's migration will add a `contract_id` column referencing the new contracts table; the publish/update API will be extended.
- **Admin moderation surfaces** (force-retract, audit-only views) — post-MVP if needed.
- **Advertisement content versioning** (separate from `schema_version`) — tracking historical content versions of a single advertisement is not in MVP.
- **Capability schema extensions** beyond `{name, description, metadata}` — richer per-capability typing is post-MVP.
- **Interaction pattern enum extensions** beyond the four MVP values — added on demand.
- **Cursor-based pagination of list-mine** — deferred.
- **Push subscriptions / change notifications** — out of MVP.

## Open Questions

None. All concept-level design questions are resolved. The advertisements slice is implementation-ready.
