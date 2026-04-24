# Layer 2 Foundation

This document records the cross-cutting design decisions that every Layer 2 (Collaboration) concept depends on. It exists so the per-concept specs — `workgroups.md`, `catalog.md`, `advertisements.md`, `sessions.md`, `contracts.md`, `envelopes.md` — can reference shared decisions rather than each relitigating the same questions.

Everything in this document is normative for Layer 2 implementation unless explicitly flagged as an open question.

## ID And Naming Scheme

Layer 2 resource IDs follow the existing Layer 1 format: `<prefix>_<12-char lowercase alphanumeric>`, e.g. `wg_a1b2c3d4e5f6`. The `[a-z0-9]{12}` body matches the controller-side regex already enforced on Layer 1 resources.

The reserved Layer 2 prefixes are:

- `wg_` — workgroup
- `wgi_` — workgroup invitation (per-invited-org record on an inter-org workgroup)
- `wgm_` — workgroup membership record
- `adv_` — advertisement
- `ses_` — session
- `con_` — contract
- `evp_` — envelope (for correlation / audit; envelopes are not expected to be first-class addressable REST resources in MVP)

IDs are opaque to clients. Clients must not assume any structure beyond prefix-plus-body.

## OpenAPI Module Organization

Layer 2 follows the existing per-domain modular structure under `internal/api/specs/`. Each concept with its own resource surface gets its own module:

- `internal/api/specs/workgroups/`
- `internal/api/specs/catalog/`
- `internal/api/specs/advertisements/`
- `internal/api/specs/sessions/`
- `internal/api/specs/contracts/`

Envelopes do not get a dedicated module in MVP because envelope transport is handled inside session-backed tunnels rather than as a distinct REST resource surface.

Shared schemas continue to live under `internal/api/specs/common/`.

## Authorization Composition

Layer 2 does not introduce new authentication credentials. The existing Layer 1 authentication surfaces remain authoritative:

- account token (via `X-TOKEN` header, handled by `HandleAccountTokenAuth` in `internal/controller/service.go`) for account-scoped operations
- admin token (via the existing admin auth path) for platform administration

What Layer 2 adds is a workgroup-scoped **resolution layer** on top of the authenticated principal:

1. Authentication resolves the request to an account, which carries an organization.
2. For any Layer 2 operation, a shared helper resolves the account's workgroup memberships.
3. Visibility and access decisions filter resources by whether the authenticated account shares a workgroup with the resource's workgroup scope.
4. Inter-organization collaboration works through shared inter-org workgroups: an account in organization A may see and interact with a resource scoped to an inter-org workgroup shared between A and B, even when the resource's owning account is in organization B.

The resolution helper lives in `internal/collaboration/` and is shared by every Layer 2 handler. Per-concept specs reference this decision rather than redefining it.

Organization-level administrative Layer 2 operations — creating a workgroup, acknowledging an inter-org invitation — use the existing platform admin token (`X-ADMIN-TOKEN`) rather than a new role on the account model. Agora's deployment model is a single self-hosted controller, and the platform admin is the effective operator across all organizations in that controller; reusing it for org-scoped Layer 2 bootstrap keeps the authorization surface small.

The only new per-account role Layer 2 introduces is **workgroup admin**, expressed as a `WorkgroupMembership` with `role = admin`. Workgroup admin is per-workgroup, not per-organization. Holders do not need platform admin privilege and can perform routine workgroup administration (adding, removing, and re-roling members of their own organization) under the normal account-token surface.

An account-level `org-admin` role is **not** introduced in MVP. Introducing one is reasonable future work if self-service workgroup creation within an organization becomes a requirement (the platform admin token is a heavier credential than an end user should need for routine workgroup creation). That is deferred until concrete demand emerges.

## Code Layout

Layer 2 follows the existing repo-wide convention: controller handlers stay in `internal/controller/`, Layer-owned implementation code goes under the layer-specific namespace.

- `internal/controller/` — handwritten HTTP handlers implementing the generated `ogen` interfaces, one file per operation (matching the Layer 1 pattern, e.g. `createTunnel.go`). Layer 2 adds files like `createWorkgroup.go`, `listAdvertisements.go`, etc.
- `internal/collaboration/` — Layer 2 domain logic not tied to a single HTTP handler. Sub-packages per concept as they accrete meaningful logic:
  - `internal/collaboration/workgroup/`
  - `internal/collaboration/catalog/`
  - `internal/collaboration/advertisement/`
  - `internal/collaboration/session/`
  - `internal/collaboration/contract/`
  - `internal/collaboration/envelope/`
- `internal/persistence/` — new repositories for Layer 2 resources (e.g. `WorkgroupsRepository`, `AdvertisementsRepository`) alongside the existing Layer 1 repositories. New migrations added under `internal/persistence/migrations/` following the existing ordered-filename convention.

Agent-side (local runtime) Layer 2 code, where it exists, lives under `internal/collaboration/`, not under `sdk/agent/`. The Layer 1 runtime is delivered in two forms (see [../layer-1/agent.md](../layer-1/agent.md) "Packaging Direction"): a standalone daemon (`agora network start`) for CLI-driven operator workflows, and an embeddable library at `sdk/agent` that Layer 2 agents import directly in-process. Layer 2 agents use the embedded form; Layer 2 runtime logic does not cross the process boundary of the agent it serves.

## CLI Command Shape

Layer 2 extends the existing domain-prefixed command tree. The user-facing top-level verbs are:

