# Layer 2: Workgroups

Tier: A. Implementation-ready. See [foundation.md](foundation.md) for cross-cutting decisions.

## Purpose

A workgroup is the policy boundary that controls visibility and interaction scope in Layer 2. Membership in a workgroup is the precondition for seeing advertisements, engaging in sessions, and operating under contracts scoped to that workgroup. Workgroups let operators separate concerns (e.g. internal analytics agents vs. customer-facing agents) and let organizations collaborate across org boundaries without flattening their tenancy model.

Workgroups are **not** an access-control list bolted onto other resources; they are the primary scoping axis for every Layer 2 concept. If a catalog query, advertisement, session, or contract is not associated with a workgroup visible to the caller, it does not exist for them.

## Relationship To Other Layer 2 Concepts

- **Catalog** filters every query by the caller's workgroup memberships.
- **Advertisements** declare which workgroups they are visible in.
- **Sessions** are established within a specific workgroup shared by the provider and consumer.
- **Contracts** may express required workgroup memberships as a precondition for session engagement.
- **Envelopes** carry the session's workgroup identity in their headers for audit and routing purposes.

Workgroups are the foundation that the rest of Layer 2 references. Changes to workgroup membership take effect immediately for subsequent queries and engagements, and — for MVP — also for in-flight sessions involving the removed account (see Lifecycle).

## Data Model

- **Workgroup**
  - `id` (`wg_...`)
  - `name` (case-insensitive, unique within `owner_organization_id`)
  - `description` (optional)
  - `scope` (`intra-org` or `inter-org`)
  - `state` (`pending`, `active`, `declined`)
  - `owner_organization_id` (always set; for `inter-org`, this is the creating org)
  - `participating_organization_ids` (list; only populated for `inter-org`; excludes the owner — these are the invited orgs)
  - `created_at`, `updated_at`

- **Workgroup Invitation** (only for `inter-org`)
  - `id` (`wgi_...`)
  - `workgroup_id`
  - `organization_id` (one record per invited org)
  - `state` (`pending`, `accepted`, `declined`)
  - `acknowledged_by_account_id` (set when `accepted` or `declined`)
  - `acknowledged_at`

- **Workgroup Membership**
  - `id` (`wgm_...`)
  - `workgroup_id`
  - `account_id`
  - `role` (`member`, `admin`)
  - `joined_at`

One membership record per `(workgroup_id, account_id)` pair. Admin role implies member role. Every active workgroup must have at least one admin membership at all times.

The workgroup's aggregate `state` is a function of its invitations for inter-org scope:

- any invitation in `declined` → workgroup is `declined` (terminal)
- all invitations in `accepted` → workgroup is `active`
- otherwise → workgroup is `pending`

For intra-org workgroups, there are no invitations and the workgroup is created directly in `active`.

## Lifecycle / State Machine

### Workgroup

Intra-org workgroups are created by the platform admin via the admin surface (see [foundation.md](foundation.md) "Authorization Composition"). The create request specifies the workgroup's name, description, owner organization, and an `initial_admin_account_id` — an account in the owner organization that receives the initial workgroup admin membership. The workgroup is created directly in `active` state.

Inter-org workgroups go through a per-org handshake:

- the platform admin creates the workgroup on behalf of the owner organization, with `participating_organization_ids = [B, C, ...]` and an `initial_admin_account_id` in the owner org
- the workgroup enters `pending`; one `WorkgroupInvitation` is created per invited org, each in `pending`
- the owner org's `initial_admin_account_id` receives the initial workgroup admin membership; the owner org does not have a `WorkgroupInvitation` (it is the owner, not an invitee)
- each invited org independently accepts or declines. Acknowledgement is performed via the platform admin surface; the admin request specifies which invited organization is acknowledging and (on accept) an `initial_admin_account_id` in that invited org who becomes that org's initial workgroup admin membership. The acknowledging account is recorded on the `WorkgroupInvitation` for audit
  - on acceptance by all invited orgs → the workgroup transitions to `active`
  - on decline by any invited org → the workgroup transitions to `declined` (terminal). Any still-pending invitations on that workgroup are effectively moot; they remain `pending` on the terminal record for audit
