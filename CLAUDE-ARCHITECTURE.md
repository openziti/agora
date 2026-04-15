# Agora

### Architectural Specification

*March 2026 — v0.1*

---

## 1. Overview

Agora is a zero-trust network purpose-built for agent-to-agent communication. It provides the identity, discovery, policy, and communication fabric that allows autonomous AI agents to find each other, negotiate terms of engagement, and interact within governed, auditable sessions.

Agora is built on the OpenZiti zero-trust overlay network, extending a decade of production-grade networking infrastructure into the agentic AI era. It is A2A-compatible — agents on the Agora network speak the Google A2A protocol — but the network they communicate over, the policy that governs who they communicate with, and the contracts that define the terms of interaction are what Agora provides.

Agora serves two distinct but complementary use cases:

**Secure connectivity.** Operators deploy services (llm-gateway, mcp-gateway, databases, custom applications) on the Agora network and make them reachable through named tunnels, scoped to workgroups. No agent concepts are required. This is the Layer 1 use case.

**Governed agent collaboration.** AI agents discover each other's capabilities through a policy-gated catalog, negotiate engagement contracts, and interact within governed sessions. This is the Layer 2 use case, built on top of the same Layer 1 primitives.

---

## 2. Layered Architecture

Agora's architecture is organized in three layers. Each layer builds on the one below it. Layer 1 is useful without Layer 2. Layer 2 requires Layer 1.

### Layer 0 — OpenZiti Fabric

Inherited, not built. The OpenZiti overlay network provides:

- Cryptographic identity for every participant
- Mutual authentication on every connection
- End-to-end encryption
- Software-defined perimeters (dark by default — no exposed endpoints)
- Smart routing across the overlay

Agora does not modify or extend Layer 0. It consumes it as a foundation.

### Layer 1 — Connectivity Primitives

The zrok-spiritual layer. Provides the operational primitives for getting on the network and creating secure connectivity:

- **Organizations** — multi-tenant boundaries
- **Accounts** — user identities within organizations
- **Environments** — provisioned network presence (identity + connectivity)
- **Tunnels** — named, private, bidirectional connectivity with TCP-like semantics
- **Metrics** — time-series observability built into the controller
- **Limits** — resource governance per account

Layer 1 is sufficient for the secure connectivity use case. An operator deploying llm-gateway uses Layer 0 + Layer 1 only.

### Layer 2 — Agent Services

The agent collaboration layer. Built entirely on Layer 1 primitives:

- **Workgroups** — policy boundaries controlling visibility and interaction scope
- **Catalog** — intrinsic discovery service with policy-gated visibility
- **Advertisements** — capability declarations published to the catalog
- **Sessions** — governed communication channels between agents (tunnels with contract metadata)
- **Contracts** — negotiated terms governing session behavior
- **Envelopes** — structured message format for session communication

A session is, at the infrastructure level, a tunnel with governance metadata. An advertisement is a catalog entry that declares a tunnel endpoint is available under certain terms.

---

## 3. Core Concepts

### 3.1 Organization

The top-level tenant boundary. An organization owns accounts, agents, environments, tunnels, and advertisements. Organizations are isolated from each other by default — Org A cannot see or interact with Org B's resources.

Cross-organization collaboration happens through shared workgroups (see §3.6).

A single Agora controller serves multiple organizations. This multi-tenancy is a core architectural requirement, not a later enhancement, because workgroups that span organizational boundaries require the organizations to be on the same network talking to the same controller.

### 3.2 Account

A user identity within an organization. Accounts are created by organization administrators. An account authenticates against the controller and can perform operations scoped to its organization's permissions.

### 3.3 Environment

A provisioned network presence. Enabling an environment provisions an identity on the OpenZiti fabric and establishes connectivity to the controller. An environment is the prerequisite for creating tunnels, publishing advertisements, or participating in sessions.

Environments are created by running `agora enable` (CLI) or calling the equivalent SDK method. Disabling an environment tears down its network presence.

### 3.4 Tunnel

A named, private, bidirectional communication channel with TCP-like semantics: reliable, ordered delivery. Tunnels are the fundamental connectivity primitive in Agora.

