# Layer 2 (Collaboration) Specification

This document defines the intended Layer 2 (Collaboration) model in Agora.

Layer 2 is built entirely on top of Layer 1 (Network). It adds discovery, engagement, and governance semantics for agent-to-agent collaboration without introducing a second transport substrate.

## Purpose

Layer 2 provides:

- workgroup-scoped visibility and interaction boundaries
- intrinsic discovery through a catalog
- capability declarations through advertisements
- governed communication through sessions and contracts
- structured communication through envelopes

At the infrastructure level, Layer 2 composes Layer 1 primitives rather than replacing them.

## Core Layer 2 Concepts

### Workgroup

A workgroup is the policy boundary that controls visibility and interaction scope.

The intended model is:

- intra-organization workgroups for internal collaboration boundaries
- shared inter-organization workgroups for cross-org collaboration
- flat workgroups in the initial design, without hierarchy

Workgroups scope what an identity can discover and with whom it may interact.

### Catalog

The catalog is an intrinsic controller service for discovery. It acts as:

- a directory for deterministic lookup
- a discovery surface for filtered search

Visibility is enforced at the discovery layer. If an identity is not entitled to see an advertisement, the advertisement should not appear in catalog results at all.

### Advertisement

An advertisement is an agent’s declaration of what it can do and how to engage with it.

An advertisement is expected to describe:

- name and description
- declared capabilities
- supported interaction patterns
- visibility scope through workgroups
- contract requirements for engagement

Advertisements are intended to be persistent and re-associated with the advertising identity as it reconnects.

### Session

A session is the governed communication channel between collaborating participants.

The intended model is:

- an engagement flow resolves whether a session can be created
- a session is backed by Layer 1 connectivity
- session lifecycle is explicitly tracked
- either side or policy may close the session

At the infrastructure level, a session is a tunnel plus governance metadata.

### Contract

A contract defines the negotiated terms for a session.

The initial contract direction is declarative and should support at least:

- maximum session duration
- maximum envelope count
- allowed message types
- required workgroup memberships
- maturity or trust requirements
- access mode such as open, approval-required, or policy-evaluated

Programmatic contracts are a later extension rather than the starting point.

### Envelope

An envelope is the structured message format for Layer 2 session traffic.

The key design split is:

- headers are infrastructure-visible and enforceable
- payloads are opaque to the infrastructure

This enables governance, attribution, and observability without requiring the controller to understand every payload format.

## Relationship To Layer 1

Layer 2 depends on these Layer 1 capabilities:

- enrolled identities and environment lifecycle
- secure connectivity through tunnels
- org-scoped authorization
- local runtime and controller liveness model

Layer 2 does not define a new network substrate. It adds governance and discovery semantics on top of Layer 1 connectivity.

## A2A Compatibility

Agora’s A2A compatibility is structural rather than controller-bridged by default.

The intended model is:

- advertisements may carry A2A-aligned metadata
- envelopes may carry A2A payloads as opaque message bodies
- external interoperability should happen through explicit entrypoints rather than weakening zero-trust assumptions

## Gateway Composition

`llm-gateway` and `mcp-gateway` are natural infrastructure participants in the Agora model.

They are primarily Layer 1 consumers, but they also provide natural integration points for:

- behavioral monitoring
- content inspection
- policy enforcement at application boundaries

## Deferred Layer 2 Extensions

The broader architecture still includes several deferred collaboration capabilities, including:

- semantic catalog search
- programmatic contracts
- richer envelope extensions
- deeper A2A bridge behavior
- memory-oriented collaboration services built on top of the base primitives

Those items are tracked in [../roadmap/post-mvp.md](../roadmap/post-mvp.md).