- `agora workgroup ...` — manage workgroups the account owns or administers (`create`, `list`, `describe`, `delete`, `member add/remove`)
- `agora advertise ...` — manage the account's own advertisements (`publish`, `update`, `retract`, `list`, `describe`)
- `agora catalog ...` — discovery (`search`, `describe`)
- `agora session ...` — session lifecycle (`propose`, `accept`, `close`, `list`, `describe`, `send` for envelope sending within an active session)

Contracts are attached to advertisements and sessions rather than managed as a standalone resource. Contract inspection nests under `agora session describe` and `agora advertise describe`; `agora contract` as a standalone command is not introduced in MVP.

Envelopes are not a user-visible command surface: envelopes flow through `agora session send` or through SDK APIs, not through a top-level `agora envelope` verb.

Admin-only Layer 2 operations (e.g. creating inter-org workgroups, auditing cross-workgroup activity) nest under `agora admin`, matching the existing pattern:

- `agora admin workgroup ...`
- `agora admin advertisement ...` (if admin-side advertisement inspection is needed)

Consistent with Layer 1, list commands default to the `go-pretty/table` presentation and accept `--json` for machine-readable output emitting raw resource objects.

Layer 2 commands that need runtime involvement (primarily `agora session` subcommands that start or tear down Layer 1 tunnels) delegate to the local `agora network` daemon by default, matching the `agora tunnel serve`/`agora tunnel connect` pattern. `--foreground` remains a debug bypass where applicable.

## Session-To-Tunnel Relationship

**An MVP session is 1:1 with a Layer 1 tunnel.** This is the critical cross-cutting decision for Layer 2 infrastructure.

Concretely:

- Establishing a session provisions a Layer 1 tunnel between the participants as part of the engagement flow. The tunnel is owned by the session's provider account and is scoped to the session's workgroup.
- Envelopes flow through that tunnel. The tunnel mode (`http`, `tcp`, `udp`) is declared by the advertisement; the consumer accepts that mode by proposing a session, or does not propose. There is no per-session mode negotiation. `tcp` is the expected default for MVP envelope transport.
- Closing the session deletes the tunnel. Session closure is the authoritative lifecycle event; tunnel deletion is a consequence.
- A given account may hold multiple concurrent sessions with the same counterparty; each session backs its own tunnel.

Consequences for the per-concept specs:

- `sessions.md` describes session lifecycle in terms of this 1:1 relationship and the envelope-transport expectations that follow from it.
- `envelopes.md` describes envelope wire format assuming a single-session, single-tunnel carrier. Multiplexed transport is explicitly out of MVP scope.
- `contracts.md` describes contract evaluation in terms of bounds on a single session's tunnel (duration, envelope count, etc.).

Multiplexing multiple sessions over a shared tunnel is a post-MVP consideration. It is not blocked by the MVP design — envelope headers include enough identifying information (see `envelopes.md`) that a future multiplexed carrier could be introduced without changing the envelope format — but the MVP commits to 1:1 for simplicity.

## Local State Posture

Layer 2 MVP introduces no new files under `~/.agora`.

- Workgroup membership, advertisement state, active sessions, and contracts are all controller-owned. Clients query the controller rather than caching locally.
- Session-backed tunnels are Layer 1 tunnel resources, and therefore surface in the existing `network.json` managed-serve/managed-connect state when the local agent is hosting provider or consumer roles.
- Any future local Layer 2 caching (advertisement caches, catalog query caches) is explicitly out of MVP scope.

This keeps the local environment root and the existing versioned `env_v0` scheme unchanged through Layer 2 MVP.

## Versioning And Deprecation

Advertisements, contracts, and envelope wire format each carry an explicit schema version field so the Layer 2 surface can evolve without breaking older participants:

- advertisements: `schema_version` integer field on the resource; MVP is `1`
- contracts: `schema_version` integer field; MVP is `1`
- envelopes: schema version in the envelope header (see `envelopes.md`); MVP is `1`

Workgroups, catalog query results, and sessions do not carry explicit schema versions in MVP — their shapes are expected to evolve through additive REST changes, consistent with the rest of the OpenAPI surface.

Deprecation of a schema version, when it eventually happens, is done by introducing the newer version alongside the older one and removing the older one in a later release. No MVP deprecation is planned.

## Observability Conventions

Layer 2 logging uses the same conventions as Layer 1:

- structured logs via `github.com/michaelquigley/df/dl`
- dynamic values surrounded by single quotes in log messages
- structured fields on significant events (e.g. `workgroup_id='...'`, `session_id='...'`, `advertisement_id='...'`, `envelope_id='...'`)
- provider-side and consumer-side request-level logging on session establishment, envelope send/receive, and contract evaluation decisions, mirroring the tunnel-level request logging already present in Layer 1

Metrics remain deferred to post-MVP work per [../roadmap/post-mvp.md](../roadmap/post-mvp.md).

## Explicit MVP Boundary

Layer 2 MVP intentionally excludes:

- metrics
- limits
- semantic catalog search
- programmatic contracts
- richer envelope extensions beyond the base header/payload split
- deeper A2A protocol bridging
- workgroup hierarchy
- memory-oriented collaboration services built on top of the base primitives
- session multiplexing over a shared tunnel

These items remain part of the broader architecture and are tracked in [../roadmap/post-mvp.md](../roadmap/post-mvp.md). Per-concept specs defer to this boundary rather than enumerating it again.
