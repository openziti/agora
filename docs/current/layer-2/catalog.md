# Layer 2: Catalog

Tier: A. Implementation-ready. See [foundation.md](foundation.md) for cross-cutting decisions.

## Purpose

The catalog is the discovery surface for Layer 2. It lets an agent find advertisements published by other participants without needing out-of-band coordination — the agent asks the catalog and gets back a filtered, visibility-enforced list of advertisements it is entitled to see. The catalog is intrinsic: it is the controller's built-in directory, not a separately operated service.

The catalog exists so agents can locate each other by capability rather than by pre-shared identity. Visibility enforcement happens at the catalog layer, not at the advertisement layer, so an agent that lacks workgroup access never sees the advertisement at all — it is not simply hidden behind a 403.

## Relationship To Other Layer 2 Concepts

- **Advertisements** are the data the catalog queries over. The catalog is not a separate persistence layer; it queries the advertisement store with visibility filters applied. See [advertisements.md](advertisements.md) for the underlying resource shape.
- **Workgroups** determine what is visible. Every catalog query is evaluated against the caller's workgroup memberships, intersected with each advertisement's `workgroup_scopes`.
- **Sessions** begin from a catalog result: an agent finds an advertisement, then proposes a session against it (in the sessions slice).
- **Contracts** will eventually surface through advertisement `contract_requirements`, letting consumers evaluate engagement feasibility before proposing a session. In the workgroups + catalog/advertisements slice, contract requirements are not yet present on advertisements; the contracts slice will add them.

## Data Model

The catalog has no resources of its own. Its data model is the advertisement model plus a runtime visibility filter and a search/filter API. Catalog responses return the same `Advertisement` resource shape defined in [advertisements.md](advertisements.md), with `workgroup_scopes` filtered to the intersection of the advertisement's scopes and the caller's memberships.

In the workgroups + catalog/advertisements slice, the catalog returns the full `Advertisement` shape rather than a separate projection. This keeps the API simple; if a smaller projection ever becomes necessary for efficiency, it can be added without breaking the existing surface.

## Lifecycle / State Machine

The catalog has no lifecycle of its own. Its surface is:

- **search** — `GET /v1/catalog/advertisements` returns a filtered, visibility-enforced list of advertisements
- **describe** — looking up a single advertisement by ID is served by the existing advertisement describe endpoint, `GET /v1/advertisements/{advId}`. The catalog provides a CLI alias (`agora catalog describe <adv_...>`) but does not introduce a separate HTTP endpoint; visibility enforcement is the same on both surfaces.

Visibility is enforced on every operation. An advertisement the caller cannot see returns the same response as an advertisement that does not exist — `404 not_found` from describe, absent from search results.

## Authorization

Any authenticated account may call the catalog. There is no admin-only catalog surface in MVP; admin moderation of advertisements (force-retract, etc.) is deferred. The catalog is read-only for all callers; advertisement mutations happen through `agora advertise ...`.

## Visibility Rules

The catalog applies the same visibility rule as the advertisement describe endpoint (defined in [advertisements.md](advertisements.md) "Visibility Rules"):

- An advertisement is visible to an account if either (a) the account is the owner, or (b) the account holds a non-deleted membership in at least one workgroup listed in the advertisement's `workgroup_scopes`.
- Retracted advertisements are not returned by the catalog regardless of visibility (the owner sees their retracted advertisements only through `agora advertise list`, not via the catalog).
- `workgroup_scopes` in catalog responses are filtered to the intersection of the advertisement's scopes and the caller's memberships, mirroring the advertisement describe behavior. This avoids leaking workgroup membership of the owner across boundaries.
- A specific describe by ID for a non-visible advertisement returns `404 not_found`; existence is not disclosed.

## Controller API Surface

Module: `internal/api/specs/catalog/`. Account-token authenticated. Property names are camelCase; timestamps RFC3339; the error response schema reuses `common/schemas.yml`'s `error`.

### Endpoints

**`GET /v1/catalog/advertisements`** — search.

Query parameters (all optional; combined with logical AND):