Tunnels are created with `agora create tunnel <name> --backend <address>` (CLI) or the equivalent SDK call. They are persistent and named — they survive agent restarts and are re-established by the agent library on startup.

Tunnels can be scoped to workgroups, controlling which identities on the network can connect to them. A tunnel not assigned to any workgroup is accessible only to its owning environment.

Under the hood, sessions (Layer 2) are tunnels with additional governance metadata. The session establishment flow creates a tunnel and attaches a resolved contract to it.

### 3.5 Identity Model

Every participant on the Agora network — whether an AI agent, an operator-deployed service, or a human using the CLI — receives a cryptographic identity through enrollment on the OpenZiti fabric. Identity is not a token or an API key; it is a certificate-based proof bound to the participant's lifecycle on the network.

The identity model does not formally distinguish between "agents" and "services." An identity is an identity. Whether it publishes advertisements and engages in sessions is a matter of what the software does, not what the controller enforces. An llm-gateway deployed by an operator and an AI agent performing code review both have identities on the network. The llm-gateway simply doesn't use the catalog.

### 3.6 Workgroup

A policy boundary that controls visibility and interaction scope. Workgroups serve as both an **isolation mechanism** and a **collaboration mechanism**.

**Intra-org workgroups** are created by an organization administrator and populated with that organization's agents and services. They scope catalog visibility — an agent only sees advertisements published within workgroups it belongs to.

**Inter-org workgroups** (shared workgroups) span organizational boundaries. They are created through an invitation model:

1. Org A creates a workgroup and invites Org B
2. Org B's administrator accepts the invitation
3. Both organizations can enroll their agents in the shared workgroup
4. Each organization manages its own agents' membership; neither can manage the other's

Shared workgroups enable the cross-organization collaboration use case — agents from different organizations discovering each other, negotiating contracts, and interacting within governed sessions.

Workgroups are flat. There is no hierarchy. If hierarchy is needed in the future, it can be composed (a workgroup whose policy references other workgroups) without changing the core model.

### 3.7 Catalog

An intrinsic network service built into the controller, always available to every connected agent. The catalog serves as both a **directory** (deterministic lookup by name or identifier) and a **discovery engine** (filtered search by capability, workgroup, and other attributes).

The catalog holds **advertisements** (see §3.8) and enforces **visibility policy at the discovery layer**. When an agent queries the catalog, it only receives results it is authorized to see based on its identity and workgroup memberships. If an agent is not in the workgroup, the advertisement does not exist as far as that agent is concerned. This is zero-trust applied to metadata — unauthorized agents don't discover that capabilities exist, not just that they can't access them.

For the initial implementation, the catalog is a filtered query over advertisements stored in PostgreSQL. Discovery queries support filtering by capability type, name pattern, and workgroup scope. Semantic search (embedding-based similarity matching) is a later enhancement that could leverage pgvector in the same PostgreSQL instance.

Tunnels (Layer 1 resources) can also be listed in the catalog as a degenerate form of advertisement — an advertisement with no contract requirements, representing infrastructure that is simply available for connection within its workgroup scope. This keeps the catalog as a unified discovery mechanism for both agent capabilities and infrastructure services.

### 3.8 Advertisement

An agent's declaration of what it can do and how to engage with it. An advertisement is published to the catalog and contains:

- **Name** — human-readable identifier
- **Description** — what the capability does
- **Capabilities** — typed strings describing what is offered (e.g., "code-review", "security-analysis", "translation")
- **Interaction patterns** — what communication patterns the agent supports (request-response, streaming, long-running)
- **Visibility scope** — which workgroups can discover this advertisement
- **Contract requirements** — what an engaging agent must satisfy to establish a session

For A2A compatibility, an advertisement can carry an A2A Agent Card as an embedded field, allowing Agora agents to expose A2A-aligned metadata through the catalog.

Advertisements are **persistent**. Like tunnels, they survive agent restarts and are re-associated with the agent's identity when it reconnects to the network. An agent that publishes an advertisement and then restarts does not need to re-publish — the advertisement remains in the catalog and becomes active again when the agent's environment is re-enabled. Withdrawal of an advertisement is an explicit action (`agora withdraw`), not a side effect of disconnection.

