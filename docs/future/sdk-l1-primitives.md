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

The new entry points are `Listen` on the provider side and `Dial` on the consumer side. `EnsureServed` / `EnsureConnected` are untouched and remain the managed option for anyone who wants the heartbeated, supervised lifecycle. `Listen` is specified for near-term implementation; `Dial` is designed here but is phase-2 work (its prerequisites are detailed below).

## Why thin is the whole point

It's worth being explicit, because an earlier draft went the other way. The temptation is to make `Listen` *also* own the controller record — create it, heartbeat it, expose its status, tear it down — so the one call does everything. That merge is exactly what produces unbounded complexity: once a primitive owns a heartbeated record, it needs a policy for what to do when heartbeats fail, a story for partitions and stale-reaping, a draining state for graceful shutdown, a status surface, and an atomic teardown contract. None of that is transport; all of it is lifecycle the application didn't ask the *primitive* to manage.

zrok avoids every bit of it by keeping provisioning separate. Agora can too, and Agora's own controller design makes it natural — as the next sections show, a provider's authorization survives without any heartbeat at all.

## Provider side: `Listen`

The embedded process should be able to write this — the same three-step shape as the zrok example:

```go
// 1. provision a direct tunnel (persists until deleted)
tnl, err := tunnel.Create(ctx, a, tunnel.Spec{
    Name:        "mcp-gateway",
    Mode:        tunnel.ModeHTTP,
    GrantEmails: []string{"consumer@example.com"},
})
if err != nil {
    return err
}
defer tunnel.Delete(ctx, a, tnl)

// 2. thin primitive: the overlay net.Listener
listener, err := tunnel.Listen(ctx, a, tnl.Name)
if err != nil {
    return err
}
defer listener.Close()

// 3. serve — the process owns its protocol
return httpServer.Serve(listener)
```

`Listen` returns a `net.Listener` and an error — nothing else. The listener is the overlay listener; the caller passes it straight to its own server and owns protocol serving. There is no `BackendTarget`, because there is no backend: the process serving the listener *is* the backend. There is no handle, no `Status()`, no heartbeat policy — the primitive is the thing zrok's `NewListener` is.

Provisioning is the separate `Create` / `Delete` step — mirroring the controller's existing `createTunnel` / `deleteTunnel` operations. It creates a direct tunnel — the Ziti service, the bind policy, and the grants — and that record persists until `Delete`, the same as a zrok share persists until `DeleteShare`.

`Listen` is thin but not *blind*. A tunnel name is not the Ziti service identifier — Agora names the service by tunnel ID — so each verb first resolves the given name or ID to a controller tunnel, then opens the overlay on the tunnel's Ziti service (its `tunnel.ID`), exactly as the managed actors already do internally. This is the one place the primitive must look past the raw name: an implementation that calls `overlay.Listen(name)` directly would pass stub tests and fail against a real fabric. Precedence is fixed so name and ID can't collide: a ref matching the tunnel-ID pattern (`^tt_[a-z0-9]{12}$`) resolves by ID only; any other ref resolves by name. *How* it resolves differs by verb: `Listen` resolves within the agent's own organization (you serve a tunnel you own), while `Attach` / `Dial` resolve against everything the agent is *granted* to reach — cross-org grants included, using the same can-connect semantics as the connect path — because a consumer routinely dials another org's tunnel. Consumer-side name resolution must be unambiguous: if more than one granted tunnel shares the name, the verb fails rather than silently picking one.

What each verb *validates* differs, because serving and dialing are different rights:

- `Listen` requires `kind = direct`, a stream mode, Ziti service metadata present, and the tunnel's environment to **match the agent** — you can only serve a tunnel your own environment owns.
- `Attach` requires the existing can-connect authorization (account- or grant-based), **not** an environment match — a consumer dials *into another* environment's tunnel by design — and creates a dialer attachment for the agent's environment.
- `Dial` requires an active, still-authorized dialer attachment (active, not deleted, live dial policy) for the agent's environment + this tunnel; it does not require the tunnel's environment to match the consumer.

`Create` defaults the tunnel's environment to the agent's enrolled environment when the `Spec` omits it.

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

Kind is inferred at the edge: a create request carrying a `backendTarget` is a managed-proxy tunnel; one that omits it is a direct tunnel. The presence rule is exact: an omitted `backendTarget` means `direct`, a trimmed non-empty value means `proxy`, and a present-but-empty or whitespace-only value is rejected rather than coerced. The field is optional in the API contract, not `nullable` — kind keys on presence-vs-absence, so an explicit JSON null is rejected (clients omit the field). A tunnel name is one kind: `EnsureServed` and `Listen`'s provisioning on the same name conflict rather than silently sharing a record. Display surfaces render a direct tunnel as "direct listener" wherever a proxy tunnel shows its `backend_target`. The discriminator is part of the API contract, not only the database: the `Tunnel` REST response exposes `kind` and makes `backendTarget` optional (absent-only, not `nullable`), so SDK, CLI, and UI clients branch on a machine-readable field rather than a rendered string.

