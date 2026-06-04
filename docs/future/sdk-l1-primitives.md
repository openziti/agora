# Go-Native Layer 1 Primitives for the Agent SDK

This spec re-introduces basic `Listen` and `Dial` primitives into the Agora SDK, paralleling zrok's `NewListener` / `NewDialer`. They let an embedded agent own its application protocol — server or client — while using Agora Layer 1 as the transport, by handing the caller a raw Go `net.Listener` or `net.Conn` over the overlay, with no local proxy hop. The motivating case is `mcp-gateway`: it already owns an HTTP server and wants to serve that server directly on an Agora listener, the same way it can serve directly on a zrok listener. The consumer-side mirror is an HTTP client that dials the overlay as a `net.Conn` without standing up a local proxy port.

The thesis is small, and worth stating plainly: the machinery already exists. `internal/network/tunnelruntime` already turns an OpenZiti context into `Listen(serviceName) net.Listener` and `Dial(serviceName) net.Conn`. The work is to expose those thinly through `sdk/agent/tunnel` — and, like zrok, to keep that exposure *thin*: the primitive returns the raw overlay object and nothing more. Provisioning the controller-side record and its grants is a separate, explicit step, exactly as zrok separates `CreateShare` from `NewListener`. The complexity that doesn't belong in a primitive — heartbeats, failure policies, managed status — stays out of it.

## The zrok parallel

zrok's whole provider-side flow is three steps, and the listener is the thinnest of them:

```go
shr, err := sdk.CreateShare(root, &sdk.ShareRequest{...})  // 1. provision the controller record
defer sdk.DeleteShare(root, shr)                           //    explicit teardown

conn, err := sdk.NewListener(shr.Token, root)             // 2. thin primitive: the overlay net.Listener
defer conn.Close()

http.Serve(conn, nil)                                      // 3. serve
```

`NewListener` itself is eleven lines: load the identity, build a ziti context, call `ListenWithOptions`, return the raw `edge.Listener`. No controller record, no heartbeat, no policy, no status. `NewDialer` is the same shape. The discipline that keeps it simple is the **separation**: `CreateShare` / `DeleteShare` own the controller record's lifecycle (explicit create, explicit delete, persists in between); `NewListener` / `NewDialer` just bind or dial the overlay. Agora's `Listen` / `Dial` follow the same split.

## What exists today in Agora

The public Layer 1 surface in `sdk/agent/tunnel` is actor-oriented and built for a local-proxy world:

- `EnsureServed` starts a managed serve actor that accepts overlay traffic and forwards it to a local `BackendTarget` — heartbeated, retrying, status-reporting.
- `EnsureConnected` starts a managed connect actor that binds a local `ListenAddress` and proxies accepted local connections out over the overlay.

Both are the right tool for CLI and daemon-style workflows that want a managed, supervised bridge. Neither helps a process that already *is* the server (or the client): `EnsureServed` insists on forwarding to a backend, so an embedded HTTP server has to allocate a throwaway loopback listener and accept its own traffic back over a needless hop. And neither is *thin* — both wrap the overlay in a heartbeated actor and a proxy loop.

Underneath, the raw primitives are already shaped the way an embedded consumer wants them. `OverlayContext` (`internal/network/tunnelruntime/overlay.go`) exposes exactly:

```go
type OverlayContext interface {
    Listen(serviceName string) (net.Listener, error)
    Dial(serviceName string) (net.Conn, error)
}
```

`Listen` returns a real `net.Listener` whose `Accept()` yields overlay connections; `Dial` returns a real `net.Conn`. The managed actors consume these inside their proxy loops rather than handing them out. The work here is the thin public path that returns the object directly.

The new entry points are `Listen` on the provider side and `Dial` on the consumer side. `EnsureServed` / `EnsureConnected` are untouched and remain the managed option for anyone who wants the heartbeated, supervised lifecycle. `Listen` is specified for near-term implementation; `Dial` is designed here but is phase-2 work (its one prerequisite is in its own section).

## Why thin is the whole point

It's worth being explicit, because an earlier draft went the other way. The temptation is to make `Listen` *also* own the controller record — create it, heartbeat it, expose its status, tear it down — so the one call does everything. That merge is exactly what produces unbounded complexity: once a primitive owns a heartbeated record, it needs a policy for what to do when heartbeats fail, a story for partitions and stale-reaping, a draining state for graceful shutdown, a status surface, and an atomic teardown contract. None of that is transport; all of it is lifecycle the application didn't ask the *primitive* to manage.

