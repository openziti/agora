# Layer 2: Catalog

Tier: B (skeleton). See [foundation.md](foundation.md) for cross-cutting decisions.

## Purpose

The catalog is the discovery surface for Layer 2. It lets an agent find advertisements published by other participants without needing out-of-band coordination — the agent asks the catalog and gets back a filtered, visibility-enforced list of advertisements it is entitled to see. The catalog is intrinsic: it is the controller's built-in directory, not a separately operated service.

The catalog exists so agents can locate each other by capability rather than by pre-shared identity. Visibility enforcement happens at the catalog layer, not at the advertisement layer, so an agent that lacks workgroup access never sees the advertisement at all — it is not simply hidden behind a 403.

## Relationship To Other Layer 2 Concepts

- **Advertisements** are the data the catalog queries over. The catalog is not a separate persistence layer; it queries the advertisement store with visibility filters applied.
- **Workgroups** determine what is visible. Every catalog query is evaluated against the caller's workgroup memberships.
- **Sessions** begin from a catalog result: an agent finds an advertisement, then proposes a session against it.
- **Contracts** are surfaced through the catalog as part of an advertisement's contract requirements, so consumers can evaluate engagement feasibility before proposing.

## Data Model

The catalog has no resources of its own. Its data model is the advertisement model plus a runtime filter layer. Catalog results are projections of advertisements with the caller's visibility already applied.

- **Catalog result entry** (wire-level shape, not persisted)
  - advertisement ID
  - name, description, capabilities summary
  - owning account and organization
  - visible workgroup scopes (intersection of the ad's scopes and the caller's memberships)
  - contract requirements summary
  - `schema_version`

Full advertisement detail is retrieved via a `describe` operation on the advertisement itself; the catalog surface returns a list-style projection.

## Lifecycle / State Machine

The catalog has no lifecycle of its own. Its surface is:

- `search` — filtered query by workgroup, capability keywords, interaction patterns, owning organization
- `describe` — deterministic lookup of a single advertisement by ID, subject to visibility

Visibility is enforced on every operation. An advertisement the caller cannot see returns the same response as an advertisement that does not exist.

## Authorization

Placeholder (Tier A).

## Visibility Rules

Placeholder (Tier A). Reference: [foundation.md](foundation.md) "Authorization Composition."

## Controller API Surface

Placeholder (Tier A). Module: `internal/api/specs/catalog/`.

## Agent / Runtime Surface

Placeholder (Tier A). Catalog is controller-only in MVP; no local agent involvement expected.

## CLI Surface

Placeholder (Tier A). Per [foundation.md](foundation.md): `agora catalog search`, `agora catalog describe`.

## SDK Surface

Placeholder (Tier A).

## Failure Modes

Placeholder (Tier A).

## Acceptance Criteria

Placeholder (Tier A).

## Out Of Scope For First Slice

- semantic catalog search (vector/embedding-based matching) — deferred per [../roadmap/post-mvp.md](../roadmap/post-mvp.md)
- catalog subscription / push-based notifications on new advertisements
- cross-controller federated catalog
- catalog-level caching on the client or agent side

## Open Questions

- **Filter expression language.** What does a catalog search filter look like — simple field-equals + keyword match on capabilities, or something richer? Proposed MVP: structured filters on owning-organization, workgroup, interaction-pattern, plus a capability-keyword substring match. Full-text or semantic search is explicitly out of MVP scope.
- **Pagination model.** Cursor-based or offset-based? Proposed: cursor-based with stable ordering, matching the pattern most scalable REST APIs use. Needs confirmation.
- **Sort ordering and stability.** What ordering does the catalog return by default, and what can the caller request? Proposed default: most-recently-updated first, with `created_at` as a stable tiebreaker.
- **"Not visible" vs "does not exist."** When a caller asks `describe adv_xxx` and the advertisement exists but is not visible, does the controller return 404 or 403? Proposed: 404 (avoid leaking existence), matching the spec's "should not appear in catalog results at all" language.
- **Read consistency.** Is the catalog strongly consistent with advertisement writes, or eventually consistent? Proposed: strongly consistent with the underlying PostgreSQL-backed advertisements repository; no caching in MVP.
- **Who can update an advertisement in the catalog.** Tracked under advertisements.md but surfaces here: does the catalog surface any admin-side operations (e.g. moderation), or is it read-only for all callers? Proposed: read-only; advertisement mutations happen through `agora advertise ...`.
