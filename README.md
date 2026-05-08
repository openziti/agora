# Agora

Agora is a native zero-trust overlay network for agent-to-agent communication.

It is built on OpenZiti and provides the identity, discovery, policy, and
communication substrate that autonomous agents need to interact safely across
organizational boundaries. Agora is A2A-compatible at the protocol layer, but
its core value is the governed network underneath: secure connectivity, explicit
policy boundaries, and auditable collaboration primitives.

Agora is pre-1.0 software. The project is active and usable for local
development and demos, but APIs and operational details may still change.

The canonical architecture, status, and roadmap materials live under
[docs/](./docs/README.md).

## Architecture

Agora is organized in layers:

- Layer 0 (Fabric): OpenZiti provides cryptographic identity, mutual
  authentication, end-to-end encryption, and dark-by-default connectivity.
- Layer 1 (Network): Agora connectivity primitives including organizations,
  accounts, environments, tunnels, tunnel grants, and the local network runtime.
- Layer 2 (Collaboration): governed agent collaboration services including
  workgroups, catalog discovery, advertisements, sessions, contracts, and
  envelopes.

See [docs/architecture/overview.md](./docs/architecture/overview.md) for the
cross-layer architecture.

## Current Status

The repository currently includes:

- a Cobra-based `agora` CLI
- a PostgreSQL persistence layer built with `sqlx`
- embedded SQL migrations managed with `rubenv/sql-migrate`
- an OpenAPI 3.x contract under `internal/api/specs`
- generated controller client/server bindings using `ogen`
- a handwritten controller service implementing the generated API interfaces
- real OpenZiti-backed environment and tunnel lifecycle flows
- a local `agora network` runtime over gRPC and Unix domain sockets
- Layer 1 tunnel serve/connect flows for `http`, `tcp`, and `udp`
- Layer 2 workgroups, catalog, advertisements, sessions, contracts, and
  envelope transport
- a browser dashboard under `ui/`
- the Macro Pulse reference demo under `examples/macro-pulse/`

Layer 1 is minimum-working and Layer 2 is MVP-complete. Remaining work is
mostly operational hardening, SDK packaging, metrics, limits, and post-MVP
collaboration extensions. For details, see:

- [docs/layer-1/status.md](./docs/layer-1/status.md)
- [docs/layer-2/status.md](./docs/layer-2/status.md)
- [docs/roadmap/post-mvp.md](./docs/roadmap/post-mvp.md)

## Repository Layout

- `cmd/agora/`: CLI entry point and Cobra command wiring
- `cmd/demo-bootstrap/`: dashboard demo topology seeding
- `environment/`: local `~/.agora` environment root model
- `sdk/agent/`: public SDK for embedded agents and the local network runtime
- `internal/api/`: generated `ogen` code and modular OpenAPI specs
- `internal/controller/`: handwritten controller service, auth, and HTTP wiring
- `internal/fabric/openziti/automation/`: OpenZiti automation helpers
- `internal/network/daemon/`: CLI client helpers for the local network runtime
- `internal/network/tunnelruntime/`: HTTP/TCP/UDP tunnel runtime engine
- `internal/persistence/`: PostgreSQL store, repositories, migrations, and tests
- `ui/`: Vite/React dashboard
- `examples/macro-pulse/`: end-to-end multi-agent reference demo
- `docs/`: architecture, layer specs, status, examples, roadmap, and maintainer docs
- `AGENTS.md`: project conventions for coding agents and contributors

Layer-owned internal packages follow the conceptual layer names:

- `internal/fabric/...` for Layer 0 implementation code
- `internal/network/...` for Layer 1 implementation code that is not part of the SDK
- `internal/collaboration/...` for future Layer 2 package-owned implementation code

Cross-cutting packages such as `internal/controller`, `internal/persistence`,
`internal/api`, and `internal/clioutput` intentionally remain top-level.

## Quick Start

Build everything:

```bash
go build ./...
```

Run the Go tests:

```bash
go test ./...
```

Some persistence and controller integration tests use PostgreSQL containers via
`testcontainers-go`, so the full test suite requires Docker access.

Build the dashboard:

```bash
cd ui
npm ci
npm run build
```

Run the local dashboard demo:

```bash
./bin/demo-up.sh
```

The demo script builds the dashboard, installs the Go demo binaries, runs store
migrations, starts the Agora controller, provisions the demo topology, and starts
the Macro Pulse workers. It expects external PostgreSQL and OpenZiti services
matching [etc/demo-controller.yaml](./etc/demo-controller.yaml).

When the script finishes, open the printed URL and log in with the printed demo
credentials. Stop managed demo processes with:

```bash
./bin/demo-down.sh
```

For a clean demo root:

```bash
./bin/demo-down.sh --purge
```

Demo operation details live in:

- [docs/dashboard/walkthrough.md](./docs/dashboard/walkthrough.md)
- [docs/dashboard/troubleshooting.md](./docs/dashboard/troubleshooting.md)
- [examples/macro-pulse/README.md](./examples/macro-pulse/README.md)

## Development

### Prerequisites

- Go 1.25+
- Node.js and npm for dashboard work
- PostgreSQL for controller development
- Docker for integration tests that use `testcontainers-go`
- an OpenZiti controller and enrolled edge router for real tunnel/demo flows

### Project Conventions

- Logging uses `github.com/michaelquigley/df/dl`.
- Structured config and handwritten JSON/YAML binding use
  `github.com/michaelquigley/df/dd`.
- The OpenAPI specification is the source of truth for the controller API.
- Generated `ogen` and protobuf code should be regenerated, not edited by hand.
- PostgreSQL is the only supported database.

### Generate API Code

```bash
./bin/generate_rest.sh
```

This regenerates the committed `ogen` client/server package from
`internal/api/specs/agora.yml`.

### Generate Protobuf Code

```bash
./bin/generate_pb.sh
```

This regenerates the committed protobuf/gRPC stubs used by the local
`agora network` runtime API.

### Run the Controller

The controller expects a YAML config file. Start from
[etc/agora-controller.yaml](./etc/agora-controller.yaml) and set:

- `bind_address`
- `admin_tokens`
- `open_ziti.api_endpoint`
- `open_ziti.auth`
- `store.dsn`

Start the controller with:

```bash
go run ./cmd/agora controller ./etc/agora-controller.yaml
```

### Configure the Local CLI Environment

Agora keeps local CLI metadata under `~/.agora`.

Set the controller API endpoint once:

```bash
go run ./cmd/agora config set api_endpoint http://127.0.0.1:8080
```

Inspect or clear it:

```bash
go run ./cmd/agora config get api_endpoint
go run ./cmd/agora config unset api_endpoint
```

### Manage Migrations

```bash
go run ./cmd/agora store migrate ./etc/agora-controller.yaml up
go run ./cmd/agora store migrate ./etc/agora-controller.yaml down --down 1
go run ./cmd/agora store migrate ./etc/agora-controller.yaml status
go run ./cmd/agora store check-schema ./etc/agora-controller.yaml
```

### Admin API Commands

Admin API commands use the local environment endpoint from
`~/.agora/config.json` or `AGORA_API_ENDPOINT`, and authenticate with
`AGORA_ADMIN_TOKEN`.

```bash
export AGORA_ADMIN_TOKEN=replace-me

go run ./cmd/agora admin create organization acme
go run ./cmd/agora admin create user <organizationId> alice@example.com test-password
go run ./cmd/agora admin list organizations
go run ./cmd/agora admin list users
go run ./cmd/agora admin list users --organization-id <organizationId>
go run ./cmd/agora admin list users --show-ids
go run ./cmd/agora admin list users --json
go run ./cmd/agora admin delete user <organizationId> <accountId>
go run ./cmd/agora admin delete organization <organizationId>
```

Human-readable list output uses a rounded table. Pass `--json` for indented raw
resource objects.

## Documentation

Start with:

- [docs/README.md](./docs/README.md)
- [docs/architecture/overview.md](./docs/architecture/overview.md)
- [docs/sdk/overview.md](./docs/sdk/overview.md)
- [docs/maintainers/current-state.md](./docs/maintainers/current-state.md)

## License

Agora is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