### 3.9 Session

A private, policy-governed communication channel between agents. A session is established through the engagement flow:

1. Agent A discovers Agent B's advertisement through the catalog
2. Agent A requests engagement
3. Both sides evaluate the contract (see §3.10)
4. If the contract resolves successfully, a session is established

Under the hood, a session is a tunnel (Layer 1) with a resolved contract attached as enforceable metadata. The session has a managed lifecycle: established, active, suspended, closed. Either party or the policy framework can close a session. Contract terms (time limits, envelope counts, interaction constraints) are enforced by the infrastructure.

### 3.10 Contract

The negotiated terms governing a session. A contract defines what is permitted within the session and is resolved during the engagement phase before the session becomes active.

For the initial implementation, contracts are **declarative** — a JSON document with typed fields:

- **Max session duration** — how long the session can remain active
- **Max envelope count** — how many envelopes can be exchanged
- **Allowed message types** — which envelope message types are permitted
- **Required workgroup memberships** — what workgroups the engaging agent must belong to
- **Required maturity level** — minimum ATF maturity level (Intern, Junior, Senior, Principal) for the engaging agent
- **Access mode** — `open` (any workgroup member can engage), `approval-required` (owner must approve), or `policy` (evaluate a policy document)

The controller evaluates contracts by checking the declared fields against the engaging agent's identity and attributes. This is deliberately simple and limited — shippable, testable, and understandable.

**Maturity level enforcement.** ATF maturity levels map to concrete Agora capability restrictions:

- **Intern** — Read-only workgroup membership. Can discover advertisements through the catalog but cannot initiate engagement. Useful for observational agents and monitoring.
- **Junior** — Can initiate engagement, but contracts require human approval gates before sessions become active. Session contracts include strict rate limits and scope restrictions.
- **Senior** — Full engagement capability within authorized workgroups. Contracts permit autonomous operation within defined boundaries. Post-action notification implemented through envelope metadata and gateway instrumentation.
- **Principal** — Self-directed engagement across multiple workgroups. Contracts define broad operational boundaries with exception-based escalation. The agent's maturity level is an identity attribute visible to other agents during engagement negotiation, enabling maturity-aware collaboration policies.

Maturity levels can be encoded as identity attributes on the Agora network. Promotion (granting higher autonomy) and demotion (restricting capabilities after incidents) are operator actions on the controller, enforceable immediately without the agent's cooperation.

The evolution path is toward **programmatic contracts** (policy-as-code with custom evaluation functions), but this should be deferred until real usage patterns reveal what declarative contracts cannot express.

### 3.11 Envelope

The structured message format for communication within sessions. Every message exchanged between agents in a session is wrapped in an envelope.

An envelope has two parts:

**Header** — inspectable by the infrastructure:

- Sender identity (cryptographic)
- Recipient identity (cryptographic)
- Session ID
- Sequence number
- Timestamp
- Message type

**Payload** — opaque to the infrastructure. Contains whatever the agents want to exchange, including A2A task messages.

The header is what makes the network governable. The infrastructure can enforce session contracts (count envelopes against limits, check time bounds), attribute every message to a cryptographic identity (enabling audit trails), and observe interaction patterns (feeding behavioral monitoring) — all without understanding the payload.

The initial envelope specification should be minimal — just the header fields listed above, describable in a single OpenAPI schema. Richer fields (provenance chains, intent declarations, cost attribution, policy constraints) should be added as optional header extensions once real usage patterns emerge.

The key distinction from raw tunnel traffic: **tunnels carry bytes, sessions carry envelopes.** A tunnel between an operator's llm-gateway and an agent is TCP-like connectivity — no envelopes involved. A session between two agents structures communication as envelopes, giving the infrastructure the hooks it needs for governance, attribution, and observability.

---

## 4. Controller Architecture

### 4.1 Centralized Controller

The Agora controller is a central service that manages all state, policy, and coordination. It is the source of truth for:

