// Package agent is Agora's SDK for building agent processes that
// participate in a zero-trust Agora overlay network.
//
// An agent built on this SDK has:
//
//   - an enrolled environment identity (account token, OpenZiti
//     identity material), loaded from ~/.agora or an overridden path
//   - a controller API client authenticated against that identity
//   - optionally, an embedded Layer 1 network runtime that hosts
//     serves and connects in-process without a separate daemon
//   - a lifecycle tied to SIGINT/SIGTERM with clean runtime teardown
//
// # Relationship to the standalone daemon
//
// The same runtime code serves two consumers:
//
//   - `agora network start` stands up the runtime as a standalone
//     daemon with ~/.agora/network.json desired-state persistence and
//     a gRPC+UDS control surface at ~/.agora/network.sock. CLI
//     commands like `agora tunnel serve` delegate to this daemon.
//   - SDK consumers embed the runtime in-process via NewEmbedded.
//     Embedded runtimes do not touch network.json and do not bind the
//     UDS socket; they are isolated from any daemon on the same host,
//     sharing only the enrolled environment identity.
//
// See docs/layer-1/agent.md "Packaging Direction" for the full
// isolation model.
//
// # Types
//
// The SDK exposes three main types:
//
//   - App is the configuration + lifecycle entrypoint. Constructed
//     via New(name, opts...); driven by (*App).Run(fn).
//   - Agent is the handle passed to an agent's business-logic
//     function. It exposes Env, Controller, Runtime (if enabled),
//     and a pre-scoped Log builder.
//   - Runtime is the embedded Layer 1 network runtime. SDK consumers
//     typically receive it via Agent.Runtime() rather than
//     constructing it directly; advanced callers can call
//     NewEmbedded for full control.
//
// # Typical usage
//
//	package main
//
//	import (
//	    "context"
//	    "os"
//
//	    "github.com/openziti/agora/sdk/agent"
//	)
//
//	func main() {
//	    app := agent.New("my-agent",
//	        agent.WithDescription("Example agent"),
//	        agent.WithRuntime(),
//	    )
//	    if err := app.Run(func(ctx context.Context, a *agent.Agent) error {
//	        a.Log().Info("alive")
//	        <-ctx.Done()
//	        return nil
//	    }); err != nil {
//	        os.Exit(1)
//	    }
//	}
//
// Agents that only consume the controller API (for example, admin
// utilities) omit WithRuntime. Agents that accept or open sessions
// need it.
package agent
