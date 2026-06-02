# Go-Native Layer 1 Primitives for the Agent SDK

This is a spec for a public SDK surface that lets an embedded agent own its
application protocol — server or client — while using Agora Layer 1 as the
transport. It originates from an external request: infrastructure like
`mcp-gateway` already owns an HTTP server, and it wants to serve that server
directly on an Agora/OpenZiti listener, the same way it can serve directly on a
zrok listener. The consumer-side mirror is an HTTP client that dials the overlay
as a `net.Conn` without standing up a local proxy port.

The thesis is small, and worth stating plainly before any of the detail: the
hard machinery already exists. `internal/network/tunnelruntime` already turns an
OpenZiti context into `Listen(serviceName) net.Listener` and
`Dial(serviceName) net.Conn`. What's missing is not transport. It's a *public
lifecycle wrapper* — something that creates the controller-side record,
heartbeats it, and hands the raw Go object out to the caller, without the local
proxy loop that the existing managed APIs wrap around it. The whole of this work
is exposing what's already there, with the lifecycle guarantees intact and the
internals still hidden.

## What exists today

The public Layer 1 surface in `sdk/agent/tunnel` is actor-oriented, and it is
built for a local-proxy world:

- `EnsureServed` starts a managed serve actor that accepts overlay traffic and
  forwards it to a local `BackendTarget`.
- `EnsureConnected` starts a managed connect actor that binds a local
  `ListenAddress` and proxies accepted local connections out over the overlay.

Both are the right tool for CLI and daemon-style workflows, where the agent's
job is to bridge the overlay to a port. Neither helps a process that already
*is* the server. `EnsureServed` insists on forwarding to a backend, which means
an embedded HTTP server has to allocate a throwaway loopback listener, hand its
address to `BackendTarget`, and then accept its own traffic back over a needless
hop. That hop is the thing this work removes.

Underneath, the raw primitives are already shaped the way an embedded consumer
wants them. `OverlayContext` (`internal/network/tunnelruntime/overlay.go`)
exposes exactly:

```go
type OverlayContext interface {
    Listen(serviceName string) (net.Listener, error)
    Dial(serviceName string) (net.Conn, error)
}
```

`Listen` returns a real `net.Listener` whose `Accept()` yields overlay
connections; `Dial` returns a real `net.Conn`. The managed actors don't hand
these out — they consume them inside a proxy loop. The work here is to build a
second path that creates the same controller lifecycle around `Listen`/`Dial`
but returns the object instead of proxying it.

These two new entry points form a symmetric pair: `OpenListener` on the provider
side, `OpenDialer` on the consumer side. Both *open* a managed Layer 1 object
and return it. The provider side is specified for near-term implementation. The
consumer side is fully designed here but is phase-2 work; the reasoning for the
split is in its own section.

## Provider side: `OpenListener`

The embedded process should be able to write this:

```go
listener, handle, err := tunnel.OpenListener(ctx, a, tunnel.ListenSpec{
    Name:        "mcp-gateway",
    Mode:        tunnel.ModeHTTP,
    GrantEmails: []string{"consumer@example.com"},
})
if err != nil {
    return err
}
defer handle.Close()

return httpServer.Serve(listener)
```

`OpenListener` returns three things: a `net.Listener`, a `*ServeHandle`, and an
error. The listener is the overlay listener — the caller passes it straight to
its own server and owns protocol serving from there. There is no `BackendTarget`,
because there is no backend; the process serving the listener *is* the backend.

### Why the listener and the handle are separate objects

The split is deliberate, and it's load-bearing for resilience. The handle owns
the *controller lifecycle*: it created the serve record, it runs the heartbeat,
and `handle.Close()` is what decommissions the record and stops the heartbeat.
The listener owns a single *overlay listen session*. Keeping them as two objects
means a caller can drop and replace the active listener after a transport flap
without tearing down — and re-establishing — the controller-side serve record
and its grants underneath.

It also avoids a footgun. `http.Server.Serve(listener)` closes the listener when
it returns. If the listener and the lifecycle were one object, an ordinary server
shutdown would also silently decommission the controller record. Separating them
keeps `listener.Close()` meaning "end this listen session" and `handle.Close()`
meaning "I'm done serving this name entirely."

`handle` exposes `Close()` and `Status()`. `Status()` reports
diagnostics-grade state — enough to answer "is this serve healthy, and if not,
why" — and reuses the existing `ServeStatus` shape, with the retry-related fields
inert (see the failure model).

### Mode is metadata here, and the path is stream-only