- Organizations, accounts, and identity enrollment
- Environment lifecycle
- Tunnel state and routing
- Workgroup membership and policy
- The catalog (advertisements and visibility)
- Session lifecycle and contract enforcement
- Metrics ingestion and query
- Limits enforcement

The controller exposes a REST API defined by an OpenAPI 3.x specification. Both the CLI and the SDK interact with the controller through this API.

The "decentralized" property of Agora refers to **administration at the edge**, not distribution of the control plane. An agent anywhere on the network can authenticate against the controller and perform operations — enable environments, create tunnels, publish advertisements, establish sessions. The controller is central; the operations are distributed. This is the same model as zrok.

### 4.2 Data Substrate

The controller uses **PostgreSQL** as its sole data substrate. There is no secondary database.

Relational data (organizations, accounts, environments, tunnels, workgroups, advertisements, sessions, contracts) lives in standard PostgreSQL tables.

Time-series data (metrics) lives in the same PostgreSQL instance using the **TimescaleDB** extension. TimescaleDB provides:

- **Hypertables** — automatic time-based partitioning for metric samples
- **Continuous aggregates** — incrementally-refreshed materialized views for rollup queries
- **Retention policies** — automatic expiration of raw samples
- **Compression** — 90%+ storage reduction for historical data

This eliminates the need for a separate time-series database (as zrok uses InfluxDB), reducing deployment complexity to a single database instance.

**Licensing note.** TimescaleDB's core hypertable functionality is Apache 2.0. Compression, continuous aggregates, and retention policies are under the Timescale License (TSL), which is permissive for self-hosted use but restricts competing managed database services. For Agora's use case (consuming TimescaleDB, not reselling it), the TSL is effectively permissive. As a risk mitigation, the metrics layer should be designed with a clean abstraction boundary so that it could fall back to pg_partman + pg_cron on vanilla PostgreSQL if the licensing situation ever required it. This fallback would require application-level rollup logic instead of continuous aggregates, but would not require architectural changes.

SQLite is not supported. Standardizing on PostgreSQL-only simplifies development, testing, and deployment.

### 4.3 API Framework

The controller API is defined as an **OpenAPI 3.x specification**. Server and client code is generated using **ogen** (github.com/ogen-go/ogen), which provides:

- Statically typed server handler interfaces and client methods
- Code-generated JSON encoding/decoding (no reflection)
- Generated Optional/Nullable wrappers (no pointer-for-everything)
- Code-generated validation from OpenAPI schema constraints
- Sum type generation for `oneOf` schemas (useful for the envelope model)
- Operation groups for organizing the API surface
- Built-in OpenTelemetry tracing and metrics

The OpenAPI specification is the single source of truth for the API contract.

### 4.4 Deployment Model

The controller is a single deployable binary (plus PostgreSQL). Deployment targets:

- **Docker** — `timescale/timescaledb-ha` image for PostgreSQL with TimescaleDB pre-installed. Controller binary as a separate container or in the same compose stack.
- **AWS RDS** — Standard RDS PostgreSQL instance with TimescaleDB extension enabled. Controller binary on EC2, ECS, or similar.
- **Self-hosted** — PostgreSQL with TimescaleDB installed, controller binary on any Linux host.
- **Kubernetes** — Helm chart packaging for both controller and PostgreSQL.

The self-hosting model is a core differentiator. Operators must be able to run the entire Agora stack on their own infrastructure with no external dependencies.

---

## 5. Agent Library

### 5.1 Embeddable Design

The Agora agent library is designed to be embedded directly into agent processes, not run as a separate daemon that spawns child processes. An AI agent imports the library, and all network I/O for tunnels, sessions, and controller communication is handled in-process through goroutines and channels.

This is a deliberate departure from the zrok agent model (which launches separate processes for each share/access). The single-process model provides:

- Lower overhead per tunnel (no IPC, no process management)
- Shared memory for session state
- Simpler lifecycle management
- Natural fit for AI agent frameworks that embed libraries

### 5.2 Standalone Mode

