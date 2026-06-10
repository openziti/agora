# Layer 1 (Network) Specification

This document defines the intended behavior and boundaries of Layer 1 (Network) in Agora.

Layer 1 provides secure connectivity primitives without requiring any Layer 2 collaboration features. It is the operator-facing network substrate built on top of Layer 0 (Fabric).

## Purpose

Layer 1 is responsible for:

- multi-tenant network administration through organizations and accounts
- environment enrollment and controller-managed identity lifecycle
- named private tunnels for HTTP, TCP, and UDP connectivity
- local runtime ownership for liveness, serve/connect hosting, and recovery
- controller-visible tracking of active provider and consumer tunnel use

Layer 1 is intentionally useful on its own for infrastructure and service connectivity.

## Core Resources

Layer 1 resources are:

- organizations
- accounts
- environments
- tunnels
- tunnel grants
- tunnel attachments
- tunnel serves

The primary tenancy rule is organization scope. Layer 1 resources are org-scoped unless a broader architecture decision explicitly says otherwise.

## Environment Model

An environment is a provisioned network presence for an account. Enabling an environment must:

- authenticate with an account token
- provision controller-managed identity material on the fabric
- return enrollment material to the local CLI/runtime
- persist enough local state for later controller interactions

Disabling an environment must:

- remove the controller-side environment presence
- clean up associated local environment state
- coordinate with the local runtime so managed tunnel activity is drained first

Environment liveness is owned by the local runtime and reported to the controller through explicit heartbeat.

## Tunnel Model

A tunnel is the Layer 1 connectivity primitive. Tunnels are:

- persistent
- named
- owned by an account within an organization
- bound to the environment that serves them
- one of two provider shapes: `proxy` or `direct`

Supported tunnel modes in the current Layer 1 surface are:

- `http`
- `tcp`
- `udp`

The user-facing verbs are:

- `serve`: provider-side runtime ownership
- `connect`: consumer-side runtime ownership

By default, `agora tunnel serve` and `agora tunnel connect` delegate to the local `agora network` runtime. Both commands support `--foreground` as a debug bypass that runs the runtime directly in the CLI process and does not alter agent-managed desired state.

Proxy tunnels are the managed runtime shape. They require a backend target,
create tunnel serve records when hosted, and are the shape used by
`agora tunnel serve` and `tunnel.EnsureServed`.

Direct tunnels are the SDK-native provider shape. They have no backend
target because the embedding process serves the overlay listener itself.
They are provisioned explicitly through `sdk/agent/tunnel.Create`, listened
through `sdk/agent/tunnel.Listen`, and deleted through
`sdk/agent/tunnel.Delete`. `Listen` returns a raw `net.Listener` and does
not create a tunnel serve record or heartbeat. Direct tunnels currently
support stream modes (`http` and `tcp`) for listening; packet-shaped direct
UDP is deferred.

Tunnel attachments have two consumer shapes: `proxy` and `dialer`. Proxy
attachments are created by the managed connect runtime and require a local
listen address because the runtime binds a local proxy port. Dialer
attachments are created explicitly through `sdk/agent/tunnel.Attach`, have
no local listen address, and authorize raw overlay dials through
`sdk/agent/tunnel.Dial`. `Dial` returns a raw `net.Conn`, does not create a
managed connect actor, and does not heartbeat. A dialer attachment remains
an active authorization until `Detach` or another revocation path removes
its stored dial policy. Direct dialers currently support stream modes
(`http` and `tcp`); packet-shaped UDP dialing is deferred.

The durable resource is the tunnel itself. The live controller-visible runtime records are:

- tunnel serves for active provider-side hosting
- tunnel attachments for active or historical consumer-side connection records

Both tunnel serves and tunnel attachments are explicit controller-side records with `active`, `stale`, and `disconnected` states. Only one active serve may exist for a tunnel at a time, and the controller exposes a singular active-serve read surface for status composition. Attachment stale reaping applies to proxy attachments only; dialer attachments are not heartbeat-backed and are removed by explicit detach or revocation cleanup.

