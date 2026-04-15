# Coding Agent Instructions

## Agora

Agora is a native zero-trust overlay network for agent-to-agent communication, built on OpenZiti. It provides secure connectivity primitives at Layer 1 (Network) and governed agent collaboration services at Layer 2 (Collaboration).

This repository is in early-stage development. Favor simple, explicit structure and keep implementation aligned with `CLAUDE-ARCHITECTURE.md`.

## Development Commands

### Build Commands
- Full build: `go build ./...`
- Install binary: `go install ./...`

### Testing
- Go tests: `go test ./...`
- Persistence integration tests use PostgreSQL containers via `testcontainers-go`
- Before finishing a change, run the narrowest relevant tests first, then `go test ./...` when the change touches shared code or project wiring

## Architecture Overview

### Core Components

**CLI Command Structure** (`cmd/agora/`):
- Main binary uses Cobra with package-level command groups and per-file leaf commands
- Follow the same structural pattern used by `zrok`: command tree in `main.go`, individual commands registered via `init()`
- CLI and controller logging should use `github.com/michaelquigley/df/dl`, not `log/slog` directly
- CLI commands that talk to the controller API should prefer the local environment root under `~/.agora` over reading controller server config files directly
- Admin API commands should take their admin token from `AGORA_ADMIN_TOKEN`, not from controller YAML
- For human-readable list commands, standardize on `go-pretty/table` with the same rounded-table presentation pattern used in `zrok`
- For machine-readable list output, use `--json` and emit indented raw resource objects rather than CLI-specific wrapper objects

**Persistence Layer** (`internal/persistence/`):
- PostgreSQL-only persistence built with `sqlx`
- Embedded SQL migrations managed through `rubenv/sql-migrate`
- Repository methods accept `context.Context` and a shared query/transaction interface

**Configuration Binding**:
- Structured config loading and all handwritten JSON/YAML binding and unbinding should use `github.com/michaelquigley/df/dd`
- Prefer `dd.MergeYAMLFile(...)` over manual file reads plus `yaml.Unmarshal(...)`
- Prefer `dd.Bind...` / `dd.Unbind...` helpers over `encoding/json`, `yaml.Unmarshal(...)`, or ad hoc marshal/unmarshal logic in handwritten code
- `dd` binds struct fields using `snake_case` by default; do not add YAML tags to config structs unless there is a concrete need to override that mapping
- Keep root-level configuration defaults in `DefaultConfig()`
- When an optional config sub-struct is a pointer, prefer implementing `ApplyDefaults()` on that sub-struct instead of doing post-merge default repair in callers
- Express unconditional config requirements with `dd:",+required"` struct tags instead of duplicating the same validation in load paths
- If dynamic config sections are introduced later, keep them within the `dd` binding model instead of introducing a second config-loading pattern

**Local Environment Root** (`environment/`):
- Keep the local CLI environment rooted under `~/.agora`, following the same general pattern as `zrok`
- Store local CLI metadata such as API endpoint config and enrolled identities there, not in controller config files
- Prefer extending the environment package over scattering ad hoc dotfiles or one-off env var parsing across commands

**Internal Package Naming**:
- Layer-owned internal packages should follow the conceptual layer names:
- `internal/fabric/...` for Layer 0 (Fabric) implementation code
- `internal/network/...` for Layer 1 (Network) implementation code
- `internal/collaboration/...` for Layer 2 (Collaboration) implementation code when that code exists
- Keep cross-cutting packages such as `internal/controller`, `internal/persistence`, `internal/api`, and `internal/clioutput` at the top level rather than forcing them into a layer namespace

### Key Architectural Patterns

**API Generation**:
- Agora uses OpenAPI 3.x as the API contract
- Generate Go API bindings with `ogen`
- Regenerate REST bindings with `./bin/generate_rest.sh`
- Regenerate protobuf/gRPC bindings with `./bin/generate_pb.sh`
- Treat the OpenAPI specification as the source of truth
- Do not hand-edit generated code
- Keep generated code in clearly designated locations and regenerate it from the spec instead of patching outputs manually
- When API shapes change, update the spec first, regenerate, then adapt handwritten code to the new generated interfaces

**Persistence and Migrations**:
- Keep schema changes in embedded SQL migration files under `internal/persistence/migrations/`
- Use the `migrations` table as the schema bookkeeping table
- Do not add SQLite compatibility paths unless explicitly requested
- Migration filenames should be ordered, stable, and descriptive, for example `0003_add_workgroups.sql`
- Prefer additive forward migrations; only write destructive rollback logic that is safe and intentional
- Enforce tenant boundaries, uniqueness, and foreign-key integrity in schema where possible instead of relying on handler-level checks

**Multi-tenancy**:
- Organizations are the top-level tenant boundary
- Keep Layer 1 (Network) resources org-scoped unless the architecture explicitly requires otherwise
- Enforce org boundaries in the database schema, not just in application logic

**Generated vs handwritten code**:
- Do not mix generated files and handwritten business logic in the same package when a cleaner boundary is practical
- Keep handwritten adapters, repositories, and command implementations separate from generated API bindings
- If a file is generated, mark it clearly and avoid manual edits

## Golang Conventions

- Comments start with lowercase letters unless referring to Go types
- Error strings use lowercase unless referring to identifiers or proper nouns
- Dynamic values in user-facing outputs should be surrounded by single quotes when practical
- Never introduce emoji in outputs or source comments
- Use `github.com/michaelquigley/df/dl` for logging; do not introduce direct `slog` logger wiring as a parallel logging pattern
- Use `github.com/michaelquigley/df/dd` for handwritten config and JSON/YAML binding/unbinding instead of ad hoc stdlib or YAML parsing paths
- Prefer tagless config structs when `dd`'s default `snake_case` field mapping is sufficient
- Prefer `ApplyDefaults()` on optional config pointer structs over ad hoc `if cfg.Sub.Field == ...` post-processing
- Keep package APIs explicit and small; prefer simple structs and functions over hidden framework behavior
- Thread `context.Context` through I/O boundaries and long-running operations
- Prefer concrete tests around real behavior over excessive mocking

## Test Expectations

- Add or update tests for behavior changes, not just happy paths
- For persistence changes, cover:
  - migration application
  - constraint enforcement
  - transaction behavior
  - compatibility checks when relevant
- For CLI changes, at minimum ensure the command wiring compiles and keep command construction structurally consistent across files
- If a test requires external runtime support such as Docker, call that out clearly when reporting results
- Do not silently weaken integration coverage just to make tests easier to run

## Development Notes

- Keep new code aligned with the current architecture document rather than `zrok` feature parity
- Borrow patterns from `zrok` where useful, but remove compatibility layers and incidental complexity that Agora does not need
- Prefer explicit repository and command structures over framework-heavy abstractions
- When copying a pattern from `zrok`, preserve the underlying structural convention only if it still fits Agora’s architecture and current implementation choices
