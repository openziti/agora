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

Layer 2 is MVP-complete. All five concepts (workgroups, catalog, advertisements, sessions, contracts, envelopes) have Tier-A specs and shipped implementations. See [../layer-2/status.md](../layer-2/status.md) for the per-slice status table; [../layer-2/foundation.md](../layer-2/foundation.md) holds the cross-cutting decisions. Post-MVP extensions are tracked in [../roadmap/post-mvp.md](../roadmap/post-mvp.md).

Workgroup implementation surfaces:

- persistence: `internal/persistence/workgroups.go`, `workgroup_invitations.go`, `workgroup_memberships.go` + migration `0003_layer2_workgroups.sql`
- API: `internal/api/specs/workgroups/` (account-token) plus admin endpoints under `internal/api/specs/admin/`
- controller: per-handler files (`createWorkgroup.go`, `acceptWorkgroupInvitation.go`, etc.) + `workgroup_helpers.go` for auth and state aggregation
- CLI: `cmd/agora/workgroup*.go` and `cmd/agora/adminWorkgroup*.go`
- demo bootstrap: `examples/macro-pulse/cmd/macro-pulse-bootstrap/main.go` provisions the full Macro Pulse topology (5 orgs, 9 accounts, 7 workgroups, plus same-org workgroup memberships) against the admin and account-token endpoints

Catalog + advertisements implementation surfaces (slice 2):

- persistence: `internal/persistence/advertisements.go` + migration `0004_layer2_advertisements.sql` (jsonb capabilities/interaction-patterns columns; `text[]` workgroup_scopes; cursor-based search via keyset pagination)
- API: `internal/api/specs/advertisements/` (5 endpoints) and `internal/api/specs/catalog/` (1 search endpoint)
- controller: `publishAdvertisement.go`, `listAdvertisements.go`, `getAdvertisement.go`, `updateAdvertisement.go`, `retractAdvertisement.go`, `searchCatalog.go` + `advertisement_helpers.go` (visibility + scope validation)
- CLI: `cmd/agora/advertise*.go` and `cmd/agora/catalog*.go`
- Macro Pulse advancement: each provider/tool agent calls `agentutil.EnsureAdvertisement` on startup; `pulse-agent` calls `client.SearchCatalog` for discovery

Sessions implementation surfaces (slice 3):

- persistence: `internal/persistence/sessions.go` + migrations `0005_advertisement_tunnel_mode.sql` (adds `tunnel_mode` column to advertisements) and `0006_layer2_sessions.sql` (sessions table with state machine and audit fields)
- API: `internal/api/specs/sessions/` (6 account-token endpoints: propose/list/get/accept/reject/close) plus admin close
- controller: `proposeSession.go`, `listSessions.go`, `getSession.go`, `acceptSession.go` (wires into `TunnelsRepository.Create` + `TunnelGrants.Create` for provisioning), `rejectSession.go`, `closeSession.go` (with shared `teardownSession` that also deprovisions the backing tunnel), `adminCloseSession.go`, `session_helpers.go`
- CLI: `cmd/agora/session*.go` and `cmd/agora/adminSession*.go`
- SDK: `sdk/agent/session/` exposes `Propose`, `RegisterHandler`, and `Session.Close`; thin wrappers over `a.Controller()` REST calls with 1 Hz polling for state transitions
- Macro Pulse advancement: all 8 provider/tool agents use `agentutil.LoggingSessionHandler` via `session.RegisterHandler`; `pulse-agent` iterates the catalog and calls `session.Propose` + `Session.Close` per advertisement
- Scope note: sessions reach `active` with the backing tunnel resource provisioned on the controller; local runtime `EnsureServe`/`EnsureConnect` attach and byte-level session I/O are deferred to the envelopes slice (slice 5)

Contracts implementation surfaces (slice 4):

- persistence: `internal/persistence/contracts.go` + migration `0007_layer2_contracts.sql` (adds `contracts` table + nullable `advertisements.contract_id` with `on delete set null`); existing sessions table's `contract_snapshot` jsonb column is now populated at accept time
- API: `internal/api/specs/contracts/` (5 endpoints: create/list/get/update/delete); advertisements schemas extended with `contractId`; session response extended with `contractSnapshot`
- controller: `createContract.go`, `listContracts.go`, `getContract.go`, `updateContract.go`, `deleteContract.go` + `contract_helpers.go` (ownership + visibility + admission evaluation + snapshot production); `publishAdvertisement` / `updateAdvertisement` validate contract ownership; `acceptSession` evaluates admission terms (workgroup memberships, account age) and writes the frozen snapshot; `sessionDurationReaper.go` closes expired sessions with `close_reason=contract_violation`
- CLI: `agora advertise publish/update --contract` + describe extensions (no standalone `agora contract` verb per spec)
- SDK: `*session.Session` gains `ContractSnapshot *api.ContractSnapshot`
- Macro Pulse advancement: each provider/tool agent ensures a shared `macro-pulse-provider-default` contract via `agentutil.ContractSpec` and attaches it to its advertisement; `pulse-agent` logs the snapshot parameters it receives per session
- Scope note: contracts persist `max_envelope_count` and `allowed_message_types` but enforcement of those two dimensions ships in the envelopes slice, which wires envelope-level observability

Envelopes implementation surfaces (slice 5):

- persistence: migration `0008_envelope_mtu_count.sql` adds `contracts.max_envelope_bytes` and `sessions.envelope_count`; `SessionsRepository.RecordEnvelopeCount` uses max-seen semantics for the count; `ContractSnapshot` persistence struct gained `MaxEnvelopeBytes`
- API: new endpoint `POST /v1/sessions/{id}/envelope-count`; `envelopeCount` field on session response; `maxEnvelopeBytes` on contract + snapshot; `backendAddress` optional field on `acceptSessionRequest`
- controller: `reportSessionEnvelopeCount.go` (provider-only, 404-leak pattern, max-seen); `acceptSession` honors `backendAddress` on the tunnel it provisions; `contract_helpers.go` + `createContract.go` + `updateContract.go` thread the new `max_envelope_bytes` field
- SDK: `sdk/agent/session/envelope.go` with the wire-format encoder/decoder (1-byte `FrameVersion` + 4+4 length-prefixed JSON header + opaque payload, 10 MiB platform ceiling); `sdk/agent/session/transport.go` with `attachConsumerStream` / `attachProviderStream` / `countReporterLoop` / `teardownTransport`; `sdk/agent/session/transport_enforcement.go` with `Session.Send` / `Session.Receive` and contract enforcement (message-type wildcard, max-count, max-bytes). `Propose` attaches consumer transport on `active`; `RegisterHandler`/`handleProposal` allocate provider listener before accept and pass `backendAddress` through the API
- CLI: `agora session send <adv_...>` one-shot (propose → send → receive → close)
- Macro Pulse advancement: `agentutil.EchoSessionHandler` replaces `LoggingSessionHandler`; providers echo inbound envelopes with `.request` → `.response`; `pulse-agent` sends a per-advertisement ping and logs the response envelope
- Scope note: MVP envelope transport is `tcp`-carrier only. `http`/`udp` carriers, envelope-level signing, richer A2A bridging, session multiplexing, and per-envelope contract-violation error envelopes are all post-MVP.

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