zrok avoids every bit of it by keeping provisioning separate. Agora can too, and Agora's own controller design makes it natural — as the next sections show, a provider's authorization survives without any heartbeat at all.

## Provider side: `Listen`

The embedded process should be able to write this — the same three-step shape as the zrok example:

```go
// 1. provision a direct service (persists until deleted)
svc, err := tunnel.CreateService(ctx, a, tunnel.ServiceSpec{
    Name:        "mcp-gateway",
    Mode:        tunnel.ModeHTTP,
    GrantEmails: []string{"consumer@example.com"},
})
if err != nil {
    return err
}
defer tunnel.DeleteService(ctx, a, svc)

// 2. thin primitive: the overlay net.Listener
listener, err := tunnel.Listen(ctx, a, svc.Name)
if err != nil {
    return err
}
defer listener.Close()

// 3. serve — the process owns its protocol
return httpServer.Serve(listener)
```

`Listen` returns a `net.Listener` and an error — nothing else. The listener is the overlay listener; the caller passes it straight to its own server and owns protocol serving. There is no `BackendTarget`, because there is no backend: the process serving the listener *is* the backend. There is no handle, no `Status()`, no heartbeat policy — the primitive is the thing zrok's `NewListener` is.

Provisioning is the separate `CreateService` / `DeleteService` step (names are illustrative; the point is the split). It creates a direct tunnel — the Ziti service, the bind policy, and the grants — and that record persists until `DeleteService`, the same as a zrok share persists until `DeleteShare`.

### The provider needs no heartbeat

This is what makes a thin listener correct rather than just convenient, and it falls out of Agora's existing controller design. The bind policy that authorizes an overlay `Listen` lives on the **tunnel**, created at tunnel-creation; the serve-side reaper (`tunnelServeReaper.go`) only ever marks a non-heartbeated serve record `stale` — a state change — and never removes a policy. So a direct tunnel's authorization survives indefinitely without any heartbeat: `Listen` keeps working until the tunnel is explicitly deleted. The primitive doesn't heartbeat, and doesn't need to.

The practical consequence is that the entire control-plane failure model an earlier draft built — `OnHeartbeatLoss` policies, draining states, stale-reap reasoning, partition semantics, a managed status surface — simply doesn't apply. There is no heartbeat to lose.

### The failure model, in full

For a thin listener it's one line: **the overlay errors, `Accept()` returns an error, the application decides what to do.** Relisten, crash and let a supervisor restart, surface it — the SDK hands over the failure exactly as a `net.Listener` does and gets out of the way. That's the whole contract, and it's the same one zrok's `edge.Listener` gives.

Two clarifications sit under that line:

- **Revocation is the overlay's job, not the SDK's.** To stop serving, delete the tunnel (or pull a grant) at the controller; that removes the Ziti policy, and OpenZiti terminates established sessions when the authorizing policy is removed — it drops live circuits, not just new ones. The SDK never tracks or force-closes connections; a malicious or buggy endpoint could ignore any such signal, so endpoint-side enforcement would be theater. Enforcement lives at the fabric. (This is a normative Layer 1 dependency: the active-stream cut on revocation rests on that OpenZiti behavior.)
- **Connections already accepted are the application's.** Closing the listener ends new `Accept()`s; the streams the app already accepted are its own to drain or close. A graceful shutdown is the app's `httpServer.Shutdown(ctx)`, and `listener.Close()` is just closing the overlay listener — there is no record being torn down underneath it.

### The direct tunnel record has no backend

A direct tunnel still needs a record — it carries the service identity, the Ziti service, the bind policy, and the grants. But the tunnel today requires a non-empty `backend_target` (`not null`, plus a non-empty check), because every serve so far has been a managed proxy forwarding to a local address. A direct tunnel has no such target, so the record grows a discriminator: a `NOT NULL` `kind` column (`proxy` | `direct`, defaulted to `proxy`), with `backend_target` made nullable under a conditional check — required for a `proxy` tunnel, null for a `direct` one. The `NOT NULL` on `kind` is load-bearing: a Postgres `CHECK` passes a NULL `kind` as UNKNOWN, so without it a null-kind row would evade both the value check and the conditional one. The migration is additive and non-destructive: add the defaulted `kind` column, constrain it to its values (mirroring the existing `state in (...)` guard), drop `backend_target`'s `NOT NULL`, swap its non-empty check for the conditional one.

