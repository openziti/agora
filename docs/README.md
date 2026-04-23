# Agora Documentation

This directory is the canonical home for architecture, implementation status, maintainer context, and forward planning.

Use the docs in this order:

- [architecture/overview.md](./architecture/overview.md): product and system architecture across all layers
- [layer-1/spec.md](./layer-1/spec.md): Layer 1 (Network) normative behavior and boundaries
- [layer-1/status.md](./layer-1/status.md): current Layer 1 implementation state and completion criteria
- [layer-1/agent.md](./layer-1/agent.md): detailed local Layer 1 runtime and agent design
- [layer-2/spec.md](./layer-2/spec.md): Layer 2 (Collaboration) architectural overview
- [layer-2/foundation.md](./layer-2/foundation.md): cross-cutting Layer 2 design decisions
- [layer-2/workgroups.md](./layer-2/workgroups.md), [catalog.md](./layer-2/catalog.md), [advertisements.md](./layer-2/advertisements.md), [sessions.md](./layer-2/sessions.md), [contracts.md](./layer-2/contracts.md), [envelopes.md](./layer-2/envelopes.md): per-concept Layer 2 specs
- [layer-2/status.md](./layer-2/status.md): current Layer 2 status, per-concept Tier tracking, and recommended build order
- [sdk/overview.md](./sdk/overview.md): Agora SDK (sdk/agent) — supported integration path for building agent processes
- [examples/index.md](./examples/index.md): reference/demo agent implementations and the example set catalog
- [examples/macro-pulse.md](./examples/macro-pulse.md): Macro Pulse — primary cross-domain reference demo
- [maintainers/current-state.md](./maintainers/current-state.md): maintainer-facing repo state, workflows, and conventions
- [roadmap/post-mvp.md](./roadmap/post-mvp.md): intentionally deferred post-MVP work

Audience guidance:

- Read the `architecture/` and `layer-*` docs for product and technical design.
- Read `maintainers/current-state.md` before making broad repo changes or choosing the next implementation slice.
- Treat `roadmap/post-mvp.md` as deferred work, not MVP scope.

The root [README.md](../README.md) remains the quick-start entry point, and [AGENTS.md](../AGENTS.md) remains the contributor guidance file.
