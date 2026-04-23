# agentbase

Shared agent scaffolding for the Macro Pulse demo agents. Every agent in `examples/macro-pulse/` uses this package to avoid ~100 lines of per-agent boilerplate around environment loading, controller client setup, local runtime connection, configuration, logging, and lifecycle.

This README is the design document for the package. The Go code is intentionally not yet written — the API surface proposed here needs review before implementation, because every agent in the demo will depend on it.

## Why

Without `agentbase`, each demo agent's `main.go` has to:

1. Locate and load the local `~/.agora`-style environment root
2. Verify the environment is enabled
3. Build a controller API client with the enrolled account's token
4. Construct and start an embedded Layer 1 runtime (for agents that host serves or open connects)
5. Parse CLI flags and environment variables
6. Set up structured logging via `df/dl`
7. Install signal handlers for clean shutdown
8. Wire in the agent-specific business logic
9. Tear down cleanly on exit (including a clean runtime shutdown that deprovisions active serves and attachments)

That is genuinely ~100 lines per agent. Nine agents × 100 lines = 900 lines of nearly-identical boilerplate, with bug fixes needing to happen in nine places. `agentbase` collapses steps 1-7 and 9 to a handful of calls; each agent's `main.go` becomes mostly its own business logic.

**Embedded runtime, not daemon client.** The runtime referenced in step 4 is the embeddable library form of the Layer 1 runtime, not a gRPC client to a separate daemon process. Each agent owns its own in-process runtime instance; there is no shared daemon. See [../../../docs/layer-1/agent.md](../../../docs/layer-1/agent.md) "Packaging Direction" and "Isolation between the daemon and embedded runtimes."

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

// WithRuntime causes agentbase to construct and start an embedded
// Layer 1 runtime. Required for any agent that accepts sessions
// (providers, tools) or opens sessions (consumers). Omit for pure
// controller-API consumers (e.g. the bootstrap program, admin
// utilities) that don't need tunnels.
//
// The embedded runtime does NOT touch ~/.agora/network.json and does
// NOT bind ~/.agora/network.sock. It is isolated from any standalone
// `agora network` daemon running on the same host.
func WithRuntime() Option

// Run parses flags, initializes the agent (including embedded runtime
// if WithRuntime() was set), invokes fn with a cancellable context
// that is cancelled on SIGINT/SIGTERM, and blocks until fn returns or
// a signal arrives. On signal, Run cancels the context and waits for
// fn to return before tearing down.
//
// Intended to be called from main as the last line:
//
//   app.Run(func(ctx context.Context, a *agentbase.Agent) error { ... })
func (a *App) Run(fn func(context.Context, *Agent) error) error
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

// Runtime returns the embedded Layer 1 runtime instance. Nil if the
// agent was constructed without WithRuntime(). Callers invoke runtime
// methods directly as Go calls; there is no gRPC dial.
func (a *Agent) Runtime() *agent.Embedded

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
    "os"

    "github.com/openziti/agora/examples/macro-pulse/internal/agentbase"
)

func main() {
    fs := flag.NewFlagSet("news-pulse", flag.ExitOnError)
    topic := fs.String("default-topic", "financial", "default topic")

    app := agentbase.New("news-pulse",
        agentbase.WithDescription("News volume and sentiment feed"),
        agentbase.WithFlagSet(fs),
        agentbase.WithRuntime(),   // news-pulse accepts sessions; needs the embedded runtime
    )
    if err := app.Run(func(ctx context.Context, a *agentbase.Agent) error {
        a.Log().Infof("alive; default_topic='%s'", *topic)
        <-ctx.Done()
        return nil
    }); err != nil {
        os.Exit(1)
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

- **Logger return type.** Resolved: `Log() *dl.Builder` returning `dl.Log().With("agent", a.Name()).With("account_id", a.AccountID())`. This matches what df/dl actually exposes (package-level log functions plus a chainable `*Builder` from `dl.Log()`) and ensures every log line from an agent carries agent-identifying structured fields automatically.
- **Runtime opt-in semantics.** Resolved: runtime is an embedded library, not a remote daemon. Agents opt in via `WithRuntime()`. Agents that need sessions (all providers, all tools, `pulse-agent`) declare it; `bootstrap` and any future pure-admin-CLI agents omit it. `a.Runtime()` returns nil when not declared; there is no "runtime unreachable" middle state. See [../../../docs/layer-1/agent.md](../../../docs/layer-1/agent.md) "Packaging Direction" for the isolation model.
- **Business logic entry point.** Resolved: business logic is passed as an argument to `app.Run(fn)` rather than via a `WithBusinessLogic(...)` option. Signature is `func(ctx context.Context, a *Agent) error`; flags are captured by closure from the surrounding scope. Construction options describe the agent's shape; `Run` invokes its behavior.

## Implementation sequence

Once this design is approved:

1. Create `agentbase.go` with the types and constructors (no runtime behavior)
2. Implement `Run()` — flag parsing, env loading, controller client setup, runtime dial, signal handler, business-logic invocation
3. Add tests: env-root override, graceful shutdown, runtime-unavailable path
4. Add one agent (`pulse-agent` makes sense as it's the most complex consumer) `main.go` skeleton using the package
5. Add the other eight agent skeletons once the pattern is validated
