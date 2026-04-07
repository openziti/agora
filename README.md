# Agora

Agora is a native zero-trust overlay network for agent-to-agent communication.

It is built on OpenZiti and is intended to provide the identity, discovery, policy, and communication substrate that autonomous agents need to interact safely across organizational boundaries. Agora is A2A-compatible at the protocol layer, but its core value is the governed network underneath: secure connectivity, explicit policy boundaries, and auditable communication primitives.

The architecture is defined in [CLAUDE-ARCHITECTURE.md](./CLAUDE-ARCHITECTURE.md).

## Architecture

Agora is organized in layers:

- Layer 0: the OpenZiti fabric provides cryptographic identity, mutual authentication, end-to-end encryption, and dark-by-default connectivity
- Layer 1: Agora connectivity primitives including organizations, accounts, environments, and tunnels
- Layer 2: agent collaboration services including workgroups, catalog/discovery, advertisements, sessions, contracts, and envelopes

This repository is currently focused on the Layer 1 foundation plus the controller API scaffolding that higher layers will build on.

## Current Status

The codebase is still early-stage, but it already includes the main structural pieces for the controller:

- a Cobra-based `agora` CLI
- a PostgreSQL-only persistence layer built with `sqlx`
- embedded SQL migrations managed with `rubenv/sql-migrate`
- an OpenAPI 3.0.3 specification under `internal/api/specs`
- generated server and client bindings using `ogen`
- a handwritten controller service that implements the generated `ogen` interfaces

The current API surface covers the first Layer 1 slice:

- organization administration
- account creation and token-based authentication
- environment lifecycle
- tunnel lifecycle

## Repository Layout

- `cmd/agora/`: CLI entrypoints and Cobra command wiring
- `internal/api/`: generated `ogen` code and the modular OpenAPI spec
- `internal/controller/`: handwritten controller service, auth logic, and HTTP server wiring
- `internal/persistence/`: store, repositories, models, migration management, and integration tests
- `CLAUDE-ARCHITECTURE.md`: high-level architecture and product direction
- `AGENTS.md`: contributor rules and project conventions

## Development

### Prerequisites

- Go 1.25+
- PostgreSQL
- Docker, if you want to run the persistence and controller integration tests that use `testcontainers-go`

### Generate API Code

```bash
./bin/generate_rest.sh
```

This regenerates the `ogen` client/server package from `internal/api/specs/agora.yml`.

### Run Tests

```bash
go test ./...
```

### Run the Controller

The controller expects a YAML config file. At minimum, configure a bind address, one or more admin tokens, and a PostgreSQL DSN.

Example:

```yaml
bindAddress: ":8080"
adminTokens:
  - "replace-me"
store:
  dsn: "postgres://user:password@localhost:5432/agora?sslmode=disable"
  maxOpenConns: 4
  maxIdleConns: 4
```

Start the controller with:

```bash
go run ./cmd/agora controller ./agora.yml
```

### Manage Migrations

```bash
go run ./cmd/agora store migrate up <configPath>
go run ./cmd/agora store migrate down <configPath>
go run ./cmd/agora store migrate status <configPath>
go run ./cmd/agora store check-schema <configPath>
```

## Design Notes

- PostgreSQL is the only supported database
- the OpenAPI specification is the source of truth for the controller API
- generated `ogen` code should be regenerated, not edited by hand
- Layer 1 should remain useful on its own for secure service connectivity, even before the full Layer 2 agent collaboration model is implemented
