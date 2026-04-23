# agentbase

Shared agent scaffolding for the Macro Pulse demo agents. Every agent in `examples/macro-pulse/` uses this package to avoid ~100 lines of per-agent boilerplate around environment loading, controller client setup, local runtime connection, configuration, logging, and lifecycle.

This README is the design document for the package. The Go code is intentionally not yet written — the API surface proposed here needs review before implementation, because every agent in the demo will depend on it.

## Why

Without `agentbase`, each demo agent's `main.go` has to:

1. Locate and load the local `~/.agora`-style environment root
2. Verify the environment is enabled
3. Build a controller API client with the enrolled account's token
4. Dial the local `agora network` daemon over gRPC+UDS (for agents that host serves or open connects)
5. Parse CLI flags and environment variables
6. Set up structured logging via `df/dl`
7. Install signal handlers for clean shutdown
8. Wire in the agent-specific business logic
9. Tear down cleanly on exit

That is genuinely ~100 lines per agent. Nine agents × 100 lines = 900 lines of nearly-identical boilerplate, with bug fixes needing to happen in nine places. `agentbase` collapses steps 1-7 and 9 to a handful of calls; each agent's `main.go` becomes mostly its own business logic.

`agentbase` is not an SDK. It is deliberately demo-internal. See [foundation.md](../../../../docs/layer-2/foundation.md) "SDK Surface" for why no public SDK is committed in the first Layer 2 slice. `agentbase` is a reference pattern for what such an SDK could eventually look like, but it ships under `examples/macro-pulse/internal/` to make its non-public status visible.

## Proposed API surface

```go
// Package agentbase provides shared scaffolding for Macro Pulse demo agents.
package agentbase

// Agent is the central handle an agent's business logic interacts with.
// Callers receive an *Agent in the OnStart callback; they do not construct
// it directly.
type Agent struct { /* ... */ }

// App configures and runs an agent. Constructed by New; mutated by
// option functions; run by Run.
type App struct { /* ... */ }

// New constructs an agent App with the given name and options.
//
//   app := agentbase.New("news-pulse",
//       agentbase.WithDescription("News volume and sentiment feed"),
//       agentbase.WithFlagSet(myFlags),
//       agentbase.WithEnvRoot("~/.agora-news-pulse"),  // optional override
//   )
func New(name string, opts ...Option) *App

// Option configures an App.
type Option func(*App)

// WithDescription sets the agent's human-readable description (used in
// logs, advertisements once the slice lands, and --help output).
func WithDescription(desc string) Option

// WithFlagSet adds a *flag.FlagSet of agent-specific flags. agentbase
// handles flag parsing and merges with its own built-in flags
// (--env-root, --log-level, --verbose, --live).
func WithFlagSet(fs *flag.FlagSet) Option

// WithEnvRoot overrides the default environment root path (~/.agora).
// Useful when running multiple agents on the same machine against
// separate enrolled environments.
func WithEnvRoot(path string) Option

// WithBusinessLogic registers the OnStart callback — the agent's main
// loop. Receives a cancellable context that is cancelled on shutdown
// and a fully-initialized *Agent.
func WithBusinessLogic(fn func(context.Context, *Agent) error) Option

// Run parses flags, initializes the agent, invokes business logic,
// and blocks until the business logic returns or a signal arrives.
// Returns a non-zero-intent error if initialization fails or business
// logic returns an error. Intended to be called from main as the last
// line: agentbase.New(...).Run().
func (a *App) Run() error
```

The `*Agent` handle exposes:

```go
// Name returns the agent's name as passed to New.
func (a *Agent) Name() string

// Env returns the loaded local environment root.
func (a *Agent) Env() env_core.Root

// Controller returns a controller API client authenticated with the
// enrolled account token. Usable for any existing Layer 1 endpoint
// and (post-slice) any Layer 2 endpoint the account can access.
func (a *Agent) Controller() *api.Client

// Runtime returns a gRPC client to the local `agora network` daemon.
// Nil until the network-runtime connection is successfully established;
// check via RuntimeAvailable() before use.
func (a *Agent) Runtime() networkpb.NetworkServiceClient

// RuntimeAvailable returns true if the local agora network daemon is
// reachable. Some agents (e.g. analytics tools) may not need it; the
// agent logs a warning but continues if the daemon is down.
func (a *Agent) RuntimeAvailable() bool

// Logger returns a df/dl logger scoped with agent_name and account_id
// fields pre-populated.
func (a *Agent) Logger() *dl.Logger

// Live returns the value of the --live flag. Data agents consult this
// to decide between snapshot and live-API behavior.
func (a *Agent) Live() bool
```

## Built-in flags and environment variables

`agentbase` contributes these flags to every agent:

- `--env-root <path>` — override environment root (default `~/.agora`)
- `--log-level <level>` — one of `debug`, `info`, `warn`, `error` (default `info`)
- `--verbose` / `-v` — shorthand for `--log-level debug`
- `--live` — opt in to live external data sources (default off; providers read it, consumers ignore it)

Environment variables:

- `AGORA_ENV_ROOT` — same as `--env-root`, flag takes precedence
- `AGORA_LOG_LEVEL` — same as `--log-level`

`--env-root` is essential for multi-agent single-machine demos: each agent points at its own `~/.agora-<agent-name>` root so they don't conflict.

## Minimal example usage

```go
package main

import (
    "context"
    "flag"

    "github.com/openziti/agora/examples/macro-pulse/internal/agentbase"
)

func main() {
    fs := flag.NewFlagSet("news-pulse", flag.ExitOnError)
    topic := fs.String("default-topic", "financial", "default topic")

    app := agentbase.New("news-pulse",
        agentbase.WithDescription("News volume and sentiment feed"),
        agentbase.WithFlagSet(fs),
        agentbase.WithBusinessLogic(func(ctx context.Context, a *agentbase.Agent) error {
            a.Logger().Infof("news-pulse alive, default_topic='%s'", *topic)
            <-ctx.Done()
            return nil
        }),
    )
    if err := app.Run(); err != nil {
        panic(err)
    }
}
```

## What agentbase does NOT do (in MVP)

- **Does not enroll environments.** `agora enable` is still done out-of-band (by `bootstrap/` or by the operator). `agentbase` assumes the environment is already enabled and will fail fast with a clear error if not.
- **Does not publish advertisements.** That is a slice-gated capability; `agentbase` gains `PublishAdvertisement(...)` when the advertisement slice lands.
- **Does not handle sessions.** Likewise slice-gated. `agentbase` gains `AcceptSessions(handler)` and `ProposeSession(...)` when the session slice lands.
- **Does not parse envelopes.** Slice-gated. `agentbase` gains envelope send/receive helpers when the envelope slice lands.
- **Does not define agent capabilities.** That is per-agent business logic. `agentbase` provides the machinery; each agent provides its `news.pulse`, `markets.equity`, etc., semantics.

## Growth by slice

| Slice | Additions to agentbase |
| --- | --- |
| (now, pre-Layer-2) | `App`, `Agent`, `Run`, `Controller`, `Runtime`, `Logger`, `Live`, built-in flags |
| Workgroups | No direct changes. Agents pick up workgroup memberships through the bootstrap program, not through agentbase. |
| Catalog + advertisements | `Advertise(spec AdvertisementSpec) (AdvertisementHandle, error)` for providers; `Discover(capability, filters) ([]Advertisement, error)` for consumers |
| Sessions | `AcceptSessions(handler SessionHandler) error` for providers; `OpenSession(advID, contract) (*Session, error)` for consumers |
| Contracts | Contract types referenced in `OpenSession`. No new `Agent` methods; contracts are a parameter. |
| Envelopes | `Session.Send(envelope Envelope) error`; `Session.Recv() (Envelope, error)`; envelope struct type with header + payload |

Each slice's PR advances `agentbase` alongside the slice itself.

## Open questions

These need resolution before the Go code is written. They are deliberately narrow; the API surface above is otherwise considered ready.

- **Logger return type.** Is `Logger() *dl.Logger` right, or should `agentbase` expose a narrower interface to avoid locking every agent to `df/dl`? Proposed: return the concrete `*dl.Logger` — `df/dl` is already a project convention and there is no reason to abstract over it here. Confirm.
- **Runtime availability semantics.** Some agents (analytics tools: `correlator`, `narrator`) don't need the local `agora network` daemon. Should `agentbase` fail if the daemon is unreachable, or warn and continue with `RuntimeAvailable() = false`? Proposed: warn and continue. Analytics tools never call `Runtime()`. Data providers and `pulse-agent` consult `RuntimeAvailable()` before attempting runtime operations. Confirm.
- **OnStart signature.** Currently `func(context.Context, *Agent) error`. Considered alternative: pass the agent's individual flags via the callback signature (`func(context.Context, *Agent, *MyAgentFlags) error`) to avoid each agent keeping a top-level pointer. Proposed: keep the current signature — flags are scoped to the caller's package anyway. Confirm.

## Implementation sequence

Once this design is approved:

1. Create `agentbase.go` with the types and constructors (no runtime behavior)
2. Implement `Run()` — flag parsing, env loading, controller client setup, runtime dial, signal handler, business-logic invocation
3. Add tests: env-root override, graceful shutdown, runtime-unavailable path
4. Add one agent (`pulse-agent` makes sense as it's the most complex consumer) `main.go` skeleton using the package
5. Add the other eight agent skeletons once the pattern is validated