- a `pending` workgroup is visible to members and workgroup admins of the owner org, and to admin-surface queries scoped to any of the invited orgs (so operators can see invitations awaiting their acknowledgement). It is not visible to regular accounts in invited orgs until their org's invitation is accepted
- a `declined` workgroup is retained as a terminal audit record and is not revivable; re-engagement requires a fresh workgroup with a fresh set of invitations

A `declined` workgroup has no memberships beyond the creating admin's seeded record and is not referenceable as a scope by advertisements, sessions, or contracts.

An active workgroup is deleted by a workgroup admin. Deletion cascades as follows:

- advertisements scoped only to this workgroup are retracted
- advertisements scoped to this and other workgroups drop this workgroup from their scopes
- active sessions under this workgroup are closed with `close_reason = workgroup_deleted`
- contracts that required membership in this workgroup become unsatisfiable for new sessions; existing sessions already using those contracts close as above
- membership records for the workgroup are removed; invitation records (if any) are retained for audit

### Workgroup membership

Once a workgroup has been seeded with an initial admin, day-to-day membership operations do not require the platform admin token:

- added by a workgroup admin in the account's own organization, under the normal account-token surface
- role changed by a workgroup admin in the account's own organization (a member may be promoted to admin, and an admin may be demoted to member, provided at least one admin membership remains on the workgroup as a whole)
- removed by a workgroup admin in the account's own organization, or self-removed by the member, with the same at-least-one-admin invariant

When a membership is removed while the affected account has active sessions in the workgroup, those sessions are closed immediately with `close_reason = membership_removed`. A configurable close policy (e.g. allow in-flight sessions to finish, or a grace period) is post-MVP work.

In inter-org workgroups, each participating organization independently manages which of its own accounts are members. No cross-org admin authority exists: a workgroup admin whose account is in org A cannot add, remove, or change the role of an account in org B, even within a workgroup both orgs share.

## Authorization

The authorization model uses:

- the existing **platform admin token** (`X-ADMIN-TOKEN`) for organization-level workgroup bootstrap operations — creating a workgroup, acknowledging an inter-org invitation
- a per-workgroup **workgroup admin** role, expressed as a `WorkgroupMembership` with `role = admin`, for day-to-day workgroup administration under the normal account-token surface

See [foundation.md](foundation.md) "Authorization Composition" for the broader authorization model. No new account-level role is introduced by workgroups.

Authorization rules:

- **Creating a workgroup** (intra-org or inter-org): caller must present the platform admin token. The request body must include an `initialAdminAccountId` in the owner organization; that account receives the initial workgroup admin membership.
- **Acknowledging an inter-org invitation** (accept or decline): caller must present the platform admin token. The request is scoped to a specific `{wg_id, organization_id}` pair identifying the invitation being acknowledged. On accept, the request body must include an `initialAdminAccountId` in the acknowledging organization; that account receives an initial workgroup admin membership for that org. On decline, no initial admin is seeded. The acknowledging request records the admin account performing the call on the `WorkgroupInvitation` for audit.
- **Adding / removing / changing role of a membership**: caller must hold workgroup admin role on the workgroup, and the target account must be in the same organization as the caller. Uses the normal account-token surface.
- **Self-removing a membership**: permitted for any member under the normal account-token surface, subject to the at-least-one-admin-per-workgroup invariant.
- **Deleting a workgroup**: caller must hold workgroup admin role on the workgroup. Deletion affects the entire workgroup regardless of organization. Uses the normal account-token surface.
- **Listing / describing a workgroup**: permitted for any account that is a member, under the normal account-token surface. Pending-invitation visibility into not-yet-accepted workgroups is exposed through the admin surface only.

The at-least-one-admin invariant is enforced at the controller on every role-change, membership-remove, and self-remove operation.

## Visibility Rules

Visibility composes with the state machine from Lifecycle. Two surfaces apply: the account-token surface (normal accounts asking about workgroups they participate in) and the admin-token surface (platform admin with cross-org visibility for bootstrap and audit).

**Account-token surface:**