- `workgroup` — filter to advertisements scoped to this `wg_...`. Multiple values allowed via repeated query parameters; combined with logical OR within the parameter.
- `capability` — keyword substring match against `capabilities[].name` (case-insensitive). MVP supports a single substring; richer expressions are post-MVP.
- `interactionPattern` — filter by `kind` enum value (e.g. `request-response`); matches advertisements that include at least one matching interaction pattern. Multiple values allowed; OR within the parameter.
- `ownerOrganizationId` — filter to advertisements owned by accounts in this `org_...`.
- `cursor` — opaque pagination cursor (see Pagination below).
- `limit` — page size, default `50`, max `200`.

Response `200`:

```
{
  "items": [Advertisement, ...],            // visibility-filtered
  "nextCursor": "<opaque-string>"           // null/omitted when no further pages
}
```

The Advertisement shape is the same as in [advertisements.md](advertisements.md), with `workgroupScopes` filtered to the caller's visible intersection.

Errors:

- `400 invalid_request` — malformed cursor, unknown enum value, malformed `wg_...`/`org_...` reference
- `401 unauthorized` — missing or invalid account token
- `500 internal`

### Pagination

Cursor-based, opaque. The cursor encodes the position of the last returned advertisement in the stable sort order (`updated_at` descending, `created_at` as a tiebreaker, `id` as a final tiebreaker for total order). Callers treat the cursor as opaque; servers may evolve the encoding without breaking compatibility as long as a server-issued cursor is always interpretable by the same controller version that issued it.

A response with no `nextCursor` field (or with `nextCursor: null`) signals the end of the result set.

### Sort ordering

Default order: `updated_at` descending, then `created_at` descending, then `id` ascending as a final tiebreaker. The MVP catalog does not allow callers to override the sort order. Custom orderings can be added in a future slice without breaking compatibility (defaulting to the current order when unspecified).

### Read consistency

The catalog is strongly consistent with the underlying advertisements store. There is no caching layer in MVP. Every catalog query reads from PostgreSQL; an advertisement published or retracted in transaction T is visible (or invisible) to every catalog query starting after T commits.

## Agent / Runtime Surface

The catalog is controller-only. The local agent runtime has no catalog state, no catalog gRPC methods, and no caching. Consumers run catalog queries from CLI or future SDK helpers via the controller HTTP API.

## CLI Surface

Per [foundation.md](foundation.md): `agora catalog ...`.

`agora catalog search`

- flags:
  - `--workgroup <name|wg_...>` — filter by workgroup; repeatable. Names resolve via `agora workgroup list`.
  - `--capability <substring>` — keyword filter on capability names
  - `--interaction <kind>` — filter by interaction pattern; repeatable
  - `--owner-org <org_...>` — filter by owning organization
  - `--limit <int>` — page size, default `50`
  - `--cursor <opaque>` — continue from a previous response's `nextCursor`
  - `-j`, `--json` — emit raw response
- positional args: none
- default output: table with columns `name`, `id`, `owner`, `workgroups`, `capabilities`, `updated`. The `capabilities` column shows up to three capability names with an ellipsis if there are more.
- prints the `nextCursor` (if any) at the bottom of the table for caller convenience

`agora catalog describe <name|adv_...>`

- flags:
  - `-j`, `--json`
- positional args: advertisement identifier (full `adv_...` ID, or a name resolved via `agora catalog search`)
- behavior: CLI sugar that hits `GET /v1/advertisements/{advId}`. There is no separate `/v1/catalog/advertisements/{advId}` endpoint.
- exits non-zero on `404`

Name-to-ID resolution for `describe`: if the supplied token starts with `adv_`, it is used directly; otherwise the CLI runs `catalog search --capability <token>` (treating the token as a possible capability keyword) and looks for an exact match on advertisement name. If multiple matches exist, the CLI lists candidates and exits non-zero so the caller can disambiguate by ID.

## SDK Surface

No stable public Go SDK surface is committed in this slice. Consumers drive the catalog through the CLI or the generated `ogen` REST client under `internal/api/`. An SDK helper layer can be extracted later once consumer patterns stabilize.

## Failure Modes

