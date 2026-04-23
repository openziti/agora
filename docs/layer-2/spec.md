# Layer 2 (Collaboration) Specification

This document is the architectural overview for Layer 2 (Collaboration) in Agora. It names the Layer 2 concepts and their relationships, and delegates implementation-level detail to the per-concept specs.

Layer 2 is built entirely on top of Layer 1 (Network). It adds discovery, engagement, and governance semantics for agent-to-agent collaboration without introducing a second transport substrate.

## Where To Read Next

- [foundation.md](foundation.md): cross-cutting Layer 2 design decisions (ID scheme, OpenAPI module organization, authorization composition, code layout, CLI shape, session↔tunnel relationship, local state posture, versioning, observability, MVP boundary). Every per-concept spec builds on these decisions.
- [workgroups.md](workgroups.md): workgroup model, membership, intra-org and inter-org scopes.
- [catalog.md](catalog.md): discovery surface over advertisements, with visibility enforcement.
- [advertisements.md](advertisements.md): agent capability declarations.
- [sessions.md](sessions.md): governed communication channels backed 1:1 by Layer 1 tunnels.
- [contracts.md](contracts.md): declarative engagement terms.
- [envelopes.md](envelopes.md): structured session message format.
- [status.md](status.md): current implementation state and per-concept Tier tracking.

## Purpose

Layer 2 provides:

- workgroup-scoped visibility and interaction boundaries
- intrinsic discovery through a catalog
- capability declarations through advertisements
- governed communication through sessions and contracts
- structured communication through envelopes

At the infrastructure level, Layer 2 composes Layer 1 primitives rather than replacing them.

## Core Concepts

The six Layer 2 concepts are:

- **Workgroup**: policy boundary controlling visibility and interaction scope. Intra-organization and shared inter-organization workgroups. Flat in the initial design.
- **Catalog**: intrinsic controller service for discovery. Directory lookup and filtered search. Visibility enforced at the discovery layer — entries not entitled to be seen do not appear in results at all.
- **Advertisement**: an agent's persistent declaration of name, description, capabilities, supported interaction patterns, visibility scope, and contract requirements. Re-associated with the advertising identity on reconnect.
- **Session**: a governed communication channel between collaborating participants, backed by Layer 1 connectivity. Explicit lifecycle, closable by either side or by policy.
- **Contract**: declarative negotiated terms for a session — duration, envelope count, allowed message types, required workgroup memberships, trust requirements, access mode.
- **Envelope**: structured message format with infrastructure-visible headers and opaque payloads, enabling governance without requiring the controller to understand every payload format.

Each concept has a dedicated spec under this directory.

## Relationship To Layer 1

Layer 2 depends on these Layer 1 capabilities:

- enrolled identities and environment lifecycle
- secure connectivity through tunnels
- org-scoped authorization
- local runtime and controller liveness model

Layer 2 does not define a new network substrate. It adds governance and discovery semantics on top of Layer 1 connectivity. The specific session↔tunnel relationship in MVP is 1:1 and is documented in [foundation.md](foundation.md).

## A2A Compatibility

Agora's A2A compatibility is structural rather than controller-bridged by default:

- advertisements may carry A2A-aligned metadata
- envelopes may carry A2A payloads as opaque message bodies
- external interoperability should happen through explicit entrypoints rather than weakening zero-trust assumptions

Deeper A2A bridge behavior is deferred to post-MVP work.

## Gateway Composition

`llm-gateway` and `mcp-gateway` are natural infrastructure participants in the Agora model. They are primarily Layer 1 consumers, but they also provide natural integration points for behavioral monitoring, content inspection, and policy enforcement at application boundaries.

## Deferred Extensions

The broader architecture includes several deferred collaboration capabilities, including semantic catalog search, programmatic contracts, richer envelope extensions, deeper A2A bridge behavior, and memory-oriented collaboration services built on top of the base primitives. Those items are tracked in [../roadmap/post-mvp.md](../roadmap/post-mvp.md) and called out in per-concept specs under "Out Of Scope For First Slice."