The same library can be wrapped in a thin daemon for cases where a persistent process is needed without writing code — for example, an operator running infrastructure services. The standalone mode exposes a local control interface (gRPC over Unix domain socket, consistent with the zrok agent pattern) for CLI-driven management.

### 5.3 SDK Languages

- **Go** — first-class, the library itself
- **Python** — native Python client calling the controller REST API (not a cgo wrapper), because the AI agent ecosystem is primarily Python
- **Node.js** — third priority, if demand warrants

---

## 6. CLI Design

The Agora CLI inherits the spiritual structure of the zrok CLI. Commands are organized by operation domain.

### 6.1 Admin Commands

Controller and organization administration, typically run by operators:

```
agora admin create organization <name>
agora admin create account --org <org> <email>
agora admin create workgroup --org <org> <name>
agora admin invite --workgroup <name> --org <target-org>
agora admin accept-invite --workgroup <name>
agora admin workgroup add-member --workgroup <name> <identity>
agora admin workgroup remove-member --workgroup <name> <identity>
```

### 6.2 Environment Commands

Environment lifecycle, run by agents or operators:

```
agora enable
agora disable
agora status
```

### 6.3 Tunnel Commands

Connectivity management:

```
agora create tunnel <name> --backend <address>
agora delete tunnel <name>
agora list tunnels
```

### 6.4 Agent Commands

Agent-layer operations (Layer 2):

```
agora advertise <name> --capability <type> --workgroup <name>
agora withdraw <advertisement-name>
agora discover --capability <type> --workgroup <name>
agora engage <advertisement-name>
agora sessions
agora close-session <session-id>
```

---

## 7. Metrics

### 7.1 Design

Metrics are a first-class primitive in the Agora controller. Every component on the network — the controller, tunnels, agents, sessions — can emit metrics that are stored, aggregated, and queryable.

### 7.2 Data Model

A metric sample consists of:

- **Metric name** — dot-separated identifier (e.g., `tunnel.bytes_tx`, `session.envelope_count`)
- **Labels** — key-value pairs as a JSON object (e.g., `{"agent": "agent-abc", "workgroup": "devops", "org": "acme"}`)
- **Timestamp** — when the sample was recorded
- **Value** — float64

Labels always include the emitting identity's organization and, where applicable, workgroup affiliation. This enables org-scoped and workgroup-scoped metric queries.

### 7.3 Storage

Metric samples are stored in a TimescaleDB hypertable. Continuous aggregates provide pre-computed rollups at multiple time granularities (1-minute, 1-hour, 1-day). Retention policies automatically expire raw samples after a configurable period while preserving aggregated history.

### 7.4 Ingest API

Push model. Components emit metrics to the controller via a batch endpoint:

```
POST /v1/metrics
Content-Type: application/json

[
  {
    "name": "tunnel.bytes_tx",
    "labels": {"tunnel": "llm-gw", "workgroup": "infra", "org": "acme"},
    "timestamp": "2026-03-25T14:30:00Z",
    "value": 1048576.0
  }
]
```

### 7.5 Query API

```
GET /v1/metrics/query?name=tunnel.bytes_tx&labels=workgroup:infra&from=...&to=...&bucket=1h&aggregate=sum
```

The query API supports filtering by metric name, label values, time range, bucket size, and aggregation function (sum, avg, min, max, count). The API routes to raw samples for recent high-resolution queries and to continuous aggregates for historical queries.

Metric queries are org-scoped — an account can only query metrics for its own organization's resources. Within that scope, queries can be further filtered by workgroup. This is important for shared workgroup scenarios: both organizations participating in a cross-org workgroup need visibility into the collaboration's operational metrics (session counts, envelope throughput, engagement success rates) without seeing each other's internal workgroup metrics. Workgroup-scoped queries return only metrics labeled with the specified workgroup, regardless of which organization's agents emitted them.

### 7.6 Default Metrics

The system emits the following metrics automatically:

**Tunnel metrics:**
- `tunnel.bytes_tx` / `tunnel.bytes_rx` — throughput per tunnel
- `tunnel.active` — active tunnel count per environment
- `tunnel.created` / `tunnel.closed` — lifecycle events