- an `active` workgroup is visible to any account holding a `WorkgroupMembership` on it, regardless of organization
- a `pending` workgroup is visible to account-token callers in the **owner** organization (members and workgroup admins only); it is not visible to account-token callers in invited organizations until their org accepts
- a `declined` workgroup is not visible through the account-token surface to any account

**Admin-token surface:**

- an `active` workgroup is visible with no filter
- a `pending` workgroup is visible with no filter; `GET /v1/admin/workgroups` supports filtering by `state` and by invitation `organization_id` so operators can page through the invitations awaiting acknowledgement in a specific org
- a `declined` workgroup is retained and remains visible through the admin-token surface for audit; list endpoints include it only when `state=declined` is requested explicitly

**Listing behavior:**

- `GET /v1/workgroups` returns only the workgroups visible to the caller under the account-token surface rules above
- `GET /v1/admin/workgroups` returns all workgroups subject to the admin-surface rules and optional filters

**Non-visibility return code:**

Requests for a specific workgroup (`GET /v1/workgroups/{wg_id}`, `DELETE /v1/workgroups/{wg_id}`, member operations) return `404 Not Found` — not `403 Forbidden` — when the workgroup exists but is not visible to the caller. This is a deliberate choice: a `403` would disclose that the workgroup exists to callers who are not entitled to know. Attempts by callers authenticated and authorized to see the workgroup but lacking the required role (e.g. a non-admin member trying to delete) return `403`. The same distinction applies to member endpoints (`404` for not-visible workgroups, `403` for insufficient role).

Membership records (`WorkgroupMembership`) are visible through `GET /v1/workgroups/{wg_id}/members` to any account that can see the workgroup itself. Membership records disclose the workgroup-relative roles of other accounts but do not disclose account emails outside the caller's organization — cross-org memberships surface `accountId` and `organizationId` only, not `email`.

Invitation records (`WorkgroupInvitation`) are visible only through the admin-token surface. Account-token describe operations do not include invitation detail in the response body.

## Controller API Surface

The surface splits between the account-token module (`internal/api/specs/workgroups/`) for day-to-day operations and the admin-token module (extending `internal/api/specs/admin/`) for platform bootstrap operations. All paths are under the existing `/v1` prefix. Request and response bodies are JSON with `application/json` Content-Type. Property names are camelCase, consistent with the existing Layer 1 surfaces (`internal/api/specs/tunnels/`, `internal/api/specs/environments/`). Timestamps are RFC3339 strings (`format: date-time` in OpenAPI).

The error response schema reuses the existing `error` schema at `internal/api/specs/common/schemas.yml` (`{ code, message }`).

### Resource shapes

**Workgroup** (response shape):

```
{
  "id": "wg_...",                                       // pattern: ^wg_[a-z0-9]{12}$
  "name": "<string>",
  "description": "<string>",                            // optional; "" if unset
  "scope": "intra-org" | "inter-org",
  "state": "pending" | "active" | "declined",
  "ownerOrganizationId": "org_...",                     // pattern: ^org_[a-z0-9]{12}$
  "participatingOrganizationIds": ["org_...", ...],     // empty for intra-org
  "createdAt": "<RFC3339>",
  "updatedAt": "<RFC3339>"
}
```

**WorkgroupInvitation** (response shape; admin surface only):

```
{
  "id": "wgi_...",                                      // pattern: ^wgi_[a-z0-9]{12}$
  "workgroupId": "wg_...",
  "organizationId": "org_...",
  "state": "pending" | "accepted" | "declined",
  "acknowledgedByAccountId": "ac_...",                  // null when state=pending
  "acknowledgedAt": "<RFC3339>"                         // null when state=pending
}
```

**WorkgroupMembership** (response shape):

```
{
  "id": "wgm_...",                                      // pattern: ^wgm_[a-z0-9]{12}$
  "workgroupId": "wg_...",
  "accountId": "ac_...",                                // pattern: ^ac_[a-z0-9]{12}$
  "organizationId": "org_...",                          // the account's org; included for cross-org visibility
  "role": "member" | "admin",
  "joinedAt": "<RFC3339>"
}
```

### Account-token endpoints

All account-token endpoints use `accountTokenAuth`. Missing/invalid token returns `401`.

