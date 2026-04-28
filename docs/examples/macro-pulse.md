# Macro Pulse

This document is the formal architecture and rollout reference for the [Macro Pulse](../../examples/macro-pulse/) demo. The operator-facing narrative, running instructions, and agent map live in the demo's own [README](../../examples/macro-pulse/README.md). This document covers the parts relevant to Agora maintainers: architecture, envelope wire shapes, slice-by-slice rollout, and implementation constraints.

## Purpose

Macro Pulse is the primary reference demo for Agora's Layer 2 (Collaboration) functionality. It exercises every Layer 2 concept — workgroups, catalog, advertisements, sessions, contracts, envelopes — in an integrated scenario plausible enough to demonstrate the platform's value to executive audiences.

The scenario: a financial-services firm produces a morning "macro pulse" brief by composing market data, weather signals, and internet-activity signals from four independent data and tool providers, each in its own organization. Agora is the governed substrate across which the orchestrator agent discovers, engages, and composes.

## Architecture

Five organizations, eight agents, seven workgroups:

```
                       ┌───────────────────┐
                       │  enterprise-client │
                       │   └─ pulse-agent   │
                       └─────────┬─────────┘
                                 │ member of all four channels
          ┌──────────────────────┼──────────────────────┐
          │                      │                      │
   ┌──────┴──────┐       ┌──────┴──────┐       ┌──────┴──────┐       ┌──────────────┐
   │ markets-co  │       │ weather-co  │       │ signals-co  │       │ analytics-co │
   │ ├ equity    │       │ └ weather   │       │ ├ search    │       │ ├ correlator │
   │ ├ fx        │       │             │       │ └ news      │       │ └ narrator   │
   │ └ commod.   │       │             │       │             │       │              │
   └─────────────┘       └─────────────┘       └─────────────┘       └──────────────┘
        │                     │                     │                      │
   markets-channel      weather-channel      signals-channel        analytics-channel
   (inter-org)          (inter-org)          (inter-org)            (inter-org)
        │                     │                     │                      │
   markets-internal     weather-internal     signals-internal
   (intra-org)          (intra-org)          (intra-org)
```

Provider-to-provider workgroups deliberately do not exist. `markets-co` and `signals-co` do not share a workgroup. Cross-channel data composition happens only at `pulse-agent`, which is the sole agent with visibility across all four channels.

## Envelope schemas (normative)

Macro Pulse is also the proving ground for the Layer 2 envelope format. Every request/response envelope it exchanges follows these conventions:

- **Headers** (infrastructure-visible, enforceable):
  - `envelope_id` (`evp_...`)
  - `session_id` (`ses_...`)
  - `sender_account_id` (`ac_...`)
  - `sender_organization_id` (`org_...`)
  - `message_type` — dot-delimited, e.g. `markets.equity.request`
  - `content_type` — `application/json` for all Macro Pulse envelopes
  - `timestamp` — RFC3339
  - `correlation_id` — optional; present on responses to tie them to their request
  - `schema_version` — `1`

- **Payloads** (opaque to infrastructure):
  - JSON-encoded per-capability schemas; see each agent's README for field-level shapes

The `message_type` naming convention is `<domain>.<capability>.<action>`:

- domain: `markets`, `weather`, `signals`, `analytics`
- capability: `equity`, `fx`, `commodities`, `current`, `forecast`, `search`, `news`, `correlate`, `narrate`
- action: `request`, `response`, `error`

Every capability exposes exactly three message types: request, response, and error. Contract `allowed_message_types` lists apply this convention.

## Slice rollout

Each Layer 2 slice advances the example. This matrix is the authoritative statement of what ships in which PR:

| Slice | What lands in Macro Pulse | Status |
| --- | --- | --- |
| **Workgroups** | `bootstrap/` Go program that creates the five orgs, nine accounts, seven workgroups, and performs all inter-org invitation handshakes. Demo at this stage: `agora admin workgroup list` shows the full demo topology. | **shipped** |
| **Catalog + advertisements** | Each provider/tool agent runs as a daemon, publishes its advertisement on startup (idempotent on `409 name_in_use`). Demo: `agora catalog search` from `enterprise-client` returns the eight visible advertisements across the four channels; `--workgroup markets-channel` narrows to the three markets feeds. | **shipped** |
| **Sessions** | Each provider/tool agent registers a `session.RegisterHandler(...)` loop that logs and accepts proposals. `pulse-agent` iterates the catalog and calls `session.Propose(...)` + `Session.Close(...)` for each discovered advertisement. Session lifecycle reaches `active` (backing tunnel provisioned on the controller); runtime-level tunnel attach and byte-level ping exchange are deferred to the envelopes slice. Demo: every agent logs a session_id / tunnel_id pairing and clean close. | **shipped** |
| **Contracts** | Each provider/tool agent ensures a shared `macro-pulse-provider-default` contract (MVP terms: `max_duration_seconds=60`, `access_mode=approval_required`) and attaches it to its advertisement. `pulse-agent` reads the snapshot off each session and logs its parameters. The session-duration reaper enforces the `max_duration_seconds` cap via scheduled `close_reason=contract_violation`. Envelope-count and `allowed_message_types` enforcement lands in the envelopes slice. | **shipped** |
| **Envelopes** | Provider agents run `EchoSessionHandler` that reads an inbound envelope and flips `.request` → `.response`. `pulse-agent` sends a per-advertisement ping (`<capability>.request` with a small JSON payload) and logs the reply envelope. The wire format is the finalized frame (1-byte version + 4+4 length-prefixed JSON header + opaque payload); envelope counts flow back to the controller via the `POST /v1/sessions/{id}/envelope-count` heartbeat. `pulse-agent`'s full narrative brief (aggregating real feed data across providers) is post-MVP demo polish. | **shipped** |

