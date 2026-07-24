# Layer 2 (Collaboration) Status

This document records the current implementation state of Layer 2 (Collaboration), the documentation depth of each concept, and the recommended order for implementation work.

## Current Position

Layer 2 is **MVP-complete**: all five slices (workgroups, catalog+advertisements, sessions, contracts, envelopes) have shipped. Every per-concept spec is at Tier A with its open-questions walk closed and implementation landed.

Documentation has moved beyond the initial architectural sketch. The cross-cutting design decisions are captured in [foundation.md](foundation.md), and each of the six concepts has its own dedicated spec. The next milestones are post-MVP extensions tracked in [the roadmap](../../roadmap.md): session multiplexing over a shared tunnel, programmatic contracts, envelope-level signing, richer A2A bridging, metrics, limits, and the other items collected there.

What ships today:

- workgroup persistence (3 tables), API (11 endpoints across account-token and admin-token surfaces), CLI (`agora workgroup ...` and `agora admin workgroup ...`)
- advertisement + catalog persistence, API, and CLI (`agora advertise ...`, `agora catalog ...`)
- session persistence (1 table), API (6 account-token + 1 admin-token endpoints), CLI (`agora session ...`, `agora admin session close`)
- `sdk/agent/session` helper package (Propose, RegisterHandler, Session.Close, Session.ContractSnapshot) that drives session lifecycle through the controller
- contract persistence (1 table) + advertisements.contract_id retrofit, API (5 endpoints), CLI extensions (`agora advertise ... --contract`), session-duration reaper that enforces `max_duration_seconds` via scheduled close with `close_reason=contract_violation`
- admission-time contract evaluation at session accept (`required_workgroup_memberships`, `maturity_requirements.min_account_age_days`) with frozen snapshot written to `sessions.contract_snapshot`
- envelope wire format (`frame_version` + length-prefixed JSON header + opaque payload), runtime-level tunnel attach on both consumer and provider sides, envelope `Send`/`Receive` on `*session.Session`, contract enforcement of `max_envelope_bytes` / `max_envelope_count` / `allowed_message_types` at the SDK layer, platform hard ceiling of 10 MiB per envelope, periodic envelope-count heartbeat via `POST /v1/sessions/{id}/envelope-count`, CLI `agora session send` for one-shot debugging
- the Macro Pulse demo runs an end-to-end ping: each provider/tool agent attaches an `EchoSessionHandler` that reads the inbound envelope and writes a `.response` reply; `pulse-agent` iterates the catalog and exchanges a `{domain}.{capability}.request` → `{domain}.{capability}.response` envelope round-trip per advertisement

The post-MVP surface tracked in [the roadmap](../../roadmap.md) covers everything not shipped in MVP.

## Documentation Tier Tracking

Per-concept specs use two tiers:

- **Tier A** — implementation-ready. Every template section populated; "Open Questions" section is empty (or populated only with explicitly deferred post-MVP items). Ready for engineers to cut code against.
- **Tier B** — skeleton. Purpose, relationships, data model sketch, lifecycle sketch, and "Open Questions" populated. Other sections named as placeholders.

Current state:

| Concept | File | Tier | Implementation |
| --- | --- | --- | --- |
| Foundation (cross-cutting) | [foundation.md](foundation.md) | A | n/a |
| Workgroups | [workgroups.md](workgroups.md) | A | shipped (slice 1) |
| Catalog | [catalog.md](catalog.md) | A | shipped (slice 2) |
| Advertisements | [advertisements.md](advertisements.md) | A | shipped (slice 2) |
| Sessions | [sessions.md](sessions.md) | A | shipped (slice 3) |
| Contracts | [contracts.md](contracts.md) | A | shipped (slice 4) |
| Envelopes | [envelopes.md](envelopes.md) | A | shipped (slice 5) |

Concepts are promoted to Tier A one slice at a time, just before implementation of that slice begins. Premature Tier A promotion of later concepts is avoided so that decisions made during earlier slices can flow forward without documentation thrash.

## What Exists Already That Layer 2 Can Build On

Layer 2 is not starting from zero. It already has the Layer 1 substrate it depends on:

- multi-tenant organizations and accounts
- controller-managed environment lifecycle
- real tunnel provisioning and connectivity
- local runtime ownership for long-lived network behavior
- controller-visible liveness, serve, and attachment state
- PostgreSQL-backed controller state
- OpenAPI-driven API workflow
- a settled runtime packaging direction (dual delivery: standalone daemon + embeddable library; see [../layer-1/agent.md](../layer-1/agent.md) "Packaging Direction")

This means the next Layer 2 work can focus on collaboration semantics rather than first building transport and identity.

## Recommended Build Order

The cleanest order for initial Layer 2 work is:

1. ~~workgroup model and membership semantics~~ — **shipped**
2. ~~catalog and advertisement persistence plus visibility rules~~ — **shipped**
3. ~~session model and engagement lifecycle~~ — **shipped**
4. ~~declarative contract model and controller-side evaluation~~ — **shipped**
5. ~~envelope model and session-governed message semantics (also wires runtime-level tunnel attach and byte-level session I/O)~~ — **shipped**

Layer 2 MVP is complete.

Each step promotes its concept spec from Tier B to Tier A before implementation starts. The ordering keeps visibility and policy boundaries in place before sessions and contracts depend on them. The session↔tunnel 1:1 relationship (fixed in [foundation.md](foundation.md)) lets sessions, contracts, and envelopes all reference a known transport model.

## Out Of Scope For The First Layer 2 Slice

These items should not shape the first Layer 2 implementation slice:

- metrics
- limits
- semantic catalog search
- programmatic contracts
- memory-oriented products
- deep protocol bridge behavior
- session multiplexing over a shared tunnel
- workgroup hierarchy

Those are tracked separately in [the roadmap](../../roadmap.md).
