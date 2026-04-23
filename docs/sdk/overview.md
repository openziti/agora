# Agora SDK Overview

The Agora SDK lives at `sdk/agent/` in this repository. It is the supported integration path for building agent processes that participate in a zero-trust Agora network — whether a Layer 2 collaboration participant or any other native consumer of Agora's primitives.

The SDK is also consumed by Agora's own `agora network start` daemon. The daemon and every Macro Pulse example agent use the same runtime code, exposed through complementary construction paths.

## Package layout

```
sdk/agent/
├── doc.go               package-level godoc
├── agent.go             Runtime type and daemon-mode constructor (NewDaemon)
├── app.go               App / Agent (SDK handle) / options / Run(fn)
├── embedded.go          NewEmbedded / Start / Close
├── state.go             in-memory state helpers
├── heartbeat/*.go       environment heartbeat (heartbeat loop, state machine)
├── tunnel_runtime.go    serve/connect lifecycle
├── tunnel_state.go      tunnel runtime state transitions
├── controller_client.go controller HTTP interactions
├── runtime_host.go      HTTP/TCP/UDP tunnel runtime host abstraction
└── pb/                  generated gRPC/protobuf types (daemon surface)
```

Adjacent, daemon-specific code lives separately at `internal/network/daemon/` (CLI-side client helpers such as `Dial`, `Ping`, `Stop`).

## Three construction paths

The SDK exposes three ways to construct a runtime, matched to three deployment shapes:

| Constructor | Use case |
| --- | --- |
| `agent.NewDaemon()` | The standalone `agora network start` daemon. Loads `~/.agora/network.json`, binds UDS, serves gRPC. |
| `agent.NewEmbedded(opts)` | Low-level embedded use. Caller provides explicit environment, identity path, and optional timing. No disk I/O; no UDS. |
| `agent.New(name, opts...)` plus `app.Run(fn)` | The SDK app pattern. Flag parsing, env loading, optional embedded runtime construction (via `WithRuntime()`), signal-handled lifecycle, clean shutdown. The recommended path for Layer 2 agents. |

Most agent authors want path 3. The Macro Pulse example agents all use it.

## Typical agent shape

```go
package main

import (
    "context"
    "flag"
    "os"

    "github.com/openziti/agora/sdk/agent"
)

func main() {
    fs := flag.NewFlagSet("my-agent", flag.ExitOnError)
    // ... agent-specific flags ...

    app := agent.New("my-agent",
        agent.WithDescription("What this agent does"),
        agent.WithFlagSet(fs),
        agent.WithRuntime(),
    )
    if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
        a.Log().Info("alive")
        <-ctx.Done()
        return nil
    }); err != nil {
        os.Exit(1)
    }
}
```

Omit `WithRuntime()` for agents that only consume the controller API and do not participate in sessions.

## Built-in flags

`(*App).Run` adds these flags to every agent, on top of any flags the user supplies via `WithFlagSet`:

- `--env-root <path>` — override the environment root (default: `AGORA_ENV_ROOT` env var, then `~/.agora`)
- `--log-level <level>` — `debug` | `info` | `warn` | `error` (default: `AGORA_LOG_LEVEL` or `info`)
- `-v` — shorthand for `--log-level=debug`
- `--live` — opt in to live external data sources (provider-specific; ignored by consumers)

Environment variable overrides match the flag names: `AGORA_ENV_ROOT`, `AGORA_LOG_LEVEL`.

## Agent handle API

The `*Agent` passed to the business-logic function exposes:

- `Name() string` — the agent name from `New`
- `EnvRoot() env_core.Root` — the loaded local environment
- `Environment() *env_core.Environment` — enrolled environment record (account token, environment ID)
- `AccountID() string` — the enrolled environment ID (identifies this agent instance within its org)
- `Controller() *api.Client` — a controller API client authenticated with the account token
- `Runtime() *Runtime` — the embedded Layer 1 runtime, or nil if `WithRuntime()` was not set
- `Log() *dl.Builder` — a df/dl structured-log builder with `agent` and `environment_id` fields pre-populated
- `Live() bool` — the `--live` flag value

## Isolation from the standalone daemon

Embedded runtimes are isolated from any `agora network start` daemon running on the same host. See [`docs/layer-1/agent.md`](../layer-1/agent.md) "Packaging Direction" for the full model. Summary:

- Embedded runtimes do **not** read or write `~/.agora/network.json`.
- Embedded runtimes do **not** bind `~/.agora/network.sock`.
- Serve and attachment records each runtime creates against the controller are distinct; neither runtime can see or modify the other's.
- Both share the enrolled environment identity (account token, identity file).

This means an operator can run `agora network start` for CLI-driven tunnel operations on the same host as one or more embedded-runtime agents, and the two will not collide.

## When the SDK does not yet suffice

- **Public-module consumption.** The SDK lives inside the main `github.com/openziti/agora` Go module. Agents in this repo (including the Macro Pulse examples) can import `github.com/openziti/agora/sdk/agent` directly. External projects that want to depend on the SDK without vendoring the full Agora source tree will need a future split of the SDK into its own module; not in scope for MVP.
- **Advertisement / session / contract / envelope APIs.** Not part of the SDK yet. They land as each Layer 2 slice ships. See [`docs/layer-2/status.md`](../layer-2/status.md) for the slice order and [`docs/examples/macro-pulse.md`](../examples/macro-pulse.md) "Slice Rollout" for which agent behavior lands alongside each.
- **Discovery by non-capability properties.** Catalog search surfaces are deferred to the catalog/advertisement slice.

## Related documentation

- [`docs/layer-1/agent.md`](../layer-1/agent.md) — runtime model, ownership boundaries, packaging direction, isolation model
- [`docs/layer-2/foundation.md`](../layer-2/foundation.md) — Layer 2 cross-cutting decisions that the SDK's capability surface will grow to reflect
- [`docs/examples/macro-pulse.md`](../examples/macro-pulse.md) — the primary reference consumer of the SDK
