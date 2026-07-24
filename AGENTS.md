# Coding Agent Instructions

## Agora

Agora is a native zero-trust overlay network for agent-to-agent communication, built on OpenZiti. It provides secure connectivity primitives at Layer 1 (Network) and governed agent collaboration services at Layer 2 (Collaboration).

This repository is in early-stage development. Favor simple, explicit structure and keep implementation aligned with `docs/current/architecture/overview.md` and the relevant layer docs under `docs/current/`.

If you've never worked in this repo before, read [`docs/current/maintainers/onboarding.md`](./docs/current/maintainers/onboarding.md) once — it covers what runs where (dev controller `:18081` vs demo controller `:18080`, external Postgres + OpenZiti), the two ways to bring the stack up (`bin/demo-up.sh` standalone vs `--attach` against your own controller), and the operational footguns below in long form.

## Where to look

| If you're changing… | Look in |
|---|---|
| Controller HTTP behavior | `internal/controller/<operationId>.go` (one file per OpenAPI operation) |
| API contract | `internal/api/specs/` (canonical) → regenerate with `./bin/generate_rest.sh` |
| DB schema | `internal/persistence/migrations/NNNN_<name>.sql` + repo in `internal/persistence/<table>.go` |
| Auth / principal middleware | `internal/controller/auth_middleware.go`, `internal/controller/service.go` |
| CLI command | `cmd/agora/<group><Verb>.go` (registered via `init()` in the group file) |
| Agent SDK | `sdk/agent/` |
| Network runtime | `internal/network/tunnelruntime/`, `internal/network/daemon/` |
| Dashboard screens | `ui/src/screens/<Screen>.tsx` |
| Dashboard API client | `ui/src/lib/api/<resource>.ts` (hand-written; re-exported from `index.ts`) |
| Dashboard polling | `ui/src/lib/api/hooks.ts` — `useApiResource(load, { intervalMs })` is the blessed live-refresh pattern |
| Demo content | `cmd/demo-bootstrap/topology.yaml` + handlers under `cmd/demo-bootstrap/` |
| Demo workers | `examples/macro-pulse/cmd/macro-pulse-*` |

## Documentation Structure

- `docs/` is split into `docs/current/` (verifiable against running code) and `docs/future/` (vision, work orders, deferred designs). New docs default to the bucket that matches their nature; promote from `future/` to `current/` as work lands. See [`docs/README.md`](./docs/README.md) for the full convention — what each bucket means, where new docs go, and the promotion procedure.
- The canonical architecture, status, maintainer, and roadmap materials live under `docs/`.
- Use `docs/current/architecture/overview.md` for cross-layer architecture and system-wide design.
- Use `docs/current/layer-1/spec.md`, `docs/current/layer-1/status.md`, and `docs/current/layer-1/agent.md` for Layer 1 (Network) normative behavior, current state, and local-runtime design.
- Use `docs/current/layer-2/spec.md` and `docs/current/layer-2/status.md` for Layer 2 (Collaboration) design and implementation status.
- Use `docs/current/maintainers/current-state.md` for repo-shape, workflow, and maintainer-facing current-state context.
- Use the roadmap (`docs/roadmap.md`, with one card per item under `docs/future/roadmap/`) for explicitly deferred work such as metrics and limits.
- Do not create new root-level planning or handoff docs that duplicate the `docs/` canon.
- Keep spec docs normative, status docs factual/current-state, and roadmap docs limited to deferred or later-phase work.

## Development Commands

### Build Commands
- Full build: `go build ./...`
- Install binary: `go install ./...`

### Build verification (do not leave stray binaries in the working tree)
- `go build ./...` at the repo root is safe: it only compiles and never writes binaries to disk.
- `go install ./...` is safe: outputs land in `$GOBIN` / `$GOPATH/bin`, not in the working tree.
- `go vet ./...` is safe and is the preferred compile-check for a single change.
- **Never** run `go build ./path/to/cmd` or `go build ./path/to/cmd/...` — those forms emit the built binary into the current directory, which is almost always the repo root for an agent session. Those stray binaries are trivial to accidentally `git add` and commit (and at ~50 MB each, they are expensive mistakes).
- If a specific `main` package needs to be compiled to check a change, use `go build -o /dev/null ./path/to/cmd` so no file is written.
- `.gitignore` already lists the known `main` package output names as a safety net, but the rule above is what prevents the problem in the first place.

### Testing
- Go tests: `go test ./...`
- Persistence integration tests use PostgreSQL containers via `testcontainers-go`
- Before finishing a change, run the narrowest relevant tests first, then `go test ./...` when the change touches shared code or project wiring

### Known Operational Footguns

Failure modes that aren't obvious from the code. Long-form descriptions in [`docs/current/maintainers/onboarding.md`](./docs/current/maintainers/onboarding.md).

- **`npm ci` corruption survives interruption.** A SIGINT during `(cd ui && npm ci)` leaves `ui/node_modules/` partially populated; the next `npm run build` fails deep inside `@types/*`. `bin/demo-up.sh` self-heals via a sentinel; if you hit this elsewhere, `rm -rf ui/node_modules && npm ci`.
- **Stale env tokens are silent.** `~/.agora-demo/envs/<agent>/environment.json` caches an `account_token`. If the controller DB is reset without also purging `~/.agora-demo/`, demo-bootstrap reuses the stale env roots and workers fail to authenticate. `bin/demo-up.sh` runs a whoami preflight; when it fires, the fix is `bin/demo-down.sh [--attach] --purge`.
- **External preconditions are external.** Neither demo-up nor the controller starts Postgres or OpenZiti. If you see startup or auth failures pointing at `localhost:5432` or `localhost:1280`, those services aren't running.
- **`--attach` mode does not rebuild the controller or UI.** It only `go install`s demo-specific binaries (`demo-bootstrap`, `macro-pulse-*`). Keep your own `agora` binary and UI bundle current when developing this way.
- **UI types don't regenerate from the spec.** `ui/src/lib/api/types.ts` is hand-maintained. Spec changes affecting UI-visible fields need a manual UI sync.

### Test Process Cleanup
- Any process started only for verification or manual testing must be stopped before handing the work back for review or moving to the next work unit. This includes Vite dev servers, mock HTTP controllers, `go run` controller processes, demo agents, background scripts, and ad hoc local listeners.
- When starting a long-running process, keep track of how it will be stopped: prefer an exec session that can be interrupted, or capture the PID/port immediately after startup.
- After stopping a process, verify the relevant port or PID is gone before reporting the step complete. For local listeners, `ss -ltnp sport = :<port>` is an appropriate check.
- Do not leave a review server running by default. If a user explicitly asks for a live URL, mention the process and port in the handoff, then stop it before beginning the next chunk unless the user asks to keep it running.
- Temporary mock services used for proxy or integration checks should be shut down even if the main verification command fails; cleanup is part of the verification task.

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

**Package Naming**:
- The public SDK lives at `sdk/agent` (top-level, outside `internal/`). It exposes the embeddable Layer 1 runtime plus App/Agent scaffolding. Both Agora's own `agora network start` daemon and every agent built on Agora import from here. See [docs/current/sdk/overview.md](./docs/current/sdk/overview.md).
- Layer-owned internal packages follow the conceptual layer names:
- `internal/fabric/...` for Layer 0 (Fabric) implementation code
- `internal/network/...` for Layer 1 (Network) implementation code that is NOT part of the SDK (e.g., `internal/network/daemon` for daemon-client helpers, `internal/network/tunnelruntime` for the HTTP/TCP/UDP engine)
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