Pre-slice (i.e. now), each agent ships a `main.go` skeleton that uses the [`sdk/agent`](../sdk/overview.md) SDK to load the local environment root, open a controller API client, and log "alive" on startup; it idles until SIGTERM. This exercises the Layer-1 side of agent boot and validates that the SDK construction path works end-to-end.

## Constraints and non-goals

- **No LLM dependency in the default path.** `narrator` is template-driven. An optional LLM-backed `narrator` is documented as a post-MVP extension (demonstrates Agora gating LLM access via a session + contract), but does not ship with the first end-to-end Macro Pulse milestone.
- **No paid API keys required.** All external data sources used in `--live` mode are free public APIs: Yahoo Finance, Open-Meteo, Wikipedia Pageviews, GDELT.
- **Fully offline by default.** Snapshot data is checked in; CI and conference-demo use cases never require network access.
- **Reference depth, not production depth.** Error handling, retry logic, and configuration flexibility are demo-appropriate, not production-appropriate. Real operators would layer on their own resilience.
- **Single-controller deployment.** All five "organizations" live in one Agora controller instance. Federation across multiple controllers is not demonstrated and is not in Agora MVP scope.

## Data sources and `--live` fallback

| Source | Data type | API | Auth required |
| --- | --- | --- | --- |
| Yahoo Finance | equity, fx, commodities | `query1.finance.yahoo.com/v8/finance/chart/<symbol>` | none |
| Open-Meteo | current + forecast weather | `api.open-meteo.com/v1/forecast?...` | none |
| Wikipedia Pageviews | search-interest proxy | `wikimedia.org/api/rest_v1/metrics/pageviews/per-article/...` | none |
| GDELT | news volume + tone | `api.gdeltproject.org/api/v2/doc/doc?...` | none |

In `--live` mode, each data agent attempts the live API first. On any network error, parse error, or response-shape mismatch, it falls back to the snapshot data with a warning log. This means demos never fail due to a transient upstream issue — they gracefully degrade to offline behavior.

Snapshot data lives under [`examples/macro-pulse/snapshots/`](../../examples/macro-pulse/snapshots/) as plain JSON files organized by capability. See [`examples/macro-pulse/snapshots/README.md`](../../examples/macro-pulse/snapshots/README.md) for the snapshot schema and update procedure.

## Relationship to Layer 2 documentation

Per-concept design in [`docs/layer-2/`](../layer-2/) is informed by Macro Pulse:

- **Workgroups** ([workgroups.md](../layer-2/workgroups.md)): the Macro Pulse topology is one of the real-world test cases. Five orgs, four inter-org workgroups, three intra-org workgroups, nine accounts — exercises both tenancy model dimensions.
- **Catalog, advertisements** ([catalog.md](../layer-2/catalog.md), [advertisements.md](../layer-2/advertisements.md)): the eight advertisements here drive the capability-naming convention (`<domain>.<capability>`) and the "discover by capability across workgroups" query pattern.
- **Sessions** ([sessions.md](../layer-2/sessions.md)): `pulse-agent` establishes many short sessions (one per request). Exercises session-per-request pattern rather than a long-lived session. Each session carries a bounded contract.
- **Contracts** ([contracts.md](../layer-2/contracts.md)): provides concrete examples for `max_duration`, `max_envelope_count`, `allowed_message_types`. Drives the contract schema toward what agents actually need.
- **Envelopes** ([envelopes.md](../layer-2/envelopes.md)): the envelope header/payload conventions documented here become reference material for envelope spec promotion to Tier A.

Changes in the Layer 2 specs that affect Macro Pulse should be reflected in the corresponding agent READMEs and in this document.
