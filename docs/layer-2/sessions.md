# Layer 2: Sessions

Tier: B (skeleton). See [foundation.md](foundation.md) for cross-cutting decisions, especially the **session-to-tunnel 1:1 relationship** that this spec depends on.

## Purpose

A session is the governed communication channel between two collaborating participants. Sessions provide lifecycle, attribution, and contract enforcement on top of Layer 1 connectivity. Every message exchanged between Layer 2 participants flows inside a session; no Layer 2 communication happens outside one.

A session is distinct from the Layer 1 tunnel that carries it. The session is the governance object — provider, consumer, workgroup, advertisement, contract, state — while the tunnel is the Layer 1 transport. Per [foundation.md](foundation.md), MVP sessions are 1:1 with a backing Layer 1 tunnel.

## Relationship To Other Layer 2 Concepts

- **Advertisements** are the target of a session. A consumer proposes a session against a specific advertisement, and the advertisement's contract requirements govern acceptance.
- **Workgroups** scope a session. Both provider and consumer must share the workgroup the advertisement is visible in.
- **Contracts** are bound to a session at engagement time. The contract's terms are in force for the session's lifetime.
- **Envelopes** are the messages exchanged within a session. Envelopes flow through the session's backing Layer 1 tunnel.
- **Layer 1 tunnels** carry session traffic. Session creation provisions a tunnel; session closure deletes it.

## Data Model

- **Session**
  - `id` (`ses_...`)
  - `provider_account_id`, `provider_organization_id`
  - `consumer_account_id`, `consumer_organization_id`
  - `workgroup_id`
  - `advertisement_id`
  - `contract_snapshot` (frozen copy of the contract at engagement time — see `contracts.md`)
  - `tunnel_id` (the Layer 1 tunnel backing this session)
  - `state` (see lifecycle below)
  - `close_reason` (populated on close)
  - `proposed_at`, `accepted_at`, `closed_at`

One session record per established channel. The `tunnel_id` references a Layer 1 tunnel resource owned by the provider account and scoped to the workgroup.

## Lifecycle / State Machine

States and transitions:

- `proposed` — consumer has proposed a session; provider has not yet responded
- `accepting` — provider has begun the engagement flow (e.g. tunnel provisioning, contract evaluation)
- `active` — engagement completed; tunnel is up; envelopes may flow
- `closing` — close has been requested; tunnel is being drained and deprovisioned
- `closed` — terminal; tunnel deleted; no further traffic

Transitions:

- consumer proposes → `proposed`
- provider accepts → `accepting` → (on success) `active`
- provider rejects during `proposed` or `accepting` → `closed` with `close_reason = rejected`
- either side closes while `active` → `closing` → `closed`
- controller closes for contract violation → `closing` → `closed` with `close_reason = contract_violation`
- controller closes because the tunnel failed → `closed` with `close_reason = tunnel_failed`

Rejection and close reasons are enumerated in `close_reason` for auditability.

## Authorization

Placeholder (Tier A). Both provider and consumer must be members of the advertisement's workgroup; contract requirements further constrain acceptance.

## Visibility Rules

Placeholder (Tier A). A session is visible to its provider, its consumer, and admin surfaces of their respective organizations.

## Controller API Surface

Placeholder (Tier A). Module: `internal/api/specs/sessions/`.

## Agent / Runtime Surface

Placeholder (Tier A). Session lifecycle involves the local runtime because the backing Layer 1 tunnel's serve/connect is agent-hosted. Coordination pattern expected to mirror the existing `agora tunnel serve`/`agora tunnel connect` delegation through the gRPC+UDS surface.

## CLI Surface

Placeholder (Tier A). Per [foundation.md](foundation.md): `agora session propose|accept|close|list|describe|send`.

## SDK Surface

Placeholder (Tier A).

## Failure Modes

Placeholder (Tier A).

## Acceptance Criteria

Placeholder (Tier A).

## Out Of Scope For First Slice

- session multiplexing over a shared tunnel — deferred (see [foundation.md](foundation.md))
- mid-session contract renegotiation
- session handoff between environments
- semantic or policy-driven auto-engagement

## Open Questions

- **Engagement flow mechanics.** Is the propose/accept exchange synchronous (consumer waits for acceptance in one REST call) or asynchronous (consumer proposes, polls for provider response)? Proposed MVP: asynchronous — consumer proposes, session starts in `proposed` state, provider accepts or rejects in a separate call; consumer polls or subscribes to state changes. Needs confirmation.
- **Tunnel mode negotiation.** A session is 1:1 with a Layer 1 tunnel, which has a mode (`http`, `tcp`, `udp`). Who picks the mode — the advertisement declares it, the consumer requests it, or it's negotiated? Proposed: the advertisement declares the mode; the consumer accepts that mode or does not propose. `tcp` is the expected default for envelope transport.
- **Unilateral close semantics.** When one side closes an `active` session, does the other side learn via envelope stream end, an explicit control envelope, or a separate REST notification? Proposed: envelope stream end (tunnel teardown) is authoritative; controller state reflects the close within the reaping window.
- **Controller-initiated close.** What triggers controller-initiated closure? Contract violations (clear), workgroup deletion (clear), environment disable (clear). Other triggers TBD.
- **Session resumption after tunnel failure.** If the backing tunnel fails (e.g. network partition), does the session attempt to reconnect, or immediately close with `tunnel_failed`? Proposed MVP: immediate close; future slice may add resumption.
- **Concurrent sessions limit.** Are there platform- or account-level caps on concurrent active sessions? Limits are deferred to post-MVP per [../roadmap/post-mvp.md](../roadmap/post-mvp.md), so MVP: no cap, tracked for observability only.
- **Provider self-session.** Can an account have a session with itself (e.g. across its own environments)? Proposed: yes, same rules apply; useful for local testing.