## Authorization And Visibility

Layer 1 tunnel authorization follows these rules:

- the owning account manages its own tunnels
- tunnel names are unique within an organization among active tunnels, using case-insensitive matching
- the owning environment serves its own tunnel; serve is environment-bound
- any environment enrolled to the owning account may connect to the account's tunnels
- same-organization accounts may connect only when explicitly granted
- active access is represented by controller-visible attachment records rather than inferred from low-level fabric state

This keeps authorization and observability at the Agora level instead of pushing policy reasoning down into raw OpenZiti inspection.

## Runtime Deletion Semantics

Layer 1 distinguishes between deleting the durable tunnel resource and removing managed runtime state.

The current CLI surface is:

- `agora tunnel delete <name|tt_...>` to delete the tunnel resource
- `agora tunnel delete serve <name|tt_...>` to remove managed provider-side runtime state
- `agora tunnel delete connect <name|tt_...> --listen <address>` to remove managed consumer-side runtime state for a specific listen address
- `agora tunnel delete <ts_...|ta_...>` as a convenience path for deleting a live serve or attachment by runtime ID

When deleting a tunnel resource, the CLI coordinates with the local runtime first so any desired serve/connect state for that tunnel is drained before controller-side deletion.

## Local Runtime Ownership

The default owner of active Layer 1 behavior is the local `agora network` runtime.

The runtime owns:

- environment heartbeat
- desired serve/connect state
- serve/connect runtime hosting
- serve lease and attachment heartbeats
- restart and reconnect reconciliation
- local runtime status

The CLI remains the user-facing control surface, but the runtime is the long-lived owner of normal Layer 1 operation.

## Local State

Layer 1 local state is rooted under `~/.agora`.

The important state categories are:

- controller endpoint configuration
- enabled environment metadata and identity material
- local network runtime desired state
- local runtime control socket

The current local files of interest are:

- `environment.json` for the enabled environment record
- `identities/environment.json` for enrolled environment identity material
- `network.json` for desired serves and connects
- `network.sock` for the local runtime control socket

Layer 1 state is intentionally local-machine state. Backend targets, local listen addresses, and runtime desired state are not controller-owned configuration.

## Status Surfaces

Layer 1 exposes two different status views:

- `agora network status`: daemon-local runtime state only
- `agora status`: composite local-plus-controller Layer 1 state

The composite view exists so operators can reason about:

- whether an environment is locally enabled
- whether the local runtime is healthy
- whether the controller sees the environment as live
- which tunnels are owned, currently served, and actively connected

`agora status` is intentionally best-effort. It composes:

- local root state, even when the runtime is not running
- local runtime state when `agora network` is reachable
- controller environment state and derived liveness
- owned tunnel state, including active serve lookup and attachment counts

Controller errors should degrade the controller portion of the output rather than making the whole command unusable.

## Controller Operational Endpoints

In addition to the typed REST API, the controller exposes two handwritten operational endpoints outside the OpenAPI surface:

- `GET /health`: process liveness. Returns `200 OK` as long as the controller process is running.
- `GET /ready`: readiness. Returns `200 OK` when the persistence store is reachable, `503 Service Unavailable` otherwise.

These endpoints are intended for container orchestration liveness and readiness probes. They are intentionally not part of the OpenAPI contract because they report operational state rather than typed resources.

The controller also performs graceful shutdown on `SIGINT` or `SIGTERM`: in-flight HTTP requests are allowed to drain within a bounded timeout, the background reaper goroutines are cancelled and awaited, and the persistence store is closed before the process exits.

## Explicit MVP Boundary

Layer 1 MVP intentionally excludes:

- metrics
- limits

Those capabilities remain part of the broader architecture, but they are deferred to post-MVP work and tracked in [../../future/roadmap/post-mvp.md](../../future/roadmap/post-mvp.md).
