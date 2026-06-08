# Layer 1 (Network) Status

This document records the current implementation state of Layer 1 (Network), the checklist for calling it minimally working, and the remaining work for broader operational completion.

## Current Position

Layer 1 should now be viewed as:

- beyond control-plane scaffolding
- functionally complete for a minimum working network slice
- still incomplete in some operational and post-MVP areas

Minimum working Layer 1 is achieved. The remaining work is mostly operational hardening plus explicitly deferred post-MVP capabilities.

## What Exists Now

Implemented Layer 1 behavior includes:

- PostgreSQL-backed organizations, accounts, environments, and tunnels
- OpenAPI-driven controller API with committed `ogen` bindings
- real environment enable/disable with controller-created identity material
- real HTTP, TCP, and UDP tunnel provisioning and connectivity
- tunnel grants, attachment tracking, and serve-lease tracking
- stale reaping for attachments and serves
- a local `agora network` runtime over gRPC+UDS
- agent-owned environment heartbeat and environment liveness reporting
- agent-hosted `serve` and `connect` runtime with reconciliation after restart/failure
- agent-backed `serve` and `connect` by default, with `--foreground` direct-runtime bypass for debugging
- SDK-native direct provider tunnels through `tunnel.Create`, `tunnel.Listen`, and `tunnel.Delete`, returning a raw `net.Listener` without the managed runtime
- unified tunnel delete semantics for both durable tunnel resources and managed runtime state
- composite `agora status` covering local root state, local agent state, controller environment state, owned tunnels, active serve details, and attachment counts
- meaningful provider-side and consumer-side request/session logging

## Minimum Working Layer 1 (Network) Checklist

This checklist is complete.

- [x] an operator can create an organization and account
- [x] an account can authenticate and enable an environment
- [x] environment enablement returns real controller-created identity material
- [x] the local runtime exists and owns enabled-environment state
- [x] a local environment can heartbeat its own liveness to the controller through that runtime
- [x] `agora status` reports meaningful local and controller-side Layer 1 state
- [x] a tunnel can be provisioned and served in `http`, `tcp`, or `udp` mode
- [x] a second environment can connect to that tunnel if authorized
- [x] access and serve heartbeats are tracked and stale state is reaped
- [x] enabled environments and served tunnels are re-established after local runtime restart or reconnect
- [x] an embedded process can provision a direct tunnel and serve its own protocol on a raw overlay listener

## Remaining Work For Broader Layer 1 Completion

The following work still remains if the goal is to call Layer 1 operationally solid rather than merely minimum-working:

- extraction of the local runtime into an embeddable library form alongside the existing standalone daemon, per the revised packaging direction (see [agent.md](agent.md) "Packaging Direction")
- SDK-native direct dialer primitives (`Attach` / `Detach` / `Dial`) and their attachment cleanup model
- a documented and repeatable local development and smoke-test stack
- clearer end-to-end operational validation for enable, serve, connect, status, and cleanup

Controller health and readiness endpoints and graceful shutdown of the controller's reaper goroutines are resolved. See [spec.md](spec.md) for the endpoint surface and the controller source for shutdown behavior.

## Broader Layer 1 Completion Checklist

This checklist is not complete.

- [x] controller health/readiness and shutdown behavior are solid
- [x] the local runtime packaging direction is finalized (dual delivery: standalone daemon + embeddable library)
- [ ] the local runtime library extraction is reflected in the implementation
- [ ] SDK-native direct dialer primitives are implemented
- [ ] the project has a documented and repeatable end-to-end operational validation path
- [ ] metrics are implemented (deferred; see [../../future/roadmap/post-mvp.md](../../future/roadmap/post-mvp.md))
- [ ] limits are implemented (deferred; see [../../future/roadmap/post-mvp.md](../../future/roadmap/post-mvp.md))

## Deferred Post-MVP Work

These items are intentionally deferred and are not part of the minimum-working Layer 1 milestone:

- metrics
- limits

They remain part of the broader architecture and are tracked in [../../future/roadmap/post-mvp.md](../../future/roadmap/post-mvp.md).

## Testing Status

The repo has useful coverage for the current Layer 1 surface:

- persistence integration tests
- controller HTTP integration tests
- focused automation tests
- focused runtime tests
- local runtime and CLI tests

Some integration tests require Docker through `testcontainers-go`, so full end-to-end verification depends on a machine with Docker access.