**`GET /v1/workgroups`** — list workgroups the caller is a member of.

- query parameters:
  - `state` (optional, one of `active` only in MVP account-token surface — `pending` applies only to the owner org's members and is returned alongside `active` when no filter is given)
- response `200`: JSON array of `Workgroup` objects, ordered by `updatedAt` descending
- errors: `401`

**`GET /v1/workgroups/{wgId}`** — describe a workgroup.

- path: `wgId` matches `^wg_[a-z0-9]{12}$`
- response `200`: `Workgroup` object
- errors: `401`; `404` when the workgroup is not visible to the caller

**`DELETE /v1/workgroups/{wgId}`** — delete a workgroup.

- path: `wgId` matches `^wg_[a-z0-9]{12}$`
- response `204` on success
- errors: `401`; `403` when the caller is a member but not a workgroup admin; `404` when the workgroup is not visible
- side effects as enumerated in Lifecycle (cascade to advertisements, sessions, contracts, memberships)

**`GET /v1/workgroups/{wgId}/members`** — list memberships.

- path: `wgId` matches `^wg_[a-z0-9]{12}$`
- response `200`: JSON array of `WorkgroupMembership` objects, ordered by `joinedAt` ascending
- errors: `401`; `404` when the workgroup is not visible

**`POST /v1/workgroups/{wgId}/members`** — add a member.

- path: `wgId` matches `^wg_[a-z0-9]{12}$`
- request body:
  ```
  {
    "accountEmail": "<string>",       // required; resolved to an account in the caller's org
    "role": "member" | "admin"        // optional; defaults to "member"
  }
  ```
- response `201`: the created `WorkgroupMembership`
- errors: `400` on malformed body or unknown email in caller's org; `401`; `403` when caller is not a workgroup admin, or when target account's org ≠ caller's org; `404` when the workgroup is not visible; `409` when the account is already a member

**`PATCH /v1/workgroups/{wgId}/members/{wgmId}`** — change a membership's role.

- path: `wgId`, `wgmId` patterns as above
- request body:
  ```
  {
    "role": "member" | "admin"        // required
  }
  ```
- response `200`: the updated `WorkgroupMembership`
- errors: `400` on malformed body; `401`; `403` when caller is not a workgroup admin, or when target membership's org ≠ caller's org; `404` when workgroup or membership is not visible; `409` when demoting would leave zero admins on the workgroup

**`DELETE /v1/workgroups/{wgId}/members/{wgmId}`** — remove a membership.

- path: `wgId`, `wgmId` patterns as above
- response `204` on success
- errors: `401`; `403` when caller is neither a workgroup admin nor the membership's owning account; `404` when workgroup or membership is not visible; `409` when removal would leave zero admins on the workgroup
- side effect: the account's active sessions in this workgroup are closed with `close_reason = membership_removed` (see `sessions.md`)

### Admin-token endpoints

All admin-token endpoints use `adminTokenAuth`. Missing/invalid token returns `401`.

**`POST /v1/admin/workgroups`** — create a workgroup.

- request body:
  ```
  {
    "name": "<string>",                               // required
    "description": "<string>",                        // optional
    "scope": "intra-org" | "inter-org",               // required
    "ownerOrganizationId": "org_...",                 // required
    "initialAdminAccountId": "ac_...",                // required; must be in owner org
    "participatingOrganizationIds": ["org_...", ...]  // required for inter-org; must be non-empty and must not include ownerOrganizationId
  }
  ```
- response `201`:
  ```
  {
    "workgroup": Workgroup,
    "invitations": [WorkgroupInvitation, ...]     // empty for intra-org
  }
  ```
- errors: `400` on malformed body, missing required fields, `initialAdminAccountId` not in owner org, `participatingOrganizationIds` containing owner, unknown org/account IDs; `401`; `409` on name collision within owner org

**`GET /v1/admin/workgroups`** — list workgroups across all organizations.

- query parameters:
  - `state` (optional, one of `pending`, `active`, `declined`; omit for all non-declined states)
  - `ownerOrganizationId` (optional, `org_...`)
  - `invitedOrganizationId` (optional, `org_...` — filters to workgroups with an invitation to this org)
- response `200`:
  ```
  [
    {
      "workgroup": Workgroup,
      "invitations": [WorkgroupInvitation, ...]
    },
    ...
  ]
  ```
  ordered by `workgroup.updatedAt` descending
- errors: `401`; `400` on malformed query parameters

**`POST /v1/admin/workgroups/{wgId}/invitations/{organizationId}/accept`** — acknowledge an invitation with `accepted`.

- path: `wgId` matches `^wg_[a-z0-9]{12}$`; `organizationId` matches `^org_[a-z0-9]{12}$`
- request body:
  ```
  {
    "initialAdminAccountId": "ac_...",    // required; must be in {organizationId}
    "acknowledgingAdminAccountId": "ac_..." // optional override; see below
  }
  ```
  The `acknowledgingAdminAccountId` field is optional. When omitted, the controller records the account derived from administrative context (deployment-specific; implementations without an admin-account concept record the call under a sentinel value such as `"admin"`). When present, the supplied account must exist and belong to `organizationId`; it is recorded on the invitation for audit.
- response `200`:
  ```
  {
    "invitation": WorkgroupInvitation,
    "workgroup": Workgroup       // reflects the aggregate state, possibly transitioned to "active"
  }
  ```
- errors: `400` on malformed body, unknown `initialAdminAccountId`, or `initialAdminAccountId` not in `{organizationId}`; `401`; `404` when no invitation exists for this `{wgId, organizationId}` pair; `409` when the invitation is already `accepted` or `declined`, or when the containing workgroup is already `declined`

**`POST /v1/admin/workgroups/{wgId}/invitations/{organizationId}/decline`** — acknowledge an invitation with `declined`.

- path as above
- request body:
  ```
  {
    "acknowledgingAdminAccountId": "ac_..."   // optional; same semantics as accept
  }
  ```
- response `200`:
  ```
  {
    "invitation": WorkgroupInvitation,
    "workgroup": Workgroup       // reflects the aggregate state, now "declined"
  }
  ```
- errors: `400`; `401`; `404` when no invitation exists for this pair; `409` when the invitation is already non-`pending`

### Pagination

List endpoints in MVP return the full result set without pagination. This is consistent with the existing Layer 1 list endpoints (`GET /v1/tunnels`, `GET /v1/environments`), which also return full arrays. Cursor-based pagination is a post-MVP addition once workgroup counts warrant it.

## Agent / Runtime Surface

Workgroups are fully controller-owned. The local `agora network` runtime has no workgroup state, no workgroup-related gRPC methods, and no reconciliation logic. The local environment root under `~/.agora` stores no workgroup information.

This remains true even though workgroup membership changes can affect active sessions: the session-close side effect (`close_reason = membership_removed` / `workgroup_deleted`) is driven by the controller through the existing session-close path that already propagates to the agent via the Layer 1 tunnel teardown. No new agent-side surface is introduced by workgroups.

## CLI Surface

All CLI output uses the existing Layer 1 conventions: human-readable tables via `go-pretty/table` (same rounded-table style as `cmd/agora/tunnelList.go`), and machine-readable JSON via `--json` emitting the raw resource objects exactly as returned by the controller (consistent with `foundation.md`'s observability conventions).

Admin token for the `admin` subcommands is taken from the `AGORA_ADMIN_TOKEN` environment variable, consistent with existing admin commands (`cmd/agora/adminCreateOrganization.go`, `cmd/agora/adminCreateUser.go`).

### Account-token commands

`agora workgroup list`

- flags:
  - `-j`, `--json` — machine-readable output
- positional args: none
- default output: table with columns `name`, `id`, `scope`, `state`, `owner_org`, `updated_at`
- JSON output: array of raw `Workgroup` objects

`agora workgroup describe <name|wg_...>`

- flags:
  - `-j`, `--json`
- positional args: workgroup identifier (name within caller's accessible workgroups, or full `wg_...` ID)
- default output: indented summary of the `Workgroup` plus a member count
- JSON output: raw `Workgroup` object
- exits 1 on `404`

`agora workgroup delete <name|wg_...>`

- flags:
  - `-y`, `--yes` — skip interactive confirmation
- positional args: workgroup identifier
- default output: one-line confirmation of deletion
- no JSON output variant
- exits 1 on `403`, `404`

`agora workgroup member list <name|wg_...>`

- flags:
  - `-j`, `--json`
- positional args: workgroup identifier
- default output: table with columns `email`, `account_id`, `organization_id`, `role`, `joined_at`; `email` column is blank for accounts outside the caller's org
- JSON output: array of raw `WorkgroupMembership` objects

`agora workgroup member add <name|wg_...> <account-email>`

- flags:
  - `--role <member|admin>` — default `member`
- positional args: workgroup identifier, target account email
- default output: one-line confirmation with the new `wgm_...` ID and role
- no JSON output variant
- exits 1 on `400`, `403`, `404`, `409`

`agora workgroup member remove <name|wg_...> <account-email>`

- flags:
  - `-y`, `--yes` — skip interactive confirmation
- positional args: workgroup identifier, target account email
- default output: one-line confirmation
- exits 1 on `403`, `404`, `409`

`agora workgroup member role <name|wg_...> <account-email> <member|admin>`

- flags: none
- positional args: workgroup identifier, target account email, new role
- default output: one-line confirmation of the role change
- exits 1 on `400`, `403`, `404`, `409`

### Admin-token commands

`agora admin workgroup create <name>`

- flags:
  - `--owner-org <org_...>` — required
  - `--initial-admin <account-email>` — required; resolved to `ac_...` in owner org
  - `--description <string>` — optional
  - `--scope <intra-org|inter-org>` — default `intra-org`
  - `--invite <org_...>` — repeatable; required when `--scope=inter-org`; rejected otherwise
- positional args: workgroup name
- default output: human-readable summary of the created workgroup plus any invitations
- `-j`, `--json` — machine-readable response body

`agora admin workgroup list`

- flags:
  - `--state <pending|active|declined>` — optional
  - `--owner-org <org_...>` — optional
  - `--invited-org <org_...>` — optional
  - `-j`, `--json`
- positional args: none
- default output: table with columns `name`, `id`, `scope`, `state`, `owner_org`, `invitations`, `updated_at`; `invitations` shows a compact `pending/accepted/declined` count for inter-org workgroups
- JSON output: raw response body

`agora admin workgroup accept <name|wg_...>`

- flags:
  - `--org <org_...>` — required; the invited org whose invitation is being accepted
  - `--initial-admin <account-email>` — required; resolved to `ac_...` in `--org`
- positional args: workgroup identifier
- default output: one-line confirmation plus the workgroup's new aggregate state

`agora admin workgroup decline <name|wg_...>`

- flags:
  - `--org <org_...>` — required
  - `-y`, `--yes` — skip interactive confirmation
- positional args: workgroup identifier
- default output: one-line confirmation

The CLI resolves workgroup names to `wg_...` IDs by calling the appropriate list endpoint; when ambiguity would arise (e.g. two workgroups with the same name visible to the caller across different orgs for the admin surface), the CLI reports the ambiguity and asks for the full ID.

## SDK Surface

No stable public Go SDK surface is committed in the first workgroup slice. Consumers drive workgroups through:

- the CLI (`agora workgroup ...`, `agora admin workgroup ...`)
- the generated `ogen` REST client under `internal/api/` (the same mechanism used by the existing Layer 1 code), invoked directly where embedding-level integration is needed

Once consumer usage patterns stabilize — particularly after sessions and advertisements ship — an explicit SDK layer may be extracted from the internal packages. Treating that as a deliberate later step avoids committing to an API surface before the Layer 2 consumer ergonomics are known.

This is a scope decision, not an Open Question. No `org-admin` role, no separate SDK package, no preemptive client types beyond what `ogen` generates.

## Failure Modes

Each row below names a failure condition, its trigger, the expected HTTP status from the controller, and the expected CLI exit behavior. All user-visible failures also emit a structured `df/dl` log entry with relevant identifiers (`workgroup_id`, `account_id`, `organization_id`, or the admin endpoint's `organization_id` path component).

- **Missing or invalid auth token.** No `X-TOKEN` on account-token endpoints or no `X-ADMIN-TOKEN` on admin endpoints. `401`. CLI exits 1 with `authentication required` message.
- **Wrong token surface.** An account token on an admin-only endpoint (or vice versa). `401`. Logged with the attempted endpoint.
- **Missing required field in create request.** E.g. `initialAdminAccountId` omitted, `participatingOrganizationIds` empty for `scope=inter-org`, `participatingOrganizationIds` non-empty for `scope=intra-org`. `400` with `error.code = invalid_request`.
- **Unknown organization or account ID in create request.** `ownerOrganizationId`, `initialAdminAccountId`, or any `participatingOrganizationIds` entry does not resolve. `400` with `error.code = unknown_reference`.
- **`initialAdminAccountId` not in the declared org.** Create or accept request specifies an account whose `organization_id` ≠ the relevant org. `400` with `error.code = cross_org_initial_admin`.
- **`participatingOrganizationIds` includes the owner.** `400` with `error.code = owner_in_participants`.
- **Name collision.** Create request name (case-insensitive) collides with an existing `active` or `pending` workgroup in the owner org. `409` with `error.code = name_in_use`. `declined` workgroups retain their names but do not block reuse.
- **Workgroup not visible to caller.** Any direct reference to a `wg_...` the caller cannot see under the applicable surface rules. `404` with `error.code = not_found`. The caller cannot distinguish "does not exist" from "not visible" — by design.
- **Membership not visible to caller.** Any direct reference to a `wgm_...` whose workgroup the caller cannot see, or a `wgm_...` that does not exist. `404` with `error.code = not_found`.
- **Insufficient role.** Caller is a member of the workgroup but not a workgroup admin, attempting an operation that requires admin (delete, add member, remove other's membership, change role). `403` with `error.code = insufficient_role`.
- **Cross-org membership operation.** Workgroup admin in org A attempts to add, remove, or re-role an account in org B within an inter-org workgroup. `403` with `error.code = cross_org_membership`.
- **Duplicate membership.** Add member targets an account already holding a membership on the workgroup. `409` with `error.code = already_a_member`.
- **At-least-one-admin invariant.** Demotion, removal, or self-removal would leave the workgroup with zero admins. `409` with `error.code = last_admin`. The CLI surface prints a hint to promote another member to admin first.
- **Invitation not found.** Accept or decline against a `{wgId, organizationId}` where no `WorkgroupInvitation` exists. `404` with `error.code = not_found`.
- **Invitation already acknowledged.** Accept or decline against an invitation whose `state ≠ pending`. `409` with `error.code = already_acknowledged`.
- **Workgroup in terminal state.** Any mutating operation against a workgroup whose aggregate state is `declined`. `409` with `error.code = workgroup_declined`.
- **Controller store error.** Any underlying database failure surfaces as `500` with `error.code = internal`. The structured log includes the underlying error.
- **CLI resolution ambiguity.** The CLI cannot resolve a name to a single `wg_...` or `ac_...` (e.g. an admin listing workgroups sees two same-named workgroups across different orgs). CLI prints the candidates and exits 1; this is a CLI-only failure and does not reach the controller.

## Acceptance Criteria

The workgroup slice is implemented when all of the following are true. Each is verifiable by running CLI commands or inspecting controller state directly.

- **Intra-org creation.** `agora admin workgroup create wg-intra --owner-org org_... --initial-admin alice@example.com` creates a workgroup in `active` state with one `WorkgroupMembership` for Alice with `role = admin`. `GET /v1/admin/workgroups/{wg_id}` (via `agora admin workgroup list` filtered by ID) returns it with zero invitations.
- **Inter-org creation.** `agora admin workgroup create wg-inter --owner-org org_A --initial-admin alice@example.com --scope inter-org --invite org_B --invite org_C` creates the workgroup in `pending` with `WorkgroupInvitation` records in `pending` for orgs B and C. Alice receives the initial admin membership.
- **Inter-org acceptance.** `agora admin workgroup accept wg-inter --org org_B --initial-admin bob@example.com` transitions `org_B`'s invitation to `accepted`, records Bob as the acknowledging account's seeded workgroup admin, and leaves the workgroup in `pending` while `org_C` remains unacknowledged. A second accept for `org_C` transitions the workgroup to `active`.
- **Inter-org decline.** `agora admin workgroup decline wg-inter --org org_C` (against a workgroup with at least one pending invitation) transitions the workgroup to `declined`. A subsequent accept on any other invitation returns `409 workgroup_declined`.
- **Active workgroup describe.** An account that is a member of an `active` workgroup, authenticated with its account token, sees the workgroup in `GET /v1/workgroups` and via `agora workgroup describe`.
- **Non-member describe returns 404.** An account that is not a member attempts `GET /v1/workgroups/{wg_id}` and receives `404`, regardless of whether the workgroup exists.
- **Member add same-org.** A workgroup admin in org A runs `agora workgroup member add <wg> carol@org-a.example` (Carol in org A). Membership is created; `GET /v1/workgroups/{wg_id}/members` returns Carol's record.
- **Member add cross-org is blocked.** The same admin attempts `agora workgroup member add <wg> dan@org-b.example` (Dan in org B). Controller returns `403 cross_org_membership`; CLI exits 1.
- **Role change admin-to-member.** A workgroup admin demotes another admin in the same org via `agora workgroup member role <wg> <email> member`. Membership updates; `GET /v1/workgroups/{wg_id}/members` reflects the new role.
- **At-least-one-admin invariant on demotion.** Demoting the last admin of a workgroup returns `409 last_admin`.
- **At-least-one-admin invariant on removal.** Removing the last admin (or self-removing as the last admin) returns `409 last_admin`.
- **Self-remove as non-last-admin.** A member self-removes via `agora workgroup member remove <wg> <self>`. Membership deleted.
- **Delete by workgroup admin.** A workgroup admin runs `agora workgroup delete <wg>`. Workgroup is deleted; `GET /v1/workgroups/{wg_id}` subsequently returns `404` for the former members.
- **Delete cascade (with other slices).** When the sessions and advertisements slices are in place, deleting a workgroup retracts advertisements scoped only to it, closes active sessions with `close_reason = workgroup_deleted`, and drops the workgroup from advertisements that scoped to it plus others. This criterion is deferred until those slices ship; workgroup-slice implementation must emit the necessary events so later slices can satisfy it.
- **Membership removal closes sessions (with sessions slice).** When the sessions slice is in place, removing a membership closes that account's active sessions in the workgroup with `close_reason = membership_removed`. Deferred like the cascade criterion above.
- **404-not-403 leak prevention.** Attempting `GET /v1/workgroups/{wg_id}` or any member-endpoint call with a valid account token but no membership on the target workgroup returns `404`, not `403`. Inspected by reviewing controller response codes under the test matrix.
- **Admin-token list filters.** `agora admin workgroup list --state pending` returns only pending workgroups. `--invited-org org_B` returns only those with a `WorkgroupInvitation` for `org_B`.
- **Observability.** Every creation, accept, decline, member add/remove/role-change, and delete emits a `df/dl` log line with at least `workgroup_id`, the actor identifier, and the outcome. Verified by `grep`-ing the controller log during a scripted smoke run.
- **Tests.** `go test ./...` passes, including a new persistence integration test that exercises the at-least-one-admin invariant and the pending→accepted/declined transitions, and a new controller HTTP integration test that exercises every endpoint's authorization paths.

## Out Of Scope For First Slice

- workgroup hierarchy (deferred per [the roadmap](../../roadmap.md))
- per-workgroup rate limits or quotas (limits are deferred)
- workgroup-level audit surfaces beyond what falls out of standard logging
- configurable membership-removal close policy (MVP hardcodes "close immediately")
- invitation cancellation by the owner (an owner-side `DELETE` on a `pending` invitation) — post-MVP if needed
- account-level `org-admin` role for self-service workgroup creation — deferred until self-service demand emerges
- cursor-based pagination of list endpoints — deferred until list sizes warrant it
- a dedicated public Go SDK surface — deferred until consumer patterns stabilize
- invitation state beyond the three defined (`pending`, `accepted`, `declined`) — e.g. `expired`, `revoked` — out of MVP

## Open Questions

None. All concept-level design questions are resolved. The workgroup slice is implementation-ready.
