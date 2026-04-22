# Layer 1 (Network) Agent

This document is the canonical design and implementation reference for the long-lived local Layer 1 runtime behind `agora network`.

It describes:

- the intended local runtime ownership model
- the local control and persistence model
- the implemented phase-by-phase rollout
- the acceptance criteria for the v1 local runtime

## Current Status

The Layer 1 agent plan is complete through its original phase 5 scope.

Implemented outcomes:

- local runtime package under `internal/network/agent`
- local gRPC-over-UDS control API
- `~/.agora/network.json` desired-state persistence
- `agora network start|stop|status`
- agent-owned environment heartbeat
- agent-hosted `serve` and `connect` runtime by default
- restart and reconnect reconciliation for desired runtime state
- composite `agora status`

## Goals

The v1 local runtime is intended to:

- run as an explicit long-lived local process
- host multiple serves and connects in one process
- own environment heartbeat and tunnel liveness loops
- persist desired local runtime state under `~/.agora`
- restore desired serves and connects after restart
- expose a local control API for the CLI
- preserve `serve` and `connect` as the user-facing verbs
- retain a direct foreground debug path

## Non-Goals

The v1 runtime does not need to:

- auto-start itself at boot or login
- expose a web UI
- store desired runtime state in the controller
- solve Layer 2 concerns
- replace the controller REST API with a second remote control protocol

## Process Model

The local runtime is user-managed through:

- `agora network start`
- `agora network stop`
- `agora network status`

The local control channel is:

- gRPC
- UDS-only in v1
- filesystem-permission secured

The runtime is the normal owner of active Layer 1 behavior. Foreground command execution remains a debug bypass rather than the primary operating mode.

## Local Files

The current local runtime uses the existing `~/.agora` root.

Important files:

- `environment.json`: enabled environment record
- `identities/environment.json`: enrolled environment identity material
- `network.json`: desired serve/connect state, including the records restored after runtime restart
- `network.sock`: local control endpoint

## Ownership Boundaries

The local runtime owns:

- environment heartbeat
- environment identity loading for runtime use
- serve lifecycle
- connect lifecycle
- serve and attachment heartbeats
- retry and reconciliation logic
- local runtime status

The CLI owns:

- command parsing and output
- one-shot local-root mutations such as enable/disable
- admin operations
- optional `--foreground` debug execution

The controller owns:

- environment enrollment and disable
- tunnel provisioning and deletion
- serve lease records
- attachment records
- authorization and policy
- persisted liveness timestamps

## Heartbeat And Reconciliation Model

Environment heartbeat is owned by the local runtime. If an environment is enabled locally, the runtime starts automatic heartbeat and reports daemon-local heartbeat status.

The current environment-heartbeat defaults are:

- heartbeat interval: `15s`
- liveness TTL: `45s`
- per-request timeout: `10s`

Desired serve/connect state is also runtime-owned. On startup or reload, the runtime:

1. loads local environment state
2. loads desired runtime state
3. starts environment heartbeat if an environment is enabled
4. reconciles desired serves
5. reconciles desired connects

Serve reconciliation:

- ensures the tunnel still exists and matches expected mode/backend
- acquires or reacquires the controller-side serve lease
- starts the local provider runtime
- maintains serve heartbeat
- retries with bounded backoff on failure

Connect reconciliation:

- resolves the tunnel
- creates a fresh controller-side attachment
- starts the local consumer runtime
- maintains attachment heartbeat
- retries with bounded backoff on failure

The current retry policy is:

- initial delay `1s`
- multiplier `2x`
- max delay `30s`
- jitter `20%`

The runtime continues retrying until the desired state is removed. Prerequisite failures such as missing enabled environment state or missing identity material are surfaced as errors but are not hot-looped.

Current heartbeat recovery rules are:

- `not found` or `unauthorized` for a live serve or attachment is treated as permanent for that specific live resource and triggers teardown plus immediate reconcile
- transient serve or attachment heartbeat failures trigger teardown plus reconcile after `3` consecutive failures
- successful heartbeats reset transient failure counters