In the managed world, `ModeHTTP` means "wrap the overlay listener in a reverse
proxy to the backend." `OpenListener` does no wrapping — the caller's server
reads bytes off the listener directly, and at the `net.Conn` level those bytes
are opaque. So `Mode` does not change listener behavior. It survives as
service/intercept metadata: it tells the controller how to configure the
service, and it's the contract the paired dialer has to agree with. An HTTP
listener and an HTTP dialer are two halves of the same agreement; `Mode` is where
that agreement is recorded.

Because a `net.Listener` is stream-shaped, the `OpenListener` path supports the
stream modes — `tcp` and `http` — and rejects `udp`. UDP over the overlay is
packet-oriented and does not fit a `net.Listener`; a packet-conn variant is out
of scope here and noted in Deferred.

### The failure model: one observable, caller owns resilience

This is the sharpest inversion from the managed actors, so it's worth being
exact about.

A managed serve actor *hides* transport flaps. When its overlay listen drops, it
retries with backoff, and the caller never sees it — resilience is the actor's
job. `OpenListener` does the opposite, because the caller now owns a real
`net.Listener` and is the only party that can decide what to do when it fails.

So the direct handle is, in effect, a serve actor with *internal retry disabled
and heartbeat kept*. And every way the Layer 1 actor can fail converges to a
single observable:

- the overlay listen session drops, **or**
- the controller serve record is lost (a heartbeat comes back permanently
  rejected — the record is gone),

…and in either case the listener is closed and `Accept()` returns an error, and
the handle moves to its error state. The caller's entire resilience contract is
one line: *if `Accept()` returns an error, tear down and reopen.* A caller that
wants to relist-and-swap writes the obvious loop; a caller that wants to crash
and let a supervisor restart it gets that for free. Agora does not decide the
policy — it surfaces the failure cleanly and lets the embedding process decide.

This is also why `Status()`'s retry fields are inert: there is no internal retry
to report on.

## Consumer side: `OpenDialer` (phase 2)

The consumer mirror lets an embedded HTTP client dial the overlay directly:

```go
dialer, err := tunnel.OpenDialer(ctx, a, tunnel.DialSpec{
    Name: "mcp-gateway",
})
if err != nil {
    return err
}
defer dialer.Close()

conn, err := dialer.DialContext(ctx)
```

`DialContext` is shaped to drop straight into an `http.Transport.DialContext`
field, so an embedded HTTP client speaks to an overlay service with no local
listener and no proxy hop. Unlike the provider side, the dialer is a single
object: a dialer naturally owns both its lifecycle (`Close()`) and its action
(`DialContext`), so there's no listener/handle split to make.

### `listenAddress` is not load-bearing for dialing

The original request worried that a direct dialer needs a Layer 1 schema change,
because the connect request and the attachment record both require
`listenAddress`, which a `net.Conn` dialer doesn't have. Reading the controller
shows the constraint is softer than it looks.

`ConnectTunnel` (`internal/controller/connectTunnel.go`) does three things with a
connect request, and `listenAddress` is load-bearing for none of them.
Authorization runs through `canConnectToTunnel`, which never sees the address.
The actual dial grant — the OpenZiti dial policy that permits this environment's
identity to reach the service — is built by `CreateAttachmentDialPolicy` from the
org, account, environment, tunnel, service, and identity IDs; it never reads
`listenAddress` either. The field's only role is to be stored on the attachment
record, echoed back, and logged. It describes the local proxy's bind port, which
exists only because today every consumer *is* a local proxy.

So a direct dialer keeps everything the attachment record is actually for —
authorization, heartbeat liveness, and observability — and lacks only a thing
the record was never using to dial.

### A reserved sentinel, not a schema change

Because the controller doesn't read `listenAddress`, the dialer doesn't need the
schema to change at all. It sends a *reserved* `listenAddress` value that means
"no local listener," reuses the existing connect path verbatim, gets back an
authorized, heartbeating attachment, and calls `Dial` directly. No migration, no
relaxing the `NOT NULL` and non-empty constraints, no OpenAPI contract change, no
new controller branch. The sentinel becomes the single point that says "this
attachment has no local listener."

Two guardrails keep that honest:

- **The token must obviously not be an address.** Real listen addresses are
  `host:port`. The reserved value must be something that can never be a valid one
  — scheme-like (`dialer://`) or colon-less — so it can never collide with a real
  address and never reads in the DB or logs as a misconfigured port. The exact
  string is an implementation choice; this spec reserves the *concept*.
- **One place interprets it.** A single `isDirectDialer(listenAddress)` helper,
  not scattered string comparisons. Display surfaces — the dashboard, the CLI
  list output — learn to render a direct-dialer attachment as "direct dialer"
  rather than printing the raw token.

