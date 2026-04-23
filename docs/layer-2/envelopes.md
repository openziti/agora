# Layer 2: Envelopes

Tier: B (skeleton). See [foundation.md](foundation.md) for cross-cutting decisions, especially the **session-to-tunnel 1:1 relationship** which envelope transport depends on.

## Purpose

An envelope is the structured message format for Layer 2 session traffic. Envelopes provide two things the raw tunnel does not: **infrastructure-visible headers** that enable governance, attribution, and observability; and **opaque payloads** that let participants exchange arbitrary content (including A2A-formatted messages) without requiring the controller to parse every payload format.

The envelope is the unit of contract enforcement and audit. Contract terms like message-type filtering and envelope-count caps operate on envelope headers. Every message a session participant sends is wrapped in an envelope; raw tunnel bytes outside the envelope format are not Layer 2 traffic.

## Relationship To Other Layer 2 Concepts

- **Sessions** are the carrier. Envelopes flow through a session's backing Layer 1 tunnel and are associated with that session via the `session_id` header.
- **Contracts** bound envelopes: `allowed_message_types` filters on an envelope's `message_type` header; `max_envelope_count` counts envelopes per session.
- **Workgroups** are referenced in envelope headers for audit attribution but do not themselves filter envelope flow (the session already scopes to a workgroup).
- **Layer 1 tunnels** transport envelopes as byte streams. The envelope wire format is independent of the Layer 1 tunnel mode (`http`, `tcp`, `udp`) but is designed primarily for stream-oriented carriers in MVP.

## Data Model

An envelope has a **header** and a **payload**. The header is structured and infrastructure-visible; the payload is opaque to infrastructure.

- **Envelope header** (MVP, `schema_version = 1`)
  - `schema_version` (integer, MVP `1`)
  - `envelope_id` (`evp_...`; assigned by the sender; used for correlation and audit)
  - `session_id` (`ses_...`)
  - `sender_account_id`, `sender_organization_id`
  - `message_type` (application-level type string; matched against contract `allowed_message_types`)
  - `content_type` (payload MIME type hint; opaque to contract enforcement)
  - `timestamp` (sender wall-clock; informational)
  - `correlation_id` (optional; sender-assigned for request-response correlation)

- **Envelope payload**
  - opaque bytes, up to a maximum size (see Open Questions)

Envelopes are not first-class REST resources in MVP. `envelope_id` exists for correlation and audit but is not addressable via controller APIs.

## Lifecycle / State Machine

Envelopes have no persistent lifecycle. Each envelope is fire-and-forget within its session:

- sender composes header + payload and writes to the session's tunnel
- receiver reads header + payload from the session's tunnel
- controller may count envelopes or inspect headers for audit/enforcement purposes without retaining envelope contents

There are no envelope states. There is no envelope delivery acknowledgement protocol at the envelope layer — reliability is whatever the Layer 1 tunnel mode provides (TCP-level reliability for `tcp`, stream-level semantics for `http`).

## Authorization

Placeholder (Tier A). A sender must be a participant in the session the envelope references; `sender_account_id` must match the authenticated identity of the sending runtime.

## Visibility Rules

Placeholder (Tier A). Envelope contents are visible only to the session's provider and consumer (and admin audit surfaces). Controller infrastructure inspects headers but does not retain payloads.

## Controller API Surface

Placeholder (Tier A). Envelopes do not get a dedicated OpenAPI module; envelope observability surfaces (e.g. per-session envelope counts in session describe) live under `internal/api/specs/sessions/`.

## Agent / Runtime Surface

Placeholder (Tier A). Envelope reading and writing happens in the session's runtime — provider and consumer — and flows through the session's backing Layer 1 tunnel. The exact framing and encoding are Open Questions.

## CLI Surface

Placeholder (Tier A). Per [foundation.md](foundation.md), envelopes are sent through `agora session send`, not through a top-level `agora envelope` verb.

## SDK Surface

Placeholder (Tier A). The SDK is the primary way agents exchange envelopes programmatically; CLI sending is mostly a debugging and smoke-test convenience.

## Failure Modes

Placeholder (Tier A).

## Acceptance Criteria

Placeholder (Tier A).

## Out Of Scope For First Slice

- richer envelope extensions beyond the base header/payload split — deferred per [../roadmap/post-mvp.md](../roadmap/post-mvp.md)
- envelope-level encryption beyond what the Layer 1 tunnel provides
- envelope persistence / replay
- cross-session envelope routing
- deeper A2A-specific envelope bridging — A2A payloads ride as opaque in MVP

## Open Questions

- **Wire format.** JSON-for-header + length-prefixed binary payload, protobuf-encoded envelope, or something else? Proposed MVP: length-prefixed frames, each carrying a JSON header and an opaque binary payload. JSON chosen for header inspectability; binary payload preserves opacity. Protobuf is a viable alternative and may be revisited. Needs confirmation.
- **Framing on stream-oriented carriers (`tcp`).** Length-prefixed framing with a 4-byte big-endian header length + 4-byte big-endian payload length, or a more elaborate framing? Proposed MVP: 4+4 length-prefixed framing.
- **Framing on `http` carriers.** Each envelope as an HTTP request/response pair, or WebSocket-style framing over a hijacked connection? Proposed: `tcp` is the recommended carrier for MVP envelope transport; `http` carrier framing is post-MVP.
- **Maximum envelope size.** Hard cap to protect infrastructure. Proposed MVP: 1 MiB total envelope (header + payload); configurable later.
- **Required vs optional headers.** Which of the listed headers are required vs optional? Proposed required: `schema_version`, `envelope_id`, `session_id`, `sender_account_id`, `sender_organization_id`, `message_type`, `timestamp`. Proposed optional: `content_type`, `correlation_id`.
- **Delivery semantics.** Layer 1 tunnel provides reliability appropriate to its mode. Is Layer 2 adding any envelope-level delivery guarantee (retry, de-dup)? Proposed MVP: no Layer 2 delivery guarantee — the Layer 1 tunnel is authoritative. Sender and receiver tolerate what the tunnel mode provides.
- **Ordering semantics.** Strict FIFO within a session (stream-oriented), or unordered? Proposed MVP: FIFO for `tcp`-backed sessions; `http`- and `udp`-backed sessions inherit their mode's semantics.
- **Infrastructure inspection model.** Does the controller see every envelope header in real time (for enforcement), or only aggregate counts and post-hoc audit events? Proposed MVP: envelope counts and enforcement decisions are reported by the provider's local runtime via existing heartbeat-style controller calls; the controller does not sit inline on every envelope. Needs confirmation.
- **Sender authentication.** Is `sender_account_id` cryptographically asserted (signed envelope) or trusted from the tunnel's Layer 0 identity binding? Proposed MVP: trusted from the Layer 0 tunnel identity binding. Envelope signing is post-MVP.
- **Correlation ID scope.** Correlation IDs are sender-assigned — are they globally unique, per-session unique, or just meaningful to the sender? Proposed: meaningful to the sender (opaque to infrastructure); no global uniqueness requirement.