## State Model

The local runtime currently exposes these serve/connect runtime states:

- `configured`
- `starting`
- `running`
- `error`
- `stopped`

The environment heartbeat state surface is:

- `disabled`
- `starting`
- `online`
- `stale`
- `error`

Serve and connect status also include:

- resolved `tunnel_id`
- live `serve_id` or `attachment_id` when present
- `last_started_at`
- `last_error`
- `retry_attempt`
- `next_retry_at`

## Command-Level Contract

Current command behavior is:

- `agora enable`: controller-direct plus local-root mutation, then best-effort runtime reload
- `agora disable`: asks the runtime to stop managed activity and clear desired state, then disables the controller-side environment and removes local state
- `agora tunnel serve`: delegates to the runtime by default, with `--foreground` as a debug bypass
- `agora tunnel connect`: delegates to the runtime by default, with `--foreground` as a debug bypass
- `agora tunnel delete <name|tt_...>`: coordinates with the runtime before removing controller-side resources
- `agora tunnel delete serve <name|tt_...>`: removes managed provider-side desired state and stops it if live
- `agora tunnel delete connect <name|tt_...> --listen <address>`: removes managed consumer-side desired state for a specific listen address and stops it if live
- `agora tunnel delete <ts_...|ta_...>`: removes a live managed serve or connect by runtime ID
- `agora network status`: daemon-local state only
- `agora status`: broader local-plus-controller Layer 1 state

## Failure Model

The runtime is expected to handle and surface:

- daemon already running
- stale socket without a live daemon
- controller heartbeat rejection
- remotely deleted tunnels
- revoked authorization for connects
- local listen conflicts
- unreachable backend targets
- controller unavailability at startup
- missing or corrupt identity material

Failures must be visible in runtime status, not only in logs.

`agora status` is also expected to degrade gracefully when the controller is unavailable. Local root and local runtime state should remain visible even when controller state cannot be fetched.

## Logging Expectations

The runtime should log:

- startup and shutdown
- socket bind path
- environment heartbeat transitions and failures
- reconciliation decisions
- runtime start and exit events
- retry scheduling

The current logging sink is stdout/stderr through `df/dl`. A future file-backed or streamed log surface is possible but not part of the current design.

## Implementation Summary

### Phase 1: Design And Local Scaffolding

Implemented:

- local runtime package
- desired-state persistence
- `agora network start|stop|status`
- gRPC-over-UDS control API
- best-effort `ReloadEnvironment` wiring

### Phase 2: Environment Heartbeat

Implemented:

- controller environment heartbeat endpoint
- agent-owned environment heartbeat loop
- daemon-local environment heartbeat status

### Phase 3: Agent-Hosted Tunnel Runtime

Implemented:

- agent-hosted serve/connect runtime by default
- `--foreground` direct runtime bypass
- managed runtime status in `agora network status`
- coordinated delete/disable behavior

### Phase 4: Reconciliation And Recovery

Implemented:

- desired-state restore on restart
- bounded retry and last-error visibility
- reconciliation after failure
- retry suppression after explicit removal/disable

### Phase 5: Composite Status

Implemented:

- top-level `agora status`
- merged local root, local runtime, and controller state
- controller-visible active-serve lookup in support of composite status

## Acceptance Criteria

The v1 Layer 1 runtime criteria are satisfied when all of the following are true:

- `agora network start` starts a healthy daemon and binds `network.sock`
- an enabled environment causes automatic environment heartbeats while the daemon is running
- `agora tunnel serve` delegates to the daemon by default
- `agora tunnel connect` delegates to the daemon by default
- multiple serves and connects can run under one daemon process
- serves and connects are restored after daemon restart
- `agora disable` drains daemon-managed runtime before removing local environment state
- `agora network status` reports useful per-runtime state and last errors
- `agora status` combines local and controller state without guessing

Those criteria are now met.
