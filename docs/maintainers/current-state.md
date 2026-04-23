# Maintainer Current State

This document replaces the old handoff-style notes and is intended for engineers or agents picking up the repository with no prior context.

## Read First

Start with these files:

- [../../README.md](../../README.md)
- [../../AGENTS.md](../../AGENTS.md)
- [../architecture/overview.md](../architecture/overview.md)
- [../layer-1/spec.md](../layer-1/spec.md)
- [../layer-1/status.md](../layer-1/status.md)
- [../layer-1/agent.md](../layer-1/agent.md)
- [../layer-2/spec.md](../layer-2/spec.md)
- [../layer-2/foundation.md](../layer-2/foundation.md)
- [../layer-2/workgroups.md](../layer-2/workgroups.md), [../layer-2/catalog.md](../layer-2/catalog.md), [../layer-2/advertisements.md](../layer-2/advertisements.md), [../layer-2/sessions.md](../layer-2/sessions.md), [../layer-2/contracts.md](../layer-2/contracts.md), [../layer-2/envelopes.md](../layer-2/envelopes.md)
- [../layer-2/status.md](../layer-2/status.md)

Then inspect:

- `cmd/agora`
- `internal/controller`
- `sdk/agent`
- `internal/network/daemon`
- `internal/network/tunnelruntime`
- `internal/persistence`
- `internal/fabric/openziti/automation`
- `internal/api/specs`
- `environment`

## Current Conventions

These conventions are already established and should be preserved unless a deliberate project-wide change is being made:

- logging uses `github.com/michaelquigley/df/dl`
- handwritten JSON/YAML binding and unbinding uses `github.com/michaelquigley/df/dd`
- regenerate REST bindings with `./bin/generate_rest.sh`
- regenerate protobuf/gRPC bindings with `./bin/generate_pb.sh`
- layer-owned internal packages use `internal/fabric/...`, `internal/network/...`, and eventually `internal/collaboration/...`
- cross-cutting packages such as `internal/controller`, `internal/persistence`, `internal/api`, and `internal/clioutput` remain top-level
- human-readable list output uses `go-pretty/table`
- machine-readable list output uses `--json` with raw resource objects

## Current Repo Shape

Major repo areas:

- `cmd/agora`: CLI command wiring and command implementations
- `environment`: local `~/.agora` environment root model
- `sdk/agent`: public SDK. Embeddable Layer 1 runtime, App/Agent scaffolding, and three construction paths (NewDaemon, NewEmbedded, SDK-app via New+Run). Consumed by `agora network start` and by every Macro Pulse example agent.
- `internal/api`: generated `ogen` code and modular OpenAPI specs
- `internal/controller`: handwritten controller logic
- `internal/fabric/openziti/automation`: Layer 0 automation helpers
- `internal/network/daemon`: daemon-client helpers (Dial, Ping, Stop) for CLI → daemon communication
- `internal/network/tunnelruntime`: HTTP/TCP/UDP runtime engine
- `internal/persistence`: PostgreSQL store, repositories, migrations, and integration tests

## What Is Implemented

The repository currently has:

- PostgreSQL-backed Layer 1 control-plane resources
- committed OpenAPI and protobuf generation workflows
- real environment enable/disable
- real tunnel create/serve/connect/delete/grant flows for `http`, `tcp`, and `udp`
- controller-visible tunnel attachments and serve leases with heartbeats and stale reaping
- provider-side and consumer-side tunnel activity logging
- a local `agora network` runtime with heartbeat, reconciliation, and status
- a composite `agora status`

Minimum working Layer 1 is achieved.

## Current CLI Surface

Major command areas:

- `agora controller <configPath>`
- `agora admin ...`
- `agora config ...`
- `agora enable <account-token>`
- `agora disable`
- `agora status`
- `agora tunnel ...`
- `agora network ...`

Important behavior notes:

- `agora tunnel serve` and `agora tunnel connect` use the local runtime by default
- `--foreground` retains the direct one-process debug path
- `agora disable` coordinates with the local runtime before removing environment state
- `agora status` is the operator-facing composite Layer 1 status view
- `agora network status` is daemon-local only

## Operational Notes

- `bootstrap` is a preflight/assertion path for OpenZiti automation
- `unbootstrap` removes Agora-tagged OpenZiti resources only; it does not clean PostgreSQL state
- the local environment root under `~/.agora` is the source of local CLI/runtime state
- controller- and persistence-integration tests require Docker through `testcontainers-go`

## Likely Next Work

Controller health/readiness endpoints, graceful shutdown, and the thin-daemon runtime packaging decision are now resolved. See [../layer-1/spec.md](../layer-1/spec.md) "Controller Operational Endpoints" and [../layer-1/agent.md](../layer-1/agent.md) "Packaging Direction."

Remaining Layer 1 operational hardening:

- a documented local development and smoke-test stack
- clearer end-to-end operational validation for enable, serve, connect, status, and cleanup

Layer 2 work now has a foundation doc ([../layer-2/foundation.md](../layer-2/foundation.md)), a Tier-A spec for workgroups ([../layer-2/workgroups.md](../layer-2/workgroups.md)), and Tier-B skeletons for the other five concepts. See [../layer-2/status.md](../layer-2/status.md) for Tier tracking and build order.

Alongside the Layer 2 specs, the project also has a primary reference demo under [../../examples/macro-pulse/](../../examples/macro-pulse/) with its own formal doc at [../examples/macro-pulse.md](../examples/macro-pulse.md). The demo is scaffolded but not yet runnable end-to-end — it advances one slice at a time as Layer 2 slices ship. See [../examples/index.md](../examples/index.md) for the example-set overview.

Metrics and limits are intentionally deferred to post-MVP work and tracked in [../roadmap/post-mvp.md](../roadmap/post-mvp.md).

## Verification Notes

Useful commands:

```bash
go test ./...
```

If a clean cache is useful locally:

```bash
export GOCACHE=/tmp/agora-go-build
export GOMODCACHE=/tmp/agora-go-mod
go test ./...
```
