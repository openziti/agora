# Layer 2 (Collaboration) Status

This document records the current implementation state of Layer 2 (Collaboration), the documentation depth of each concept, and the recommended order for implementation work.

## Current Position

Layer 2 is currently **documented but not implemented**.

Documentation has moved beyond the initial architectural sketch. The cross-cutting design decisions are captured in [foundation.md](foundation.md), and each of the six concepts has its own dedicated spec. The next milestone is per-concept Tier A promotion and implementation in build order.

The repository does not yet have:

- workgroup persistence and API surface
- catalog APIs or controller logic
- advertisement lifecycle
- session lifecycle
- contract evaluation
- envelope handling infrastructure
- Layer 2 CLI commands
- Layer 2 SDK surfaces

## Documentation Tier Tracking

Per-concept specs use two tiers:

- **Tier A** — implementation-ready. Every template section populated; "Open Questions" section is empty (or populated only with explicitly deferred post-MVP items). Ready for engineers to cut code against.
- **Tier B** — skeleton. Purpose, relationships, data model sketch, lifecycle sketch, and "Open Questions" populated. Other sections named as placeholders.

Current state:

| Concept | File | Tier |
| --- | --- | --- |
| Foundation (cross-cutting) | [foundation.md](foundation.md) | A |
| Workgroups | [workgroups.md](workgroups.md) | A |
| Catalog | [catalog.md](catalog.md) | B |
| Advertisements | [advertisements.md](advertisements.md) | B |
| Sessions | [sessions.md](sessions.md) | B |
| Contracts | [contracts.md](contracts.md) | B |
| Envelopes | [envelopes.md](envelopes.md) | B |

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
- a settled runtime packaging direction (thin daemon; see [../layer-1/agent.md](../layer-1/agent.md) "Packaging Direction")

This means the next Layer 2 work can focus on collaboration semantics rather than first building transport and identity.

## Recommended Build Order

The cleanest order for initial Layer 2 work is:

1. workgroup model and membership semantics
2. catalog and advertisement persistence plus visibility rules
3. session model and engagement lifecycle
4. declarative contract model and controller-side evaluation
5. envelope model and session-governed message semantics

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

Those are tracked separately in [../roadmap/post-mvp.md](../roadmap/post-mvp.md).
