# Layer 2: Advertisements

Tier: B (skeleton). See [foundation.md](foundation.md) for cross-cutting decisions.

## Purpose

An advertisement is an agent's declaration of what it can do and how to engage with it. Advertisements let a participant publish capabilities to the catalog so other participants in shared workgroups can find and engage them without prior out-of-band coordination. The advertisement is the durable record of intent; the catalog is the query surface over the set of advertisements.

Advertisements are persistent and tied to the advertising account, not to a specific environment or session. If the advertising participant disconnects and reconnects, its existing advertisements remain associated with its identity — they are not re-created on every reconnect.

## Relationship To Other Layer 2 Concepts

- **Workgroups** determine where an advertisement is visible. An advertisement declares one or more workgroup scopes.
- **Catalog** queries return advertisement projections filtered by the caller's workgroup memberships.
- **Contracts** are referenced from advertisements as contract requirements — the terms the advertiser requires for any session engagement.
- **Sessions** are proposed against an advertisement. The advertisement's contract requirements are evaluated when a session is proposed.

## Data Model

- **Advertisement**
  - `id` (`adv_...`)
  - `account_id`, `organization_id` (owning identity)
  - `name`
  - `description`
  - `capabilities` (see Open Questions)
  - `interaction_patterns` (see Open Questions)
  - `workgroup_scopes` (list of `wg_...` IDs)
  - `contract_requirements` (reference to a contract, or inline contract)
  - `schema_version` (MVP: `1`)
  - `status` (`active`, `retracted`)
  - `created_at`, `updated_at`, `retracted_at`

Advertisements are soft-deleted (status `retracted`) rather than hard-deleted, so in-flight references from sessions or audit logs remain resolvable.

## Lifecycle / State Machine

- `active` — published and visible to callers with matching workgroup membership
- `retracted` — no longer visible in catalog queries; existing session references remain valid

Transitions:

- publish: create as `active`
- update: mutate in place (name, description, capabilities, scopes, contract requirements); remains `active`
- retract: transition to `retracted`; not visible in catalog going forward
- re-associate on reconnect: an advertising participant reconnecting via a new environment enrollment picks up its existing `active` advertisements automatically, without republishing (see Open Questions for exact mechanism)

## Authorization

Placeholder (Tier A). The owning account manages the advertisement. Admin moderation is an Open Question.

## Visibility Rules

Placeholder (Tier A). Visibility is governed by `workgroup_scopes` intersected with the caller's memberships; see [foundation.md](foundation.md).

## Controller API Surface

Placeholder (Tier A). Module: `internal/api/specs/advertisements/`.

## Agent / Runtime Surface

Placeholder (Tier A). Advertisements are controller-owned. The local agent does not host advertisement state in MVP.

## CLI Surface

Placeholder (Tier A). Per [foundation.md](foundation.md): `agora advertise publish|update|retract|list|describe`.

## SDK Surface

Placeholder (Tier A).

## Failure Modes

Placeholder (Tier A).

## Acceptance Criteria

Placeholder (Tier A).

## Out Of Scope For First Slice

- advertisement content versioning (separate from `schema_version` — i.e. tracking historical content versions of a single advertisement)
- admin-side moderation surfaces
- richer envelope extensions exposed through advertisements — deferred per [../roadmap/post-mvp.md](../roadmap/post-mvp.md)
- deeper A2A metadata bridging beyond carrying A2A payloads as opaque — deferred

## Open Questions

- **Capabilities shape.** Is `capabilities` a list of structured records (e.g. `{name, version, description}`), a list of freeform strings, or A2A-style structured metadata? Proposed MVP: list of structured `{name, description}` entries, with A2A-aligned metadata carried as opaque key-value fields. Needs confirmation.
- **Interaction patterns shape.** Enum (`request-response`, `stream`, `broadcast`) or freeform? Proposed: enum for MVP, with a `custom` escape hatch that carries a freeform string. Exact enum values open.
- **One-advertisement-per-workgroup or multi-workgroup.** Can a single advertisement target multiple workgroups, or must a participant publish one per workgroup? Proposed: single advertisement can list multiple `workgroup_scopes`; visibility is the union.
- **Reconnect re-association mechanism.** How exactly does "existing advertisements remain associated with the advertising identity on reconnect" work? Options: (a) advertisements are tied to `account_id` alone, so any environment enrolled to that account can act as the advertising runtime; (b) advertisements carry an `environment_id` reference that must be updated on reconnect. Proposed: option (a), for simplicity. Needs confirmation.
- **Name uniqueness scope.** Per-account? Per-organization? Per-workgroup? Globally? Proposed: per-owning-account, case-insensitive.
- **Contract requirements: embedded or referenced.** Is the advertisement's contract requirements an inline contract structure, a reference to a standalone contract resource, or either/or? Cross-referenced with `contracts.md`.
- **Update semantics.** Does updating an advertisement invalidate in-flight sessions that were established under the prior state? Proposed: no — in-flight sessions carry a snapshot of the contract at engagement time. Needs confirmation.