The honest tradeoff, named rather than buried: this is in-band signaling in a
typed field — a string carrying two meanings. It's a common and pragmatic move
(port `0` meaning "any port," `-` meaning stdin in CLIs), and it's the right call
*now* because the attachment record is young and a migration isn't worth spending
to prove the dialer out. If direct dialing earns first-class status, the sentinel
can be promoted to a properly modeled listener-less attachment later; nothing
else depends on the field, so that door stays open.

### Why phase 2

The provider listener unblocks the motivating case — embedded gateway serving
parity with zrok — and depends on nothing new. The dialer's design is settled
here, but it's a separate, smaller body of implementation work (a new public
type, the sentinel, the display handling), and it earns its own slice. Until it
lands, `EnsureConnected` remains the supported public consumer-side concept for
anything that can live with a local listener.

## Required semantics

The new listener and dialer handles must preserve the same Layer 1 lifecycle
guarantees as the managed serve/connect actors. Concretely, each handle must:

- create the controller-side serve or attachment record on open,
- start and maintain heartbeats for the life of the handle,
- expose status sufficient for diagnostics,
- close the underlying listener or dialer when the Layer 1 actor fails — the
  failure model above is the provider-side instance of this,
- remove the runtime actor and the controller record when the handle is closed,
- never expose `internal/network/tunnelruntime` to SDK consumers.

The difference between these handles and the managed actors is precisely one of
policy, not of guarantees: the handle disables internal retry and surfaces
failure, where the actor hides it. Everything else — record creation, heartbeat,
clean teardown — is identical, and should be built by reusing the same controller
lifecycle calls the actors already drive (`StartServe` / `HeartbeatServe` /
`StopServe` and their attachment equivalents), minus the retry wrapper.

## Scenarios

**An embedded gateway serves HTTP directly.** `mcp-gateway` calls `OpenListener`
with `Mode: ModeHTTP` and its grant list, gets back a `net.Listener`, and runs
`httpServer.Serve(listener)`. No loopback listener, no `BackendTarget`, no extra
hop. When the process shuts the server down, `handle.Close()` decommissions the
serve record.

**A server rides out a transport flap.** The overlay listen drops; `Accept()`
returns an error. The gateway's serve loop treats that as its cue: it closes the
old listener, calls `OpenListener` again to get a fresh one, and resumes serving
— its own resilience policy, expressed in its own code, because Agora handed it
the failure instead of hiding it. The controller serve record and its grants are
untouched across the swap if the caller keeps the handle.

**An embedded client dials over the overlay (phase 2).** A process builds an
`http.Client` whose `Transport.DialContext` is the dialer's `DialContext`. Every
request the client makes dials the named overlay service directly as a
`net.Conn`. There is no local port, and the attachment that authorizes the dials
is identical to a proxy attachment but for its reserved `listenAddress`.

## Deferred (and Why)

- **The dialer implementation.** Design is settled in this spec; the
  implementation is phase-2 work, sequenced behind the provider listener because
  the listener unblocks the motivating case and the dialer is an independent
  slice.
- **UDP on the raw listener.** A `net.Listener` is stream-shaped and can't carry
  packet semantics. A packet-conn variant for UDP overlay traffic is a separate
  design; the stream path (`tcp`, `http`) covers the motivating cases.
- **Relisten-reuse as an optimization.** The baseline resilience story is
  close-the-handle-and-reopen. Keeping a single controller serve record alive
  across multiple overlay listen sessions — so a flap re-lists without re-running
  record creation and grants — is a real optimization, but it adds lifecycle
  surface and isn't needed to ship. Deferred until the simple model shows it's
  worth it.
- **Promoting the dialer sentinel to a modeled attachment.** If direct dialing
  becomes a common, first-class pattern, the reserved-`listenAddress` sentinel
  should be promoted to a properly modeled listener-less attachment (nullable
  field or a distinct variant). Deferred because the sentinel ships the dialer
  with zero schema cost and nothing else depends on the field.
- **Deeper status and metrics.** `Status()` is scoped to diagnostics. Richer
  per-handle metrics and limits ride the broader post-MVP metrics work rather
  than this surface.

## Non-Goals

- Do not require SDK consumers to import Agora internal packages —
  `internal/network/tunnelruntime` stays hidden.
- Do not remove or weaken `EnsureServed` or `EnsureConnected`; they remain the
  high-level local-proxy APIs for CLI and daemon-style workflows.
- Do not make embedded applications run the external Agora daemon just to use the
  listener or dialer APIs.
- Do not fake direct support by forcing consumers through an unnecessary loopback
  proxy hop. The point of this work is to remove that hop, not to hide it.
