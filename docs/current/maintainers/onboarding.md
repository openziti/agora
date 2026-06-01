# Agent Onboarding

A one-pass runbook for an engineer or coding agent landing on this repo cold. Companion to `AGENTS.md` (auto-loaded into every session) and `docs/current/maintainers/current-state.md` (snapshot of what's implemented). Read this once, then operate off `AGENTS.md` plus the canonical docs under `docs/current/`.

## What runs where

Agora's controller is a Go binary backed by **PostgreSQL** and an **OpenZiti** control plane. Both are external preconditions — `bin/demo-up.sh` does not start them. Two controller configs ship in the tree and run on different ports against different databases so they can coexist on one workstation:

| Config | Bind | Admin token | DSN | Used by |
|---|---|---|---|---|
| `etc/dev-agora-controller.yaml` | `:18081` | `dev-admin-token` | `postgres://agora:agora@localhost:5432/agora` | Your hand-managed dev stack |
| `etc/demo-controller.yaml` | `:18080` | `demo-admin-token` | `postgres://agora:agora@localhost:5432/agora_demo` | `bin/demo-up.sh` (standalone) |

Both configs expect OpenZiti's controller at `https://localhost:1280` with updb auth `admin/admin`. If OpenZiti isn't running, controller startup and tunnel/session flows will fail.

The UI (React + Vite + Tailwind under `ui/`) is **embedded into the controller binary** at build time via `ui/embed.go`, so a built `agora controller` serves the dashboard at its `bind_address`. During UI development you can also run `npm run dev` in `ui/` and let Vite proxy `/v1/*` to the controller (`ui/vite.config.ts` — `AGORA_CONTROLLER_URL` env var overrides the target).

## First stack up

Pick whichever matches your goal:

- **You just want to see the demo running.** Follow the README's Quick Start — `bin/demo-up.sh` provisions the demo controller, runs migrations, bootstraps demo orgs/accounts/workgroups/contracts, and starts the eight macro-pulse worker agents plus the pulse-agent orchestrator. State lives under `~/.agora-demo/`. `bin/demo-down.sh --purge` resets it.
- **You want to develop against your own controller.** Run the dev controller yourself (`agora controller etc/dev-agora-controller.yaml`) with the dev DSN's Postgres available and OpenZiti up. Then `bin/demo-up.sh --attach` will skip the UI build, migrations, and controller start, and instead bootstrap demo data + start worker agents against your already-running controller. Requires `AGORA_DEMO_CONTROLLER_URL` and `AGORA_DEMO_ADMIN_TOKEN` exported in your shell. See `bin/demo-up.sh --help`.

Both paths converge on the same demo topology (orgs, accounts, contracts, agents) — `--attach` just pushes that topology into the controller you're already running instead of into a script-managed one.

## Where to look for X

A short map; mirrors `current-state.md`'s repo shape but oriented to "where do I edit when…?"

| If you're changing… | Look in |
|---|---|
| Controller HTTP behavior | `internal/controller/<operationId>.go` (one file per OpenAPI operation) |
| API contract / response shape | `internal/api/specs/` (spec is canonical), then regenerate with `./bin/generate_rest.sh` |
| DB schema | `internal/persistence/migrations/NNNN_<name>.sql` (additive forward only) + repo method in `internal/persistence/<table>.go` |
| Authentication / session middleware | `internal/controller/auth_middleware.go` + `internal/controller/service.go` (principals) |
| Local CLI environment / `~/.agora` | `environment/` |
| CLI command (Cobra) | `cmd/agora/<group><Verb>.go`; commands register via `init()` from a group file like `advertise.go` / `admin.go` |
| Agent SDK (embeddable runtime, App/Agent scaffolding) | `sdk/agent/` |
| Network runtime (HTTP/TCP/UDP engine, daemon helpers) | `internal/network/tunnelruntime/`, `internal/network/daemon/` |
| OpenZiti automation (Layer 0 helpers) | `internal/fabric/openziti/automation/` |
| Dashboard screens | `ui/src/screens/<Screen>.tsx` (one screen per top-level route) |
| Dashboard API client | `ui/src/lib/api/<resource>.ts` (hand-written wrappers around `apiRequest`); re-exported from `ui/src/lib/api/index.ts` |
| Dashboard polling | `ui/src/lib/api/hooks.ts` — `useApiResource(load, { intervalMs })` is the only blessed live-refresh pattern; pair it with an `initial-load` gate on `isLoading` so the AppShell status pill doesn't flicker on background refresh |
| Demo content (orgs, accounts, contracts, workgroup membership) | `cmd/demo-bootstrap/topology.yaml` + handlers in `cmd/demo-bootstrap/` |
| Demo workers | `examples/macro-pulse/cmd/macro-pulse-*` |

## Known operational footguns

These are real failure modes encountered in practice. None are obvious from grepping.

- **`npm ci` corruption survives interruption.** SIGINT during `(cd ui && npm ci)` leaves `ui/node_modules/` half-populated; the next `npm run build` then fails with type-import errors deep inside `@types/*`. `bin/demo-up.sh` writes a `ui/node_modules/.demo-npm-ci-in-progress` sentinel around `npm ci` and self-heals on the next run by wiping `node_modules` before retry. If you ever see "Cannot find module 'undici-types'" or similar `node_modules`-internal errors out of nowhere, the fix is `rm -rf ui/node_modules && npm ci`.
- **Stale env tokens are silent and lethal.** `~/.agora-demo/envs/<agent>/environment.json` caches an `account_token` and `api_endpoint`. If the controller DB is reset (or you switch controllers) without also purging `~/.agora-demo/`, demo-bootstrap will "reuse" the env roots, workers will start with tokens that don't authenticate, and the controller log will show `account token authentication failed: persistence: not found`. `bin/demo-up.sh` now runs a whoami preflight against every cached env token before bootstrap — when it fires, the fix is `bin/demo-down.sh [--attach] --purge` and rerun.
- **`bin/demo-up.sh --attach` does not touch your `agora` binary in `$GOBIN`.** It only `go install`s the demo-specific binaries (`demo-bootstrap`, `macro-pulse-*`). If your controller binary is out of date, that's on you — same with the UI bundle.
- **Generated UI bindings drift from the spec.** If you edit `internal/api/specs/` and rebuild only the controller, the UI still has the old TypeScript types. The UI does not currently regenerate from the spec — its `ui/src/lib/api/types.ts` is hand-maintained. Spec changes that affect UI-visible fields need a manual UI sync.
- **`go build ./path/to/cmd` writes a ~50MB binary into the working tree.** Repo rule, in `AGENTS.md`. Use `go vet ./...` for compile checks; if you really need to build a specific main, `go build -o /dev/null ./path/to/cmd`.
- **Process cleanup is part of "done."** Per `AGENTS.md`: any controller, Vite dev server, or worker started for verification must be stopped before handing the work back. `ss -ltnp sport = :<port>` is the standard check for "is anything still listening on the port I used?"

## Verification recipes

Common end-of-change checks. None of these are mandatory in isolation; pick the narrowest set that covers what you actually touched.

- **Go change, single package**: `go vet ./internal/<package>/... && go test ./internal/<package>/...`. If the change crosses package or wiring boundaries, follow with `go test ./...`.
- **Persistence change**: run the relevant `*_integration_test.go` (uses Postgres testcontainers — needs Docker). Don't weaken integration coverage to make tests easier to run locally.
- **OpenAPI spec change**: edit spec → `./bin/generate_rest.sh` → adapt handwritten code to the new generated types → `go vet ./...` → relevant controller tests. If protobuf shapes changed, also `./bin/generate_pb.sh`.
- **UI change**: `cd ui && npm run build` (tsc + vite) — this is the type-check. For visual verification spin up `npm run dev` against a running controller; stop the dev server before handing back. Use the existing `useApiResource` polling pattern instead of reinventing data refresh.
- **Demo change**: bring the stack up with whichever mode fits, verify the change end-to-end, then `bin/demo-down.sh [--attach] [--purge]`.

## After this

Operate off `AGENTS.md` for the rules-with-reasons. Read into `docs/current/<area>/` only when the task lands you in an area you haven't worked in before — the layer docs and architecture overview are normative, not introductory.
