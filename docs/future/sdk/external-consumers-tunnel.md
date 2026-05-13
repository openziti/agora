# Agora SDK — External Consumer Spec: Layer 1 Tunnels

Status: design proposal, not yet implemented.
Driving consumer: the LLM Gateway and MCP Gateway agora-mode work,
specifically the "serve over Agora" and "per-provider connect" features
(see [`llm-gateway/docs/future/agora.md`](https://github.com/openziti/llm-gateway/blob/main/docs/future/agora.md)).

This spec complements [`external-consumers.md`](./external-consumers.md),
which addresses Layer 2 catalog operations. This document addresses
Layer 1 tunnel operations (serve and connect) plus a cross-cutting
issue — external `*Agent` construction — that affects both surfaces.

## Purpose

Two related gaps in the SDK's external-consumer story:

1. **Tunnels.** External Go modules need a Go-idiomatic public package
   for Layer 1 tunnel serve and connect operations. The current
   `*Runtime` surface uses protobuf types (`networkpb.EnsureServeRequest`
   etc.) directly. While those types are in a public package path,
   their proto idioms (enum integers, string-typed modes, etc.) are
   awkward for Go-native consumers.

2. **Agent construction.** External Go modules currently cannot
   construct an `*agent.Agent` outside of `app.Run`. `app.Run` is
   opinionated — it installs signal handlers, parses `os.Args`, and
   initializes the global `dl` logger. Consumers that have their own
   lifecycle (e.g., an HTTP gateway with its own Cobra-managed signal
   handling) need a path to obtain `*Agent` without inheriting those
   behaviors.

This spec fixes both. The fixes are described independently; either
can ship without the other, though the tunnel package is unusable in
practice without the Agent-construction fix because external
consumers can't acquire an `*Agent` to call its functions on.

## Background

### The Layer 1 surface today

`*Runtime` exposes `EnsureServe`, `RemoveServe`, `ListServes`, plus
the `EnsureConnect`/`RemoveConnect`/`ListConnects` equivalents. They
take and return types from `github.com/openziti/agora/sdk/agent/networkpb`,
which is a public package. So external consumers can in principle
call them today:

```go
runtime := someAgent.Runtime()
runtime.EnsureServe(ctx, &networkpb.EnsureServeRequest{
    Name:          "llm-gateway",
    Mode:          "tcp",
    BackendTarget: "127.0.0.1:8080",
    GrantEmails:   []string{"alice@example.com"},
})
```

What's awkward:

- `Mode` is a raw string, not a typed constant. Typos are runtime
  errors, not compile errors.
- `networkpb.RuntimeState_RUNTIME_STATE_RUNNING` is the proto-generated
  enum name for "running" — verbose and unidiomatic.
- The proto messages have generated fields with `Get*()` accessors
  that go unused (because the messages also have direct field access),
  cluttering the API surface.
- Error semantics are whatever the underlying call returns; no
  sentinel-based wrapping consistent with `sdk/agent/catalog`.
- No high-level `EnsureServed`-style helper that wraps the runtime's
  restart-on-match behavior with input validation, typed errors, and
  a clear synchronous-attempt contract. Catalog's `EnsurePublished`
  is a structural cousin but not an exact analog — catalog records
  persist on the controller and `EnsurePublished` returns the
  existing record on name conflict; tunnel actors are runtime-scoped
  and reconcile by restart instead.

None of these are blockers. They're ergonomic friction that makes
external consumers' code look like it's reaching into transport
internals when it isn't.

### The Agent-construction barrier

`*agent.Agent` is a public type. Its constructor is not:

```go
type Agent struct {
    appName   string         // unexported
    root      env_core.Root  // unexported
    env       *env_core.Environment
    accountID string
    api       *api.Client
    runtime   *Runtime
    live      bool
}
```

The only path to a populated `*Agent` is through `app.Run`. That call:

1. **Parses `os.Args`** via a `flag.FlagSet`, with built-in flags
   (`--env-root`, `--log-level`, `-v`, `--live`) merged with any
   user-supplied flag set via `WithFlagSet`. A consumer that does its
   own CLI parsing — e.g., a Cobra-based gateway — has to either
   surface SDK flags through its own CLI, work around them with a
   filter on `os.Args`, or duplicate the flags' effects in its own
   flag definitions.
2. **Calls `dl.Init` globally**, overwriting any prior dl
   configuration. A consumer that has its own dl-based logger setup
   sees its log level, output, and prefix-trim settings reset to the
   SDK's choices.
3. **Installs a signal handler** via `signal.NotifyContext` for
   `SIGINT` and `SIGTERM`, with an internal context cancelled when
   either signal fires. A consumer that already handles signals (the
   gateway uses Cobra's signal-handled `cmd.ExecuteContext`) will
   have two parallel handlers. Both fire on signal; cleanup is
   idempotent in well-written code; the result is technically
   correct but reflects poorly on a consumer that has its own model.
4. **Blocks until `fn` returns**, then closes the embedded runtime
   with a 10-second internal timeout. A consumer that needs to
   manage shutdown timing externally has to coordinate with that
   timeout.

For the macro-pulse agents this is exactly what's wanted: small CLI
programs with the SDK as the primary lifecycle. For an HTTP gateway
that already has its own CLI, logging, and signal model, the right
shape is a build-it-yourself constructor that leaves lifecycle to the
caller.

## Scope

In scope:

- A new package, `sdk/agent/tunnel`, that exposes Go-idiomatic types
  and helpers for serve and connect operations, internally wrapping
  the existing `*Runtime` methods.
- A new exported constructor for `*Agent` (proposed: `agent.NewStandalone`)
  that builds `*Agent` from explicit options, without signal handlers,
  flag parsing, or global log init.
- The lifecycle contract for the new constructor: how the caller starts
  the embedded runtime, drives the agent, and shuts it down cleanly.

Explicitly out of scope:

- Sessions. `sdk/agent/session/` still has its own public-types pass
  to do; this spec does not address it. The gateway's agora-mode work
  does not exercise sessions.
- Direct exposure of the OpenZiti identity surface. The new constructor
  takes an env-root path; identity material is resolved internally,
  matching `app.Run`'s behavior.
- A "daemon connect" path that lets the gateway talk to a running
  `agora network start` daemon via gRPC instead of embedding a runtime.
  The gateway always embeds. Daemon-mode is supported by agora's own
  CLI for operator-facing tasks; it is not a gateway shape.

## Proposed package: `sdk/agent/tunnel`

### Why `tunnel`

The package name mirrors the namespace agora's CLI already uses
(`agora tunnel serve`, `agora tunnel connect`). Reading
`tunnel.EnsureServed(...)` and `tunnel.EnsureConnected(...)` from a
consumer's code is immediately obvious in agora's vocabulary.
"Layer1" would be technically accurate but more obscure; "transport"
collides with the dl/df conventions of the broader ecosystem.

### Package layout

```
sdk/agent/tunnel/
  doc.go            — package-level godoc
  serve.go          — EnsureServed, RemoveServe, ListServes
  connect.go        — EnsureConnected, RemoveConnect, ListConnects
  types.go          — public ServeSpec, ConnectSpec, ServeStatus,
                      ConnectStatus, Mode, State
  errors.go         — ErrNotFound, ErrInvalidSpec, ErrRuntimeMissing,
                      ErrTransient, ErrUnsupportedMode
  serve_test.go     — unit tests against a fake runtime
  connect_test.go   — unit tests against a fake runtime
  example_test.go   — godoc Examples
```

### Public types

```go
package tunnel

import "time"

// Mode is the transport mode a tunnel speaks.
type Mode string

const (
    ModeTCP  Mode = "tcp"
    ModeHTTP Mode = "http"
    ModeUDP  Mode = "udp"
)

// State is the lifecycle state of a serve or connect actor as the
// embedded runtime sees it.
type State string

const (
    StateConfigured State = "configured"
    StateStarting   State = "starting"
    StateRunning    State = "running"
    StateError      State = "error"
    StateStopped    State = "stopped"
)

// ServeSpec describes a tunnel serve the caller wants to ensure
// exists. Identity is by Name (within the caller's account); the
// runtime upserts on Name.
type ServeSpec struct {
    // Name is the service name as advertised to the controller and
    // visible to consumers that dial. Required, non-empty.
    Name string

    // Mode is the transport mode. Required.
    Mode Mode

    // BackendTarget is the local address the runtime forwards to
    // after accepting fabric connections — e.g., "127.0.0.1:8080"
    // for a TCP serve. Required, non-empty.
    BackendTarget string

    // GrantEmails is the optional list of account emails granted
    // access to the served tunnel. The list is **additive**: each
    // entry is added on every EnsureServed call (already-existing
    // grants are ignored without error), but grants from prior calls
    // are not revoked when omitted from a later call. Operators who
    // need to revoke a grant must do so out-of-band via the agora
    // CLI; see "Grant reconciliation" under "What's not in scope."
    GrantEmails []string
}

// ServeStatus reflects the runtime's view of a serve actor.
type ServeStatus struct {
    Name          string
    Mode          Mode
    BackendTarget string
    TunnelID      string
    ServeID       string
    State         State
    LastError     string
    LastStartedAt time.Time // zero if never started
    NextRetryAt   time.Time // zero if not retrying
    RetryAttempt  uint32
}

// ConnectSpec describes a tunnel connect the caller wants to ensure
// exists. Identity is by Name + ListenAddress (within the caller's
// account); the runtime upserts on that key.
type ConnectSpec struct {
    // Name is the service name to dial. Required, non-empty.
    Name string

    // ListenAddress is the local address the runtime binds to and
    // forwards across the fabric — e.g., "127.0.0.1:9080". Required;
    // must be a concrete host:port. Kernel-assigned ports ("…:0")
    // are not supported because the runtime currently does not
    // expose the resolved address back to the caller; pre-allocate
    // a port before calling EnsureConnected if one is needed.
    ListenAddress string
}

// ConnectStatus reflects the runtime's view of a connect actor.
type ConnectStatus struct {
    Name          string
    ListenAddress string // the address passed in ConnectSpec, echoed back
    TunnelID      string
    AttachmentID  string
    State         State
    LastError     string
    LastStartedAt time.Time
    NextRetryAt   time.Time
    RetryAttempt  uint32
}
```

### Public functions

```go
package tunnel

import (
    "context"

    "github.com/openziti/agora/sdk/agent"
)

// EnsureServed ensures a serve actor exists for spec.Name. The runtime
// upserts by Name and reconciles **by restart**: any existing actor
// with the same Name is stopped, and a new one is started from spec.
// Callers should expect a brief connection blink and a fresh ServeID
// on every call. There is no spec input for identifying an existing
// actor to preserve.
//
// Requires the agent to have been constructed with WithRuntime (or
// the standalone-constructor equivalent). Calling without an embedded
// runtime returns ErrRuntimeMissing.
func EnsureServed(ctx context.Context, a *agent.Agent, spec ServeSpec) (*ServeStatus, error)

// RemoveServe removes the serve actor with the given name (matching
// the ServeSpec.Name passed to EnsureServed). Treats "no matching
// actor" as terminal success, so shutdown paths can call it without
// bookkeeping. Works regardless of actor state — including
// StateError, where the runtime-issued ServeID has been cleared.
//
// Cleanup is best-effort on the controller side; see Lifecycle
// semantics for the propagation contract.
func RemoveServe(ctx context.Context, a *agent.Agent, name string) error

// ListServes returns every serve actor the runtime is managing,
// regardless of state. The caller filters by State if a subset is
// wanted.
func ListServes(ctx context.Context, a *agent.Agent) ([]ServeStatus, error)

// EnsureConnected ensures a connect actor exists for the spec's
// Name+ListenAddress key. The caller supplies a concrete listen
// address in the spec; the returned ConnectStatus echoes it.
//
// Reconciliation, runtime requirement, and ID rotation follow the
// same contract as EnsureServed: existing matching actors are
// stopped and replaced on each call; AttachmentID is fresh per call.
func EnsureConnected(ctx context.Context, a *agent.Agent, spec ConnectSpec) (*ConnectStatus, error)

// RemoveConnect removes the connect actor with the given name and
// listen address (matching ConnectSpec.Name + ConnectSpec.ListenAddress
// passed to EnsureConnected). Treats "no matching actor" as terminal
// success. Works regardless of actor state — including StateError,
// where the runtime-issued AttachmentID has been cleared.
//
// Cleanup is best-effort on the controller side; see Lifecycle
// semantics.
func RemoveConnect(ctx context.Context, a *agent.Agent, name, listenAddress string) error

// ListConnects returns every connect actor the runtime is managing.
func ListConnects(ctx context.Context, a *agent.Agent) ([]ConnectStatus, error)
```

### Lifecycle semantics

- The runtime must be **running** at the time of the call (i.e.,
  `Runtime.Start` has been called and `Runtime.Close` has not). All
  six functions return `ErrRuntimeMissing` if the agent has no
  runtime, and surface the underlying error if the runtime is in a
  failed state.
- `EnsureServed` and `EnsureConnected` make **one synchronous
  attempt**. On success they return a `Status` with
  `State == StateRunning`. On first-attempt failure they return an
  `ErrTransient`-wrapped error and the actor is left in `StateError`.
  The runtime's retry loop continues in the background regardless;
  the actor's subsequent state transitions are observable via
  `ListServes` / `ListConnects` but not through the original call's
  return value.
- `RemoveServe` and `RemoveConnect` look up the actor by its
  **stable desired identity** — `Name` for serves, `Name +
  ListenAddress` for connects — which is the same key the runtime
  upserts on. This works for actors in any state, including failure
  states where the ephemeral `ServeID` or `AttachmentID` has been
  cleared by the runtime. The two functions perform **best-effort
  cleanup**: they tear down the local actor's bookkeeping and
  attempt the controller-side deprovision. The local teardown is
  synchronous and always reflected on return. The controller call's
  outcome is not propagated — failures (network error, controller
  unavailable, etc.) are logged at warn level and the function
  returns success. Records that fail to deprovision age out per
  Agora's TTL behavior.
- The actor's heartbeat loop, retry loop, and reconciliation against
  controller state are all handled by the runtime, not by this
  package. The caller does not poll, restart, or otherwise babysit
  actors.

### Error semantics

Sentinel errors with `errors.Is` matching, consistent with
`sdk/agent/catalog`:

```go
var (
    ErrNotFound        = errors.New("tunnel: not found")
    ErrInvalidSpec     = errors.New("tunnel: invalid spec")
    ErrRuntimeMissing  = errors.New("tunnel: agent has no embedded runtime")
    ErrUnsupportedMode = errors.New("tunnel: unsupported mode")
    ErrTransient       = errors.New("tunnel: transient runtime error")
)
```

Mapping from runtime responses:

| Runtime condition                                         | Wrapped error        |
| --------------------------------------------------------- | -------------------- |
| `Agent.Runtime() == nil`                                  | `ErrRuntimeMissing`  |
| `validateDesiredServe` rejects (empty name, etc.)         | `ErrInvalidSpec`     |
| `unsupportedTunnelModeError` from runtime host            | `ErrUnsupportedMode` |
| Controller `StartServe`/`StartConnect` returns error      | `ErrTransient`       |
| Runtime reports `State == Error` after `EnsureServed`     | `ErrTransient` (with last_error in message) |
| `RemoveServe` / `RemoveConnect` finds no actor            | nil (success)        |
| `RemoveServe` / `RemoveConnect` controller deprovision fails | nil (success, logged at warn) |

### Construction notes

The implementation lives entirely inside this repo and is free to
import `internal/api`, `internal/network/tunnelruntime`, and the proto
types. Sketch:

```go
// serve.go
func EnsureServed(ctx context.Context, a *agent.Agent, spec ServeSpec) (*ServeStatus, error) {
    if a == nil {
        return nil, invalidSpec("agent is required")
    }
    runtime := a.Runtime()
    if runtime == nil {
        return nil, fmt.Errorf("tunnel.EnsureServed: %w", ErrRuntimeMissing)
    }
    if err := validateServeSpec(spec); err != nil {
        return nil, err
    }
    req := &networkpb.EnsureServeRequest{
        Name:          spec.Name,
        Mode:          string(spec.Mode),
        BackendTarget: spec.BackendTarget,
        GrantEmails:   spec.GrantEmails,
    }
    res, err := runtime.EnsureServe(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("tunnel.EnsureServed: %v: %w", err, ErrTransient)
    }
    return fromProtoServeStatus(res.Serve), nil
}
```

`validateServeSpec`, `fromProtoServeStatus`, etc. are unexported
translators. External consumers never see `networkpb`.

## Proposed Agent construction additions

The shape: a new `agent.NewStandalone` function plus a `Close` method
on `*Agent`. The function is the analog of `NewEmbedded` for the
runtime — it constructs a fully-populated `*Agent` from explicit
options, leaving lifecycle to the caller.

### Public API

```go
package agent

// StandaloneOptions configures an Agent constructed for external-
// driven lifecycle. The caller is responsible for starting the
// optional embedded runtime, handling signals, and calling Close
// when done.
type StandaloneOptions struct {
    // Name is the agent name. Required, non-empty.
    Name string

    // Description is optional human-readable text.
    Description string

    // EnvRoot is the path to the enrolled environment root.
    // Empty falls back to AGORA_ENV_ROOT, then ~/.agora.
    EnvRoot string

    // WithRuntime causes NewStandalone to construct (but not start)
    // an embedded Layer 1 runtime. The caller starts and stops it
    // via (*Agent).StartRuntime / (*Agent).StopRuntime, or via the
    // tunnel package's lifecycle.
    WithRuntime bool

    // Live mirrors the --live flag's effect for SDK consumers that
    // honor it.
    Live bool
}

// NewStandalone constructs an Agent from explicit options, without
// signal handling, flag parsing, or global logger init. Suitable
// for external Go modules with their own lifecycle (HTTP gateways,
// long-running services with their own CLI).
//
// The caller's responsibilities:
//   - Drive signal handling and pass a cancellable context to any
//     blocking calls.
//   - Call Close when done, to release the embedded runtime and any
//     other agent-held resources.
//   - Provide any logger initialization separately; NewStandalone
//     does not call dl.Init, install signal handlers, or parse
//     os.Args. It does set the environment package's root dir
//     (a process-wide mutation) when EnvRoot is non-empty; this is
//     the only global state it touches.
func NewStandalone(opts StandaloneOptions) (*Agent, error)

// StartRuntime starts the embedded runtime. No-op when the agent
// has no runtime (WithRuntime was false) or when the runtime is
// already started. Returns an error if the runtime has been stopped
// — a stopped agent cannot be revived; construct a new one.
func (a *Agent) StartRuntime(ctx context.Context) error

// StopRuntime stops the embedded runtime. No-op when the agent has
// no runtime, when the runtime was never started, or when it has
// already been stopped. After StopRuntime returns successfully, the
// agent is permanently stopped — StartRuntime will return an error.
// Called automatically by Close if not invoked explicitly.
func (a *Agent) StopRuntime(ctx context.Context) error

// Close releases agent-held resources by calling StopRuntime once.
// Idempotent; safe to defer.
func (a *Agent) Close(ctx context.Context) error
```

### Runtime lifecycle

The agent maintains a small state machine around the embedded runtime,
guarded by an internal mutex. The states are:

```
not-started ──StartRuntime──► started ──StopRuntime──► stopped
                                    └────Close────────►
```

The machine is **one-way**. Once an agent has been stopped, the
underlying `*Runtime`'s `Close` has consumed its `stopOnce` and the
heartbeat / reconciliation loops have torn down. Calling
`StartRuntime` on a stopped agent returns an error rather than
silently restarting — a fresh runtime requires a fresh `*Agent`.

Concrete behavior for each transition attempt:

| Current state | Call          | Effect                                  |
| ------------- | ------------- | --------------------------------------- |
| not-started   | StartRuntime  | calls `Runtime.Start`, → started        |
| started       | StartRuntime  | no-op (idempotent)                      |
| stopped       | StartRuntime  | returns error                           |
| any           | StopRuntime (no runtime) | no-op                        |
| not-started   | StopRuntime   | no-op                                   |
| started       | StopRuntime   | calls `Runtime.Close`, → stopped        |
| stopped       | StopRuntime   | no-op (idempotent)                      |
| any           | Close         | StopRuntime, then returns its result    |

The state guard exists at the agent layer because the underlying
`*Runtime` provides only partial protection: `Runtime.Start` has no
"already started" guard and would re-launch background loops if
called twice, and `Runtime.Close` is one-way via `stopOnce` but
doesn't surface an error to callers that try to restart afterward.
The agent's mutex-protected state field is the single source of
truth for what's valid.

### Why this shape

- `StandaloneOptions` is a struct rather than functional options so
  the public API surface stays small and the field set is obvious at
  call sites. The functional-options pattern (`agent.New(name, ...)`)
  exists for `App`-style consumers; standalone consumers tend to
  build the options from a config struct anyway.
- `StartRuntime`/`StopRuntime`/`Close` are explicit. `app.Run`'s
  implicit lifecycle is the right model for CLI agents but wrong for
  callers that want to control timing.
- `Close` calls `StopRuntime` and returns its result, so a deferred
  close is always correct. The one-way state machine (see Runtime
  lifecycle above) matches the embedded runtime's `stopOnce`
  semantics — there's no Start-after-Close path to support.

### What `NewStandalone` does

1. Resolves the env root (`opts.EnvRoot`, then `AGORA_ENV_ROOT`,
   then `~/.agora`). When the resolved value is non-empty, applies
   it via `environment.SetRootDirName` — this mutates package-level
   state in `environment`, the same call `app.Run` makes for the
   same purpose.
2. Loads the environment root and asserts it's enabled. Returns a
   clear error if not (`"environment is not enabled (run 'agora
   enable <token>' or set --env-root)"`).
3. Builds the controller API client via `root.Client()`.
4. If `WithRuntime`: locates the identity path
   (`root.ZitiIdentityNamed(environmentIdentityName)`), constructs an
   embedded runtime via `NewEmbedded`, **but does not start it.**
   Starting is the caller's responsibility via `StartRuntime`.
5. Returns the populated `*Agent`.

### What `NewStandalone` does NOT do

- No flag parsing.
- No `dl.Init` call.
- No `signal.NotifyContext` install.
- No `os.Args` access.

The one global it does touch is `environment.SetRootDirName` when
`EnvRoot` is non-empty, called out in the procedural description
above. For single-consumer processes (the gateway is the driving
example) this is benign. For multi-tenant processes that need
isolated env roots per agent, a future `environment.LoadRootAt(path)`
addition would let `NewStandalone` honor `EnvRoot` without touching
the package global — straightforward follow-on, out of scope here.

These are explicit non-behaviors, documented in the godoc. A
consumer can call `NewStandalone` from inside another Go program
with no surprises beyond the documented env-root mutation.

## Design considerations

### Why a separate constructor instead of options on `App`

`App` and its options exist because `app.Run` does a lot, and
`Option` functions are how callers customize what it does. Adding
`WithoutSignalHandling`, `WithoutLogInit`, `WithoutFlagParsing` to
`App` would make `app.Run` increasingly conditional — each option
gates a different branch of behavior. The mental model degrades.

`NewStandalone` keeps `App`'s opinionated path opinionated and
provides a separate path for callers that don't want any of it. The
two paths share the underlying `*Agent` type, share the embedded
runtime construction, share `Close` semantics — they diverge only in
how they wire the agent into a process.

### Why the tunnel package mirrors catalog's shape

Consistency of surface. A consumer who learns `catalog.EnsurePublished`
can predict the general shape of `tunnel.EnsureServed`: public types,
sentinel errors, "ensure" verb that takes a spec and returns a status.
The **reconciliation strategy differs** — catalog returns the existing
record on name match (controller records persist indefinitely);
tunnel restarts on match (actors are runtime-scoped, fresh state on
every call). The two strategies match the lifetime model of their
underlying resource; the surface convention is what carries across.
Future public packages (`session`, etc.) should follow the same
surface template, adapting the reconciliation strategy to the
underlying lifetime.

### Why one synchronous attempt, not "block until eventually running"

`EnsureServed` could be specified two other ways: return immediately
with the actor still in `StateStarting`, or block across retries
until the runtime declares success or terminal failure. Neither
fits gateway-style consumers.

Return-immediately makes "did the serve actually come up?" the
caller's problem — startup code has to poll `ListServes` itself,
duplicating the runtime's retry loop.

Block-across-retries hides operational signal. A serve that took 30
seconds of transient errors to come up is a real condition the
caller should see, not paper over.

One synchronous attempt is the honest middle: the caller learns the
first-attempt outcome immediately, can decide whether to abort
startup or tolerate slower convergence, and can observe further
progress via `ListServes` when wanted.

### Why the package depends on `*agent.Agent`

`tunnel.EnsureServed(ctx, a, spec)` takes `*agent.Agent` rather than,
say, `*Runtime` directly because the type is the natural identity for
"the calling agent." Future package functions that need the
controller client (for advertisement linkage, contract resolution,
etc.) would also reach for `*Agent`. Keeping the dependency uniform
across the SDK's public packages avoids the choose-your-handle
dilemma at every call site.

The dependency does imply the caller must have an `*Agent`, which
brings us back to `NewStandalone`. This is the cross-cutting nature
of the gap.

## Module structure

Same as the catalog spec — external consumers depend on
`github.com/openziti/agora` as a normal Go module:

```go
import (
    "github.com/openziti/agora/sdk/agent"
    "github.com/openziti/agora/sdk/agent/catalog"
    "github.com/openziti/agora/sdk/agent/tunnel"
)
```

The future SDK module split is non-breaking from the external-
consumer perspective. The `agent.NewStandalone` constructor and
`Close` method move with the SDK; their signatures don't change.

## What's not in scope

- **Sessions.** `sdk/agent/session/` remains internal-types-leaky.
  When a driving consumer for external session use appears (deep
  integration on gateways, or an external consumer that proposes
  sessions against advertisements), a follow-on spec applies the
  same public-types treatment.
- **Multi-runtime per agent.** Each `*Agent` has at most one
  embedded runtime. Consumers that need multiple isolated runtimes
  on one host today construct multiple agents.
- **Daemon-mode tunnel operations.** `agora network start`'s gRPC
  surface for tunnel control is for operator-facing CLI use; the
  tunnel package targets embedded runtimes only.
- **Tunnel CRUD on the controller without an embedded runtime.**
  The controller's tunnel/serve/attachment REST endpoints exist, but
  there's no use case in scope for external consumers to manipulate
  them without an embedded runtime in the same process.
- **Kernel-assigned connect ports.** `ConnectSpec.ListenAddress`
  requires a concrete host:port; `"…:0"` is not supported because
  the runtime's status snapshot serializes the *desired* listen
  address, not the post-bind resolved one. Adding support is a
  straightforward follow-on: a `ResolvedListenAddress` field on
  `ManagedConnectStatus` plus runtime state to capture it after
  binding. Out of scope here.
- **Grant reconciliation.** `EnsureServed` is additive on
  `GrantEmails` — it adds entries but never revokes. Full
  reconciliation (list existing → diff against spec → add new,
  revoke removed) would close the gap, but depends on a revoke
  primitive on the controller/runtime path. Out of scope here.
  Operators who need to revoke a grant use the agora CLI in the
  meantime.
- **Strong-cleanup Remove semantics.** `RemoveServe` and
  `RemoveConnect` are best-effort on the controller side — failures
  to deprovision are logged but not propagated to the caller. A
  stronger contract that surfaces controller-cleanup errors is
  viable as a follow-on (the runtime's stop helpers need to return
  errors instead of logging them), but doesn't fit the current
  runtime and isn't required by the driving consumer's shutdown
  tolerance.

## Acceptance criteria

1. `sdk/agent/tunnel/` exists, exports the types and functions
   defined above, and depends only on `sdk/agent/networkpb` and
   internal packages (no `internal/api` types in public signatures).
2. `agent.NewStandalone`, `(*Agent).StartRuntime`,
   `(*Agent).StopRuntime`, `(*Agent).Close` exist and are documented.
3. A unit test verifies the external-style consumer pattern: build
   an `*Agent` via `NewStandalone(WithRuntime: true)`, call
   `StartRuntime`, call `tunnel.EnsureServed` against a fake
   controller, call `Close`. No `app.Run`, no signal handlers, no
   `dl.Init` call from the SDK.
4. A second test verifies that `tunnel.EnsureServed` and
   `tunnel.EnsureConnected` return `ErrRuntimeMissing` when called
   against an agent constructed without `WithRuntime`.
5. Error tests cover the full mapping table in §Error semantics,
   with `errors.Is` matching the documented sentinels.
6. Godoc on every exported symbol, with at least one runnable
   `Example` on `EnsureServed` and one on `NewStandalone`.
7. `docs/current/sdk/overview.md`'s "When the SDK does not yet suffice"
   section is updated to retire the bullets this spec addresses and
   to mention `sdk/agent/tunnel` and `NewStandalone`.

## Related documentation

- [`external-consumers.md`](./external-consumers.md) — the catalog
  spec; the same template applies here
- [`../../current/sdk/overview.md`](../../current/sdk/overview.md) — the SDK's shape; this spec
  expands its "When the SDK does not yet suffice" section
- [`../../current/layer-1/agent.md`](../../current/layer-1/agent.md) — Layer 1 runtime
  model and isolation
- [`llm-gateway/docs/future/agora.md`](https://github.com/openziti/llm-gateway/blob/main/docs/future/agora.md)
  — the driving consumer's requirements
- [`sdk/agent/networkpb/network.proto`](../../../sdk/agent/networkpb/network.proto)
  — the proto source of truth for the shapes this spec wraps