**Session metrics:**
- `session.established` / `session.closed` — lifecycle events
- `session.duration` — session duration at close
- `session.envelope_count` — envelopes exchanged per session

**Catalog metrics:**
- `catalog.advertisement_count` — advertisements per workgroup
- `catalog.discovery_count` — discovery queries
- `catalog.engagement_count` — engagement attempts (offered, accepted, rejected)

**Controller metrics:**
- `controller.api_requests` — request count by endpoint
- `controller.api_latency` — request latency by endpoint

Custom metrics can be emitted by any component using the same ingest API with arbitrary metric names and labels.

---

## 8. Limits

Resource limits are enforced per account by the controller. Limits are configured in the controller's database and checked at resource creation time.

**Layer 1 limits:**
- Environments per account
- Tunnels per environment
- Metric ingest rate (samples per second)

**Layer 2 limits:**
- Advertisements per account
- Active sessions per account

Session-level constraints (max envelopes, max duration) are enforced through the contract mechanism, not the limits system. The limits system governs resource allocation; contracts govern interaction behavior.

---

## 9. A2A Compatibility

Agora's A2A compatibility is **structural, not protocol-bridging**. This means:

- Agora advertisements can carry an A2A Agent Card as an embedded field
- Agora agents that want to be A2A-discoverable publish advertisements that include their Agent Card
- Agora uses A2A-aligned data structures where they map naturally to Agora concepts
- Agora envelopes can carry A2A task messages as opaque payloads

**How A2A flows through Agora.** An agent on the Agora network that wants to use A2A semantics for its interactions wraps A2A JSON-RPC messages (task/send, task/sendSubscribe, etc.) as the opaque payload of Agora envelopes. The Agora infrastructure governs the session — enforcing contracts, counting envelopes, attributing messages to identities — while remaining agnostic to the A2A content inside the payload. The agents interpret the payloads according to A2A conventions. This is the mechanism that makes "A2A-compatible" concrete: A2A is the interaction language between agents, Agora is the secure, governed transport.

An external A2A agent cannot directly interact with the Agora network — that would break the zero-trust model (no exposed endpoints). However, an Agora agent can expose an A2A-compatible endpoint *through* an Agora tunnel for specific use cases where external interoperability is required. In this case, the Agora agent acts as a bridge: it accepts A2A requests from the external world through its tunnel and relays them into Agora sessions with other agents on the network.

Full gateway-style bridging (where the Agora controller translates between Agora sessions and A2A task lifecycles) is a later capability, not part of the initial architecture.

---

## 10. Agentic Trust Framework Alignment

Agora's architecture aligns with the Agentic Trust Framework (ATF) specification across its five core elements.

**Identity** — Strong native coverage. Agora's cryptographic identity model, mutual authentication, and policy-as-code contracts exceed ATF's identity requirements at every maturity level. Agora agents are unreachable unless the network creates a path, providing a stronger guarantee than ATF specifies.

**Behavior** — Moderate coverage. Envelope attribution provides message-level identity tracking. The gateway stack (llm-gateway, mcp-gateway) provides instrumentation points for behavioral monitoring tools. Anomaly detection and explainability are application-layer concerns that integrate through these instrumentation points.

**Data Governance** — Strong coverage. End-to-end encryption is native. Envelope provenance provides data lineage. Session contracts can encode classification controls, retention requirements, and data handling constraints. The gateway stack provides enforcement points for content inspection (PII detection, toxicity filtering).

**Segmentation** — Strongest alignment. Network-layer isolation is foundational to Agora, delivered at all levels — not as a Senior+ add-on as ATF specifies. Workgroups are segmentation boundaries. The catalog enforces visibility at the discovery layer. The engagement model prevents uncontrolled agent-to-agent cascades.

**Incident Response** — Moderate coverage. Network-level kill switch (revoke identity or workgroup membership to immediately isolate an agent). Session revocation is native and immediate. Graceful degradation is supported through contract renegotiation. Automated recovery logic is application-specific.

ATF maturity levels (Intern → Junior → Senior → Principal) can be encoded as identity attributes on the Agora network and referenced in contract requirements, enabling maturity-aware engagement policies.

