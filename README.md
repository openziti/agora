# Agora

Agora is a native zero-trust overlay network for agent-to-agent communication.

It is built on OpenZiti and is intended to provide the identity, discovery, policy, and communication substrate that autonomous agents need to interact safely across organizational boundaries. Agora is A2A-compatible at the protocol layer, but its core value is the governed network underneath: secure connectivity, explicit policy boundaries, and auditable communication primitives.

The architecture is defined in [CLAUDE-ARCHITECTURE.md](./CLAUDE-ARCHITECTURE.md).

## Architecture

Agora is organized in layers:

- Layer 0 (Fabric): the OpenZiti fabric provides cryptographic identity, mutual authentication, end-to-end encryption, and dark-by-default connectivity
- Layer 1 (Network): Agora connectivity primitives including organizations, accounts, environments, and tunnels
- Layer 2 (Collaboration): agent collaboration services including workgroups, catalog/discovery, advertisements, sessions, contracts, and envelopes

This repository is currently focused on the Layer 1 (Network) foundation plus the local runtime and controller surfaces that higher layers will build on.

## Current Status

The codebase is still early-stage, but it already includes the main structural pieces for a working Layer 1 (Network) slice:

- a Cobra-based `agora` CLI
- a PostgreSQL-only persistence layer built with `sqlx`
- embedded SQL migrations managed with `rubenv/sql-migrate`
- an OpenAPI 3.0.3 specification under `internal/api/specs`
- generated server and client bindings using `ogen`
- a handwritten controller service that implements the generated `ogen` interfaces
- real OpenZiti-backed environment and tunnel lifecycle flows
- a phase 2 local `agora network` agent over gRPC+UDS with environment heartbeat and local heartbeat status

The current API surface covers the first Layer 1 (Network) slice:

- organization administration
- account creation and token-based authentication
- environment lifecycle
- tunnel lifecycle

## Repository Layout

- `cmd/agora/`: CLI entrypoints and Cobra command wiring
- `internal/api/`: generated `ogen` code and the modular OpenAPI spec
- `internal/controller/`: handwritten controller service, auth logic, and HTTP server wiring
- `internal/fabric/openziti/automation/`: Layer 0 (Fabric) OpenZiti automation primitives
- `internal/network/agent/`: local `agora network` agent, client, and protobuf service implementation
- `internal/network/tunnelruntime/`: HTTP/TCP/UDP tunnel runtime implementations used by `serve` and `connect`
- `internal/persistence/`: store, repositories, models, migration management, and integration tests
- `internal/collaboration/`: reserved for future Layer 2 (Collaboration) package-owned code
- `CLAUDE-ARCHITECTURE.md`: high-level architecture and product direction
- `AGENTS.md`: contributor rules and project conventions

Layer-owned internal packages follow the conceptual layer names:

- `internal/fabric/...` for Layer 0 (Fabric)
- `internal/network/...` for Layer 1 (Network)
- `internal/collaboration/...` for Layer 2 (Collaboration)

Cross-cutting packages such as `internal/controller`, `internal/persistence`, `internal/api`, and `internal/clioutput` intentionally remain top-level.

## Development

### Prerequisites

- Go 1.25+
- PostgreSQL
- Docker, if you want to run the persistence and controller integration tests that use `testcontainers-go`

### Project Conventions

- Logging is handled through `github.com/michaelquigley/df/dl`
- Structured config and handwritten JSON/YAML binding/unbinding are handled through `github.com/michaelquigley/df/dd`

### Generate API Code

```bash
./bin/generate_rest.sh
```

This regenerates the `ogen` client/server package from `internal/api/specs/agora.yml`.

### Generate Protobuf Code

```bash
./bin/generate_pb.sh
```

This regenerates the committed protobuf/gRPC stubs used by the local `agora network` agent API.

### Run Tests

```bash
go test ./...
```

### Run the Controller

The controller expects a YAML config file. At minimum, configure a bind address, one or more admin tokens, and a PostgreSQL DSN.

Templates:

- `etc/agora-controller.yaml`: documented baseline template for real deployments
- `etc/dev-agora-controller.yaml`: local development stub with placeholder/dev-friendly values

Minimal example:

```yaml
bind_address: ":8080"
admin_tokens:
  - "replace-me"
open_ziti:
  api_endpoint: "https://controller.example"
  auth:
    mode: "updb"
    updb:
      username: "admin"
      password: "replace-me"
store:
  dsn: "postgres://user:password@localhost:5432/agora?sslmode=disable"
  max_open_conns: 4
  max_idle_conns: 4
```

Start the controller with:

```bash
go run ./cmd/agora controller ./agora.yml
```

### Configure the Local CLI Environment

Agora keeps local CLI metadata under `~/.agora`, following the same general pattern as `zrok`.

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
go run ./cmd/agora admin store migrate ./etc/dev-agora-controller.yaml up
go run ./cmd/agora admin store migrate ./etc/dev-agora-controller.yaml down --down 1
go run ./cmd/agora admin store migrate ./etc/dev-agora-controller.yaml status
go run ./cmd/agora admin store check-schema ./etc/dev-agora-controller.yaml
```

### Admin API Commands

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

These commands use the local environment endpoint from `~/.agora/config.json` or `AGORA_API_ENDPOINT`, and authenticate with `AGORA_ADMIN_TOKEN`.
Human-readable list output uses a zrok-style rounded table. Pass `--json` for indented raw resource objects instead.

## Design Notes

- PostgreSQL is the only supported database
- the OpenAPI specification is the source of truth for the controller API
- generated `ogen` code should be regenerated, not edited by hand
- Layer 1 (Network) should remain useful on its own for secure service connectivity, even before the full Layer 2 (Collaboration) model is implemented
