# Layer 2: Contracts

Tier: B (skeleton). See [foundation.md](foundation.md) for cross-cutting decisions.

## Purpose

A contract defines the negotiated terms under which a session operates. It bounds the session on dimensions the platform can observe — duration, envelope count, allowed message types, workgroup and trust preconditions, and the access mode that gates acceptance. Contracts exist so providers can publish engagement terms declaratively and so the controller can evaluate consumer requests against those terms without the provider needing to be online to decide.

The contract is the governance contract between provider and consumer. It is declarative in MVP: a fixed set of fields the controller understands and evaluates. Programmatic contracts (arbitrary code evaluated per engagement) are explicitly deferred.

## Relationship To Other Layer 2 Concepts

- **Advertisements** reference a contract as their contract requirements — the terms a consumer must satisfy to engage.
- **Sessions** carry a frozen snapshot of the contract at engagement time. The snapshot is the authority for session enforcement; later changes to the source contract do not affect in-flight sessions.
- **Workgroups** may be listed as required memberships in a contract (above and beyond the workgroup scoping of the advertisement itself).
- **Envelopes** are subject to contract enforcement: disallowed message types are rejected, envelope-count bounds are tracked, duration bounds trigger session close.

## Data Model

- **Contract** (MVP shape, `schema_version = 1`)
  - `id` (`con_...`)
  - `schema_version` (integer, MVP `1`)
  - `max_duration` (session-level cap; may be absent for no cap)
  - `max_envelope_count` (session-level cap; may be absent)
  - `allowed_message_types` (list; empty list means no restriction — see Open Questions)
  - `required_workgroup_memberships` (list of `wg_...`; consumer must hold all)
  - `maturity_requirements` (see Open Questions)
  - `access_mode` (`open`, `approval_required`, `policy_evaluated`)
  - `created_at`, `updated_at`

Whether a contract is a standalone persisted resource or embedded inside an advertisement is an Open Question.

## Lifecycle / State Machine

Contracts themselves are largely static. Their lifecycle is:

- created (by a provider-side account, typically as part of publishing or updating an advertisement)
- referenced by one or more advertisements
- snapshotted into sessions at engagement time (the session carries its own frozen copy)

Contracts have no "active/closed" states; they are either extant or deleted. Deleting a contract that is still referenced by an active advertisement is disallowed.

## Authorization

Placeholder (Tier A).

## Visibility Rules

Placeholder (Tier A). A contract is visible to anyone who can see an advertisement that references it (via the advertisement's visibility); standalone contract inspection is an Open Question.

## Controller API Surface

Placeholder (Tier A). Module: `internal/api/specs/contracts/`.

## Agent / Runtime Surface

Placeholder (Tier A). Contract evaluation is controller-side in MVP. Agent-side enforcement is an Open Question (see below).

## CLI Surface

Placeholder (Tier A). Per [foundation.md](foundation.md), contracts do not get a standalone top-level command in MVP; inspection nests under `agora session describe` and `agora advertise describe`.

## SDK Surface

Placeholder (Tier A).

## Failure Modes

Placeholder (Tier A).

## Acceptance Criteria

Placeholder (Tier A).

## Out Of Scope For First Slice

- programmatic contracts (arbitrary code or DSL evaluated per engagement) — deferred per [../roadmap/post-mvp.md](../roadmap/post-mvp.md)
- mid-session contract renegotiation
- contract versioning beyond `schema_version` (i.e. tracking historical content versions)
- cross-organization shared contract templates

## Open Questions

- **Standalone resource vs embedded.** Is a contract a first-class persisted resource with its own ID and API, or is it always embedded inline in an advertisement? Proposed MVP: standalone resource with `con_...` IDs, so one contract template can be referenced by multiple advertisements. Needs confirmation.
- **Maturity / trust requirements shape.** "Maturity or trust requirements" is listed in the spec but undefined. Candidate dimensions: account age, prior-session success count, organization trust flag, external attestation reference. Proposed MVP: account age (minimum duration since account creation) only; richer trust model deferred.
- **Access mode semantics.**
  - `open`: automatic acceptance when other contract terms are satisfied.
  - `approval_required`: provider must explicitly accept each proposed session.
  - `policy_evaluated`: controller evaluates a declarative policy against the proposal and decides.
  - Open: exact wire-level representation of `policy_evaluated` in MVP, if any. Proposed: treat `policy_evaluated` as post-MVP and allow only `open` and `approval_required` in MVP.
- **Violation handling.** What happens when a contract is violated mid-session? Options: silent envelope drop, error envelope to the violator, session close with `close_reason = contract_violation`, logged-only. Proposed MVP: session close with `close_reason = contract_violation`; controller emits a structured log entry; per-envelope rejection is future work.
- **`allowed_message_types` semantics.** Is an empty list "no restriction" or "no types allowed"? Proposed MVP: empty list means no restriction, matching the most forgiving default. Needs confirmation.
- **Where evaluation happens.** Controller-side only in MVP, or also agent-side (provider local enforcement)? Proposed MVP: controller-side only for admission and duration/count bounds; no agent-side enforcement in MVP.
- **Snapshot vs live contract behavior.** On session engagement, does the session bind a frozen snapshot of the contract, or does the session re-evaluate against the live contract? Proposed: snapshot at engagement — later contract updates do not affect in-flight sessions. Needs confirmation and clear documentation to avoid surprising consumers.
- **Standalone inspection.** Can a consumer fetch a contract by ID separately from the advertisement that references it? Proposed: yes, subject to the same workgroup-based visibility as the referencing advertisement(s).