---

## 11. Composition with Gateway Stack

Agora composes with the existing llm-gateway and mcp-gateway products to create a full agentic infrastructure stack.

**llm-gateway** provides secure LLM inference routing. On the Agora network, llm-gateway is deployed as a service with a named tunnel, scoped to a workgroup. Agents in the workgroup route inference requests through it. The gateway provides an instrumentation point for behavioral monitoring, content filtering, and cost attribution.

**mcp-gateway** provides secure MCP tool access. On the Agora network, mcp-gateway is deployed identically — a service with a named tunnel, scoped to a workgroup. Agents access tools through it, inheriting the same zero-trust security model as all other connectivity.

Both gateways are Layer 1 consumers — they use tunnels and workgroup scoping but do not require advertisements, sessions, or contracts. They are infrastructure services, not agent-layer participants.

**Instrumentation role.** Beyond connectivity, the gateways serve as the primary integration points for ATF Behavior and Data Governance tooling. Behavioral monitoring tools (anomaly detection, intent drift analysis) plug in at the gateway layer to observe inference patterns and tool usage. Content inspection tools (PII detection via Microsoft Presidio or equivalent, toxicity filtering, prompt injection detection) enforce data governance policies at the gateway before requests reach their destinations. The gateways see all traffic that passes through them with full identity context (who is making the request, from which workgroup, within which session), making them the natural enforcement surface for application-layer controls that the network fabric itself does not provide.

---

## 12. Deferred Capabilities

The following capabilities are part of the Agora vision but are explicitly deferred from the initial architecture:

**Agora Memory** — Governed shared memory for agent networks. The concept defines three tiers: **private memory** (agent-scoped, invisible to others on the network), **workgroup memory** (shared context within a collaboration boundary, advertised through the catalog and accessed through sessions), and **exchange memory** (cross-organizational knowledge sharing governed by engagement contracts that enforce classification controls, summarization requirements, temporal constraints, and attribution). Exchange memory composes with the gateway stack — llm-gateway enforces summarization-before-sharing policies, and mcp-gateway provides memory access as MCP tool calls. Memory is a product built on top of Agora primitives, not a primitive itself. Once tunnels, sessions, and the catalog work, a memory service can advertise itself through the catalog and enforce access through session contracts without the controller needing to know about memory specifically.

**Programmatic contracts** — Policy-as-code with embedded scripting or custom evaluation functions. Deferred until real usage patterns reveal what declarative contracts cannot express.

**Semantic catalog search** — Embedding-based similarity matching for capability discovery. The initial catalog uses filtered PostgreSQL queries. Semantic search is a later enhancement using pgvector.

**A2A gateway bridging** — A controller-level bridge that translates between Agora sessions and A2A task lifecycles, allowing non-Agora A2A agents to interact with Agora agents through the controller.

**Workgroup hierarchy** — Composable workgroup structures with parent-child relationships. The initial model is flat. Hierarchy can be composed if real needs arise.

**Envelope extensions** — Provenance chains, intent declarations, cost attribution fields. Added as optional header extensions once usage patterns emerge.

---

## 13. Glossary

| Term | Definition |
|---|---|
| **Advertisement** | A capability declaration published to the catalog by an agent |
| **Catalog** | Intrinsic discovery service built into the controller with policy-gated visibility |
| **Contract** | Negotiated terms governing a session, resolved during engagement |
| **Engagement** | The negotiation phase between discovering an advertisement and establishing a session |
| **Envelope** | Structured message format for session communication (header + opaque payload) |
| **Environment** | A provisioned network presence with identity and connectivity |
| **Organization** | Top-level multi-tenant boundary owning accounts, agents, and resources |
| **Session** | A governed communication channel between agents (tunnel + contract) |
| **Tunnel** | Named, private, bidirectional connectivity with TCP-like semantics |
| **Workgroup** | Policy boundary controlling visibility and interaction scope |

---

*This document captures architectural decisions as of March 2026. It is intended to bootstrap deeper specification work, not to serve as a final implementation reference.*
