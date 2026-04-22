# Layer 2 (Collaboration) Status

This document records the current implementation state of Layer 2 (Collaboration) and the recommended order for beginning Layer 2 work.

## Current Position

Layer 2 is currently architectural, not implemented.

The repository does not yet have:

- workgroup persistence and API surface
- catalog APIs or controller logic
- advertisement lifecycle
- session lifecycle
- contract evaluation
- envelope handling infrastructure
- Layer 2 CLI commands
- Layer 2 SDK surfaces

## What Exists Already That Layer 2 Can Build On

Layer 2 is not starting from zero. It already has the Layer 1 substrate it depends on:

- multi-tenant organizations and accounts
- controller-managed environment lifecycle
- real tunnel provisioning and connectivity
- local runtime ownership for long-lived network behavior
- controller-visible liveness, serve, and attachment state
- PostgreSQL-backed controller state
- OpenAPI-driven API workflow

This means the next Layer 2 work can focus on collaboration semantics rather than first building transport and identity.

## Recommended Build Order

The cleanest order for initial Layer 2 work is:

1. workgroup model and membership semantics
2. catalog and advertisement persistence plus visibility rules
3. session model and engagement lifecycle
4. declarative contract model and controller-side evaluation
5. envelope model and session-governed message semantics

This order keeps visibility and policy boundaries in place before sessions and contracts depend on them.

## Out Of Scope For The First Layer 2 Slice

These items should not shape the first Layer 2 implementation slice:

- metrics
- limits
- semantic catalog search
- programmatic contracts
- memory-oriented products
- deep protocol bridge behavior

Those are tracked separately in [../roadmap/post-mvp.md](../roadmap/post-mvp.md).
