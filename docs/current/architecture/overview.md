# Agora Architecture Overview

This document is the canonical high-level architecture overview for Agora. It describes the layered model, core system boundaries, and the major design choices that shape implementation across the repository.

Detailed layer-specific behavior lives in:

- [Layer 1 (Network) spec](../layer-1/spec.md)
- [Layer 1 (Network) status](../layer-1/status.md)
- [Layer 1 (Network) agent](../layer-1/agent.md)
- [Layer 2 (Collaboration) spec](../layer-2/spec.md)
- [Layer 2 (Collaboration) foundation](../layer-2/foundation.md)
- Layer 2 per-concept specs: [workgroups](../layer-2/workgroups.md), [catalog](../layer-2/catalog.md), [advertisements](../layer-2/advertisements.md), [sessions](../layer-2/sessions.md), [contracts](../layer-2/contracts.md), [envelopes](../layer-2/envelopes.md)
- [Layer 2 (Collaboration) status](../layer-2/status.md)
- [Post-MVP roadmap](../../future/roadmap/post-mvp.md)

## Overview

Agora is a zero-trust network for agent-to-agent communication built on OpenZiti. It provides:

- secure connectivity primitives that are useful on their own
- governed collaboration primitives built on top of that connectivity
- a self-hostable controller and local runtime model

Agora serves two related use cases:

- secure service connectivity without any agent-specific concepts
- governed agent collaboration with discovery, engagement, and policy

## Layered Model

Agora is organized in three layers:

- Layer 0 (Fabric): OpenZiti Fabric providing cryptographic identity, mutual authentication, end-to-end encryption, dark-by-default connectivity, and overlay routing
- Layer 1 (Network): organizations, accounts, environments, tunnels, and the local network runtime
- Layer 2 (Collaboration): workgroups, catalog, advertisements, sessions, contracts, and envelopes

Layer 1 is useful without Layer 2. Layer 2 depends on Layer 1.

## Shared System Concepts

The core cross-layer concepts are:

- Organization: the top-level tenant boundary
- Account: a user identity within an organization
- Environment: a provisioned network presence with controller-managed identity material
- Tunnel: the fundamental connectivity primitive in Layer 1
- Identity: certificate-based proof bound to an enrolled participant on the network

Layer 2 adds higher-order collaboration concepts on top of those primitives rather than introducing a second transport substrate.

## Controller Architecture

Agora uses a centralized controller as the source of truth for state, policy, and coordination. The controller owns:

- organization and account administration
- environment lifecycle
- tunnel lifecycle and authorization
- Layer 2 metadata and policy when that layer is implemented
- operational coordination such as liveness tracking and status

The controller architecture is intentionally simple:

- PostgreSQL is the only relational data substrate
- OpenAPI 3.x is the controller API source of truth
- `ogen`-generated server and client bindings define the typed REST contract
- handwritten controller logic lives behind the generated interfaces

The system is self-hosting oriented. A controller plus PostgreSQL is the core deployment model.

## Runtime Model

Agora uses a long-lived local Layer 1 runtime managed through `agora network`. Today that runtime exists as a thin local daemon with a gRPC-over-UDS control plane.

The architectural direction remains:

- keep the local runtime as the owner of environment liveness and tunnel runtime
- preserve CLI verbs like `serve` and `connect` as the user-facing interface
- allow a future embeddable library form of the same runtime where that makes sense

The local runtime is a Layer 1 concern. It is distinct from future Layer 2 collaboration participants, even though both concepts use the word "agent" in casual discussion.

## CLI Shape

The top-level CLI is organized around operation domains:

- `agora admin ...` for administrative controller operations
- `agora enable` / `agora disable` / `agora status` for operator-facing Layer 1 workflow
- `agora tunnel ...` for tunnel lifecycle and runtime interaction
- `agora network ...` for local runtime control

Layer 2 will add collaboration-specific operations on top of this surface rather than replacing the Layer 1 command model.

## A2A And Gateway Composition

Agora is structurally compatible with A2A rather than acting as a protocol bridge by default:

- Layer 2 messages can carry A2A-aligned payloads
- advertisements can expose A2A-style metadata
- external interoperability is expected to happen through explicit network entrypoints, not by weakening the zero-trust model

Agora also composes with infrastructure services such as `llm-gateway` and `mcp-gateway`. Those services are Layer 1 consumers first and can later participate in Layer 2 patterns where useful.

## Deferred Scope

Not every item from the broader architecture is part of the current MVP or immediate implementation plan.

In particular:

- metrics are deferred to post-MVP work
- limits are deferred to post-MVP work
- several broader collaboration and governance enhancements remain intentionally deferred

Those items are tracked in [future/roadmap/post-mvp.md](../../future/roadmap/post-mvp.md) rather than mixed into the MVP-layer specs.