Kind is inferred at the edge: a create request carrying a `backendTarget` is a managed-proxy tunnel; one that omits it is a direct tunnel. The presence rule is exact, since generated types represent absence several ways — absent or null means `direct`, a trimmed non-empty value means `proxy`, and a present-but-empty or whitespace-only value is rejected rather than coerced. A service name is one kind: `EnsureServed` and `Listen`'s provisioning on the same name conflict rather than silently sharing a record. Display surfaces render a direct tunnel as "direct listener" wherever a proxy tunnel shows its `backend_target`.

### Mode is metadata, and the path is stream-only

`Mode` is a provisioning-time attribute of the service, not behavior of the primitive. In the managed world `ModeHTTP` means "wrap the overlay listener in a reverse proxy"; `Listen` does no wrapping — the caller's server reads bytes off the listener directly. So `Mode` survives only as service metadata that tells the controller how to configure the Ziti service; it is not a handshake the two ends negotiate, and matching protocols across a listener/dialer pair is the application's responsibility. Because a `net.Listener` is stream-shaped, the `Listen` path supports the stream modes (`tcp`, `http`) and rejects `udp`; a packet-conn variant is out of scope (see Deferred).

## Consumer side: `Dial` (phase 2)

The consumer mirror lets an embedded HTTP client dial the overlay directly:

```go
dialer, err := tunnel.Dial(ctx, a, "mcp-gateway") // returns a net.Conn-yielding dialer
if err != nil {
    return err
}
defer dialer.Close()

client := &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}
```

`Dial` is the thin consumer primitive: it dials the named overlay service and yields a raw stream-oriented `net.Conn`. `DialContext` carries the exact `http.Transport.DialContext` signature and ignores `network`/`addr` (the target is fixed by the service name), so it drops straight into a transport. Because `addr` is ignored, a transport wired to it should be dedicated to the named service — every request routes there regardless of URL host.

### Making the dialer thin: a reaper adjustment

The consumer side has one wrinkle the provider side doesn't, and closing it is a small Layer 1 change. Today the attachment reaper (`tunnelAttachmentReaper.go`) *removes the dial policy* 45s after the last heartbeat — so a dial authorization currently requires ongoing heartbeat, which a thin primitive shouldn't have to send. The fix is to make the attachment reaper behave like the serve reaper already does: mark a non-heartbeated attachment `stale` (a state change for observability) **without removing its dial policy**. With that change, a direct dialer's authorization persists until the attachment is explicitly deleted — symmetric with the provider side — and `Dial` needs no heartbeat. Truly-abandoned dial grants are then cleaned up the same way abandoned serves and tunnels are: by explicit delete, or when the environment itself goes away. This reaper adjustment is the phase-2 prerequisite that makes `Dial` as thin as `Listen`.

With it in place, the dialer's failure model is the listener's: a per-dial error (no route, remote down, canceled request) fails that `DialContext` call and returns to its caller; the dial stops working entirely only when its authorization is deleted (revocation), which the overlay enforces. Cancellation has its own prerequisite: `ctx` must propagate to the underlying dial so a stalled dial never outlives its request — and today's internal primitive is the contextless `OverlayContext.Dial`, so phase 2 must first add a genuinely cancellable internal dial that aborts the OpenZiti attempt and disposes of any late connection.

### `listenAddress` is not load-bearing, so a dialer attachment needs none

The original request worried that a direct dialer needs a schema change because the connect request and attachment record both require `listenAddress`. Reading the controller shows the field is softer than it looks. `ConnectTunnel` (`internal/controller/connectTunnel.go`) authorizes through `canConnectToTunnel` (which never sees the address) and builds the dial grant through `CreateAttachmentDialPolicy` (which never reads it either); `listenAddress` is only stored, echoed, and logged. It describes the local proxy's bind port, which exists only because today every consumer is a local proxy. A direct dialer keeps everything the record is actually for — authorization, observability — and lacks only a thing the record never used to dial.

So the attachment mirrors the tunnel: an explicit `NOT NULL` `kind` (`proxy` | `dialer`), with `listen_address` nullable under a conditional check (a `proxy` attachment must carry a real address, a `dialer` attachment must carry none), inferred at the edge from address presence with the same exact rule. Because nullable `listen_address` won't supply a uniqueness rule on its own (Postgres treats NULLs as distinct), a dialer attachment is unique by environment + tunnel via a partial unique index (`where kind = 'dialer'`), and a second `Dial` for the same environment + tunnel conflicts rather than silently sharing the record. Display surfaces render a `dialer` attachment as "direct dialer."

## Lifecycle: provisioning is explicit, primitives are thin

The division of labor across the surface:

- **Provisioning** (`CreateService` / `DeleteService` and the dialer's attach equivalent) owns the controller record and its Ziti policies — explicit create, explicit delete, persists in between. This is the `CreateShare` / `DeleteShare` parallel. Deleting removes the policies; for a direct tunnel that terminates established sessions at the fabric (the revocation path).
- **The primitive** (`Listen` / `Dial`) binds or dials the provisioned service and returns the raw `net.Listener` / `net.Conn`. Its `Close()` closes the overlay object and nothing else — there is no record under it to tear down.

Neither primitive heartbeats. The provider never needs to (the bind policy persists); the dialer never needs to once the reaper adjustment lands. That symmetry is the design: two thin primitives over records that persist until explicitly deleted.

## Required semantics

- `Listen` returns a `net.Listener`, `Dial` returns a dialer yielding `net.Conn`; both are the raw overlay objects, with no managed handle, heartbeat, or status surface wrapped around them.
- Provisioning is separate and explicit: a direct service/attachment record and its Ziti policies are created on provision and live until deleted; the primitives assume the record exists.
- The primitives need the `Agent`'s environment and identity to build the overlay context (and to provision), but they do **not** stand up or require the heartbeated managed runtime — they are thin, like zrok's `root`-based `NewListener`.
- A failure of the overlay surfaces as a normal `net.Listener` / `net.Conn` error (`Accept()` / `DialContext` returns), and the application owns the response. The SDK does not retry, does not heartbeat, and does not manage status.
- Revocation is enforced at the overlay by deleting the record's policy; the SDK never tracks or closes active connections. (Normative Layer 1 dependency: OpenZiti terminates established sessions on policy removal.)
- The phase-2 dialer requires the attachment-reaper adjustment (mark-stale-without-policy-removal) and a cancellable internal dial primitive, both named above, before it ships.
- Never expose `internal/network/tunnelruntime` to SDK consumers.

## Scenarios

**An embedded gateway serves HTTP directly.** `mcp-gateway` provisions a direct service (`Mode: ModeHTTP`, its grant list), calls `Listen` to get a `net.Listener`, and runs `httpServer.Serve(listener)` — no loopback, no `BackendTarget`, no hop. On shutdown it drains its own server and deletes the service. The flow is line-for-line the zrok `http-server` example, with `CreateService`/`Listen`/`DeleteService` standing in for `CreateShare`/`NewListener`/`DeleteShare`.

**A server rides out a transport flap.** The overlay errors; `Accept()` returns. The gateway's serve loop closes the listener and calls `Listen` again for a fresh one — its own resilience policy, in its own code. The provisioned service is untouched (the bind policy persists), so there's nothing to re-provision.

**An embedded client dials over the overlay (phase 2).** A process builds an `http.Client` whose `Transport.DialContext` is the dialer's. Every request dials the named service directly as a `net.Conn`, with no local port. The attachment authorizing the dials is a `dialer`-kind record with no listen address, and — with the reaper adjustment — it needs no heartbeat to stay authorized.

## Deferred (and Why)

- **The dialer implementation.** Designed here; phase-2 work, sequenced behind the provider listener. Its prerequisites — the attachment-reaper adjustment and a cancellable internal dial primitive — are named above and land with it.
- **UDP on the raw listener.** A `net.Listener` is stream-shaped; a packet-conn variant for UDP overlay traffic is a separate design. The stream path (`tcp`, `http`) covers the motivating cases.
- **A managed direct lifecycle.** If a future caller wants a *supervised* direct serve — heartbeated liveness, a managed status surface, automatic re-listen — that is a distinct, heavier surface built on these primitives, not folded into them. It is explicitly not part of this work; the primitives stay thin and `EnsureServed` / `EnsureConnected` remain the managed proxy option in the meantime.
- **Further record kinds.** The `kind` discriminator is introduced for `proxy`/`direct` and `proxy`/`dialer`; it's the right seam for any later shape that doesn't fit the local-proxy mold.

## Non-Goals

- Do not bundle a heartbeated, policy-driven lifecycle into `Listen` / `Dial`. The primitives return raw overlay objects; managed lifecycle is `EnsureServed` / `EnsureConnected`'s job, or a future explicit managed-direct surface.
- Do not require SDK consumers to import Agora internal packages — `internal/network/tunnelruntime` stays hidden.
- Do not remove or weaken `EnsureServed` or `EnsureConnected`.
- Do not make embedded applications run the external Agora daemon to use the primitives.
- Do not fake direct support with an unnecessary loopback proxy hop — removing that hop is the point.
- Do not make the SDK enforce revocation by tracking or closing active connections. Revocation is enforced at the overlay by deleting the policy; OpenZiti terminating established sessions on policy removal is a normative Layer 1 dependency of this surface.