- **Missing or invalid auth token** → `401 unauthorized`. CLI exits 1.
- **Malformed cursor** → `400 invalid_request` with a clear "cursor is no longer valid; restart the search" message. Treated as a developer mistake; the CLI does not attempt to recover.
- **Unknown `interactionPattern` enum value** → `400 invalid_request`.
- **Unknown `wg_...` or `org_...` reference in filters** → `400 invalid_request` with a "no such workgroup/organization" message. (Catalog deliberately distinguishes "ID does not exist" from "ID exists but you can't see results scoped to it" — the former is a developer error; the latter is silently empty results.)
- **Filter references a workgroup the caller is not a member of** → returns empty results (no `400`, no `403`). The caller is allowed to ask; the visibility filter just produces zero matches.
- **No matching advertisements** → `200` with `items: []` and no `nextCursor`. Not an error.
- **Describe of non-visible advertisement** → `404 not_found` from the underlying `GET /v1/advertisements/{advId}`. Same response as a non-existent ID.
- **Controller store error** → `500 internal`. Logs include the underlying error.

## Acceptance Criteria

The catalog slice is implemented when all of the following are true. Each is verifiable by running CLI commands or inspecting controller state directly.

- **Empty search.** `agora catalog search` against a controller with no advertisements returns `items: []` with no `nextCursor`.
- **Search returns visible advertisements.** A caller with membership in `wg_x` and an active advertisement scoped to `wg_x` (owned by another account) sees that advertisement in `agora catalog search`.
- **Search excludes non-visible advertisements.** The same caller does not see an advertisement scoped only to `wg_y` (where they have no membership).
- **Search excludes retracted advertisements.** A retracted advertisement does not appear in any caller's catalog results, including the owner's.
- **Workgroup-scope filter.** `--workgroup wg_x` returns only advertisements that include `wg_x` in their scopes (and that the caller can see).
- **Capability filter.** `--capability summarize` returns only advertisements with at least one capability whose name contains "summarize" (case-insensitive substring) — and that the caller can see.
- **Interaction-pattern filter.** `--interaction request-response` returns only advertisements that declare a matching interaction pattern.
- **Owning-organization filter.** `--owner-org org_x` returns only advertisements whose owning account belongs to `org_x` — and that the caller can see.
- **Combined filters AND together.** Multiple filter parameters narrow results consistently.
- **Pagination.** With `--limit 2` and three or more visible advertisements, the first response carries `items.length == 2` and a `nextCursor`; the second response (with that cursor) returns the remaining advertisements and no `nextCursor`.
- **Stable order.** Repeated `agora catalog search` against an unchanging dataset returns results in the same order across calls.
- **Cursor invalidation.** A cursor issued before a controller upgrade or a sort-key change may return `400 invalid_request`. The MVP does not commit to long-term cursor stability beyond the controller's process lifetime.
- **Describe-by-id round-trip.** `agora catalog describe <adv_...>` returns the same advertisement as `agora advertise describe <adv_...>` (they hit the same endpoint).
- **Describe of non-visible.** A caller with no shared workgroup with the advertisement receives `404`; the response is indistinguishable from a non-existent ID.
- **Strong consistency.** Publishing an advertisement, then immediately calling `agora catalog search`, returns the new advertisement (subject to visibility).
- **Tests.** `go test ./...` passes including catalog HTTP integration tests for visibility, filter, and pagination behavior.

## Out Of Scope For First Slice

- **Semantic search** (vector/embedding-based matching) — deferred per [../../future/roadmap/post-mvp.md](../../future/roadmap/post-mvp.md).
- **Full-text search** across capability descriptions — MVP supports substring on `name` only.
- **Catalog subscriptions / push notifications** on new advertisements — post-MVP.
- **Cross-controller federated catalog** — out of MVP.
- **Catalog-level caching** on client or agent — out of MVP.
- **Custom sort orders** — MVP fixes the order; future slices may add a `--sort` parameter.
- **Admin-side catalog moderation surfaces** — deferred.

## Open Questions

None. All concept-level design questions are resolved. The catalog slice is implementation-ready.