### Mode is metadata, and the path is stream-only

`Mode` is a provisioning-time attribute of the tunnel, not behavior of the primitive, and it is purely *Agora-side* metadata — the current provisioner is mode-agnostic at the fabric and does not use it to configure the Ziti service or its policies. What `Mode` actually drives today is SDK/runtime validation and display, plus, in the managed world, which wrapping the proxy applies (`ModeHTTP` = "wrap the overlay listener in a reverse proxy"). `Listen` does no wrapping — the caller's server reads bytes off the listener directly — and it negotiates no handshake; matching protocols across a listener/dialer pair is the application's responsibility. So for a direct `Listen` / `Dial`, `Mode` collapses to stream-or-not: `http` and `tcp` are stream metadata and are supported, `udp` is rejected because a `net.Listener` is stream-shaped (a packet-conn variant is out of scope, see Deferred).

## Consumer side: `Dial` (phase 2)

The consumer mirrors the provider in the same three steps — the exact parallel to zrok's consumer flow, where `CreateAccess` / `DeleteAccess` provision the access and `NewDialer` dials it:

```go
// 1. provision the dial grant (persists until detached)
att, err := tunnel.Attach(ctx, a, "mcp-gateway")
if err != nil {
    return err
}
defer tunnel.Detach(ctx, a, att)

// 2. thin primitive: a raw overlay net.Conn, dialed per call
client := &http.Client{Transport: &http.Transport{
    DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
        return tunnel.Dial(ctx, a, "mcp-gateway")
    },
}}
```

`Attach` / `Detach` are the consumer's provisioning step: they create and remove the tunnel attachment and its dial policy, persist-until-delete, mapping to the controller's `connectTunnel` / `deleteTunnelAttachment`. They mirror zrok's `CreateAccess` / `DeleteAccess`.

Because a dialer attachment persists until detached and the reaper no longer reaps it, `Attach` / `Detach` must be keyed to something a restarted process can re-derive — its own environment plus the tunnel — not the opaque attachment ID it may have lost in a crash. So `Attach` is **idempotent by that natural key**: if an active, still-authorized dialer attachment already exists for the agent's environment + tunnel — active, not deleted, and holding a live dial policy — `Attach` returns it rather than conflicting, making recovery as simple as calling `Attach` again. And `Detach` accepts the same natural key (tunnel name or ID), so a process can clean up by-key without the ID. Together these close what would otherwise be a restart dead-end: the controller's by-ID-only delete and manager-only attachment listing leave a consumer no other way to find or remove its own grant after a crash.

`Dial` is the thin primitive — `Dial(ctx, a, name) (net.Conn, error)` returns a single raw stream-oriented overlay `net.Conn`, the same shape as zrok's `NewDialer` (`edge.Conn`). It is not a dialer object and has no `Close()` of its own; the conn's own `Close()` closes the conn, and the attachment's lifecycle lives entirely in `Attach` / `Detach`. For an HTTP client, wrap it in the trivial `DialContext` closure above — and because the target is fixed by the tunnel name and `addr` is ignored, that transport should be dedicated to the one tunnel (every request routes there regardless of URL host).

### Making the dialer thin: a reaper adjustment, and the cleanup it implies

The consumer side has one wrinkle the provider side doesn't, and closing it is a small but careful Layer 1 change. Today the attachment reaper (`tunnelAttachmentReaper.go`) deprovisions the dial policy and detaches the attachment 45s after the last heartbeat — so a dial authorization currently requires ongoing heartbeat, which a thin primitive shouldn't have to send.

