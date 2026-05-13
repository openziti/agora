# Post-MVP Roadmap

This document tracks work that is intentionally deferred until after the current Layer 1 MVP milestone.

These items remain part of the broader architecture, but they are not part of the minimum-working Layer 1 target.

## Deferred Layer 1 Work

### Metrics

Metrics remain part of the long-term Layer 1 architecture, but they are deferred to post-MVP work.

The intended direction still includes:

- PostgreSQL/TimescaleDB-backed metric storage
- metrics ingest API
- metrics query API
- automatic emission of controller and network metrics
- org-scoped metric visibility

### Limits

Limits also remain part of the broader Layer 1 architecture, but they are deferred to post-MVP work.

The intended direction still includes:

- per-account resource limits
- controller-side enforcement during environment and tunnel lifecycle
- an administrative surface for setting and changing limits

## Deferred Collaboration And Platform Work

The broader architecture also includes intentionally deferred items such as:

- semantic catalog search
- programmatic contracts
- richer envelope extensions
- deeper A2A bridge behavior
- workgroup hierarchy
- memory-oriented collaboration services built on top of Agora primitives

## Notes

Deferring these items does not remove them from the architecture. It only means they should not be treated as MVP blockers or mixed into the current Layer 1 completion criteria.