The change must be **kind-specific**, not blanket. The reaper serves both proxy and dialer attachments, and a managed `proxy` attachment *should* still be deprovisioned-and-detached when it goes stale — leaving its dial policy behind would be a zero-trust regression, and the audit trail (a `tunnel.detached` event) would lie. So proxy-kind attachments keep today's behavior exactly. For **dialer-kind** attachments the reaper does nothing at all — it neither deprovisions the policy nor marks the row stale. Staleness is a heartbeat-liveness signal, and a primitive dialer doesn't heartbeat by design, so there is no staleness to detect: the attachment simply persists as an active authorization until it is explicitly detached (symmetric with the provider's bind policy), and `Dial` needs no heartbeat. The controller makes no claim about the dialer process's liveness — it can't, and pretending to via a stale/detached state would be the actual misrepresentation. What it shows honestly is that the grant is active, which is true until `Detach`.

Retaining the policy means the reaper is no longer the universal sweep that revoked an abandoned dialer's grant, so every revocation path must now ensure the dial *policy* is removed — that policy is the security boundary. zrok ships this model too: no consumer reaper, revocation enforced at the policy layer, cleaned up on provider teardown, consumer-environment teardown, and explicit detach. Agora can do it more robustly than zrok, though — zrok identifies a consumer's policy only by tag and must `CleanupByTag`, whereas Agora records each attachment's `dial_policy_id` on its row, so the authoritative path is to enumerate the affected attachment rows and deprovision each by its stored id. The phase-2 work covers these paths (the work order specifies the exact queries):

- **Provider-side teardown** — tunnel delete, session teardown, and provider-environment disable must deprovision the dial policies of *every* consumer of the tunnel, **including cross-org consumers**. This is load-bearing: these paths today enumerate attachments scoped to the *provider's* org, which misses a cross-org consumer's dial policy — and with the reaper gone, that policy would survive as a live, dangling grant. The fix is cross-org enumeration keyed on the tunnel (not the provider org), deprovisioning each by its stored policy id.
- **Grant removal** — removes only the de-granted *account's* dial policies for the tunnel (cross-org-capable, account-scoped), touching no other consumer — not a whole-service sweep.
- **Consumer-environment disable/delete** — enumerates the environment's *own* dialer attachments (where it is the consumer) and deprovisions them. `DisableEnvironment` today only enumerates the tunnels an environment *owns*.
- **Explicit `Detach`** — removes its own attachment and dial policy by natural key.

Because each path deprovisions and deletes the rows it enumerates, nothing orphans. The only residue would be a policy with no row from a prior failed write; an optional periodic GC handles that and is deferred. (A dialer that merely crashes while its environment stays alive is likewise not a new gap: its attachment persists until explicit `Detach`, the same persist-until-delete trade the provider's bind policy already makes.)

The kind-specific reaper change and the consumer-attachment teardown cleanup are together the phase-2 prerequisite that makes `Dial` as thin as `Listen`.

With it in place, the dialer's failure model is the listener's: a per-dial error (no route, remote down, canceled request) fails that `Dial` call and returns to its caller; the dial stops working entirely only when its authorization is deleted (revocation), which the overlay enforces. Cancellation has its own prerequisite: `ctx` must govern the dial so a stalled dial doesn't outlive its request *from the caller's view* — today's internal primitive is the contextless `OverlayContext.Dial`, so phase 2 adds a `DialContext`-shaped method that returns promptly on cancellation and disposes of any late connection. It uses a context-capable ziti dial where the SDK offers one; otherwise a bounded goroutine fallback whose orphaned attempt finishes and is discarded in the background, so the caller never receives a late conn. The work order specifies the contract.

### `listenAddress` is not load-bearing, so a dialer attachment needs none

The original request worried that a direct dialer needs a schema change because the connect request and attachment record both require `listenAddress`. Reading the controller shows the field is softer than it looks. `ConnectTunnel` (`internal/controller/connectTunnel.go`) authorizes through `canConnectToTunnel` (which never sees the address) and builds the dial grant through `CreateAttachmentDialPolicy` (which never reads it either); `listenAddress` is only stored, echoed, and logged. It describes the local proxy's bind port, which exists only because today every consumer is a local proxy. A direct dialer keeps everything the record is actually for — authorization, observability — and lacks only a thing the record never used to dial.

So the attachment mirrors the tunnel: an explicit `NOT NULL` `kind` (`proxy` | `dialer`), with `listen_address` nullable under a conditional check (a `proxy` attachment must carry a real address, a `dialer` attachment must carry none), inferred at the edge from address presence with the same exact rule. Because nullable `listen_address` won't supply a uniqueness rule on its own (Postgres treats NULLs as distinct), a dialer attachment is unique by environment + tunnel via a partial unique index whose predicate matches the authorization predicate exactly (`where kind = 'dialer' and state = 'active' and not deleted and dial_policy_id is not null`) — one *active* grant per pair. The authorization predicate is precise: an active, non-deleted dialer row with a non-null `dial_policy_id`. A disconnected or policy-removed row is not a grant — it neither satisfies `Attach`/`Dial` nor blocks a fresh `Attach` — which matters because the controller's delete path marks rows disconnected without setting `deleted = true`. `Attach` is idempotent on the key: a repeat returns the existing active attachment rather than conflicting, so a crashed dialer recovers by re-attaching. Display surfaces render a `dialer` attachment as "direct dialer." As with the tunnel, the `TunnelAttachment` REST response exposes `kind` and makes `listenAddress` optional (absent-only, not `nullable`) in the contract. And because a dialer-kind row sits at `state = active` for its whole life with no heartbeat, status and dashboard surfaces should read that `active` kind-awarely — as a live *authorization*, not a live process: count dialer attachments as active grants (or hold them apart from live-runtime/heartbeat counts) rather than reporting a heartbeat freshness they don't have. (Non-normative; UI's call on exact presentation.)

## Lifecycle: provisioning is explicit, primitives are thin

The division of labor across the surface:

- **Provisioning** (`Create` / `Delete` for tunnels, `Attach` / `Detach` for dial attachments) owns the controller record and its Ziti policies — explicit create, explicit delete, persists in between. This is the zrok `CreateShare`/`DeleteShare` and `CreateAccess`/`DeleteAccess` parallel. Deleting removes the policies; for a direct tunnel that terminates established sessions at the fabric (the revocation path).
- **The primitive** (`Listen` / `Dial`) binds or dials the provisioned tunnel and returns the raw `net.Listener` / `net.Conn`. Its `Close()` closes the overlay object and nothing else — there is no record under it to tear down.

Neither primitive heartbeats. The provider never needs to (the bind policy persists); the dialer never needs to once the reaper adjustment lands. That symmetry is the design: two thin primitives over records that persist until explicitly deleted.

## Required semantics

- `Listen` returns a `net.Listener`, `Dial` returns a single `net.Conn`; both are the raw overlay objects, with no managed handle, heartbeat, or status surface wrapped around them.
- Provisioning is separate and explicit: a direct tunnel/attachment record and its Ziti policies are created on provision and live until deleted; the primitives assume the record exists.
- The primitives need the `Agent`'s environment and identity to build the overlay context (and to provision), but they do **not** stand up or require the heartbeated managed runtime — they are thin, like zrok's `root`-based `NewListener`.
- A primitive resolves the given tunnel name or ID to a controller tunnel before opening the overlay on the tunnel's Ziti service id (never the raw name). Resolution and validation are per verb: `Listen` resolves same-org and requires `kind = direct`, a stream mode, and the tunnel's environment to match the agent; `Attach` / `Dial` resolve via can-connect authorization (cross-org grants included, name matches required to be unambiguous) and require that authorization rather than an environment match, since a consumer dials another environment's tunnel.
- A failure of the overlay surfaces as a normal `net.Listener` / `net.Conn` error (`Accept()` / `Dial` returns), and the application owns the response. The SDK does not retry, does not heartbeat, and does not manage status.
- Revocation is enforced at the overlay by deleting the record's policy; the SDK never tracks or closes active connections. (Normative Layer 1 dependency: OpenZiti terminates established sessions on policy removal.)
- The phase-2 dialer requires the attachment-reaper adjustment (the reaper skips dialer-kind attachments entirely), the cross-org revocation-path policy cleanup (every revocation path deprovisions affected consumers' dial policies by enumerating their attachment rows cross-org and removing each by its stored policy id — provider teardown for all consumers, grant removal account-scoped, consumer-env disable for that environment's own dials), and a cancellable internal dial primitive — all named above — before it ships.
- Never expose `internal/network/tunnelruntime` to SDK consumers.

## Scenarios

**An embedded gateway serves HTTP directly.** `mcp-gateway` provisions a direct tunnel (`Mode: ModeHTTP`, its grant list), calls `Listen` to get a `net.Listener`, and runs `httpServer.Serve(listener)` — no loopback, no `BackendTarget`, no hop. On shutdown it drains its own server and deletes the tunnel. The flow is line-for-line the zrok `http-server` example, with `Create`/`Listen`/`Delete` standing in for `CreateShare`/`NewListener`/`DeleteShare`.

**A server rides out a transport flap.** The overlay errors; `Accept()` returns. The gateway's serve loop closes the listener and calls `Listen` again for a fresh one — its own resilience policy, in its own code. The provisioned tunnel is untouched (the bind policy persists), so there's nothing to re-provision.

**An embedded client dials over the overlay (phase 2).** A process calls `Attach` to provision its dial grant, then builds an `http.Client` whose `Transport.DialContext` wraps `tunnel.Dial`. Every request dials the named tunnel directly as a fresh `net.Conn`, with no local port. The attachment authorizing the dials is a `dialer`-kind record with no listen address that — with the reaper adjustment — needs no heartbeat to stay authorized, and is removed on `Detach`. The flow mirrors zrok's `CreateAccess`/`NewDialer`.

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
