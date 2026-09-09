# Bug: runtime never recovers a lost ziti session

**Component:** `agora` network runtime — confirmed on **both** the consumer
(`connect`) and provider (`serve`) side
**Severity:** high — silently kills every tunnel it serves or connects, and both
the local runtime and the controller keep reporting the environment healthy, so
the outage is invisible until someone tries to use a tool. Five production MCP
tunnels stayed dead six days this way with nothing alerting.
**Observed:** 2026-08-17 (consumer, macOS 26 arm64, after multi-day sleep);
2026-08-31 → 2026-09-08 (provider, Linux host, no sleep involved)
**Versions:** client `v0.1.x [developer build]`, go1.25.7, darwin/arm64;
controller `agora-cloud:0.1.5` / chart `agora-0.1.7`; ziti 2.0.0

## The defect

**Once the runtime's ziti session is lost, it never re-establishes it.** The
process stays alive, keeps its environment heartbeat flowing, and reports
healthy — while its data plane is dead. Only a restart recovers it.

Sleep is not required. The provider-side occurrence was a server host that never
slept, so the trigger is session loss by any cause.

## Two confirmed variants

**Consumer side (`connect`).** The service list empties; every `http`-mode
connect returns 502 from the local listener because the ziti SDK cannot find the
service to dial. No circuit is ever attempted — zero fabric activity.

**Provider side (`serve`).** The bind sessions die and are never re-established,
so the tunnel loses its terminators. `tunnel_serves` rows go
`state=stale`/`disconnected` with `deleted=false` and simply stay there. The
fabric rejects every dial from every consumer.

Provider-side timeline for `ev_afjugjexhvrh` (`nf-tools-mcp-server@host`, ziti
identity `5hCsN4cjyM`), which serves five MCP tunnels:

| tunnel | last serve heartbeat |
|---|---|
| `mcp-redshift`, `mcp-es-noip`, `mcp-es-zrok` | 2026-08-31 16:44:58 |
| `ask-netfoundry`, `mcp-elasticsearch` | 2026-09-02 16:09:03, disconnected 16:10:00 (57s later) |

Never re-established on its own. Restarting the host's `agora-network.service`
on 2026-09-08 14:43:23 brought all five back to `state=active` with current
heartbeats within seconds, and the fabric's "no terminators" errors went from
~1,400/10min to zero — confirming the session, not the config, was the fault.

## The misreporting is server-side too

Originally logged as a local-only diagnosability problem. It is not.

Across both failures and continuing after them, the provider's **environment
heartbeat stayed an unbroken 240/hour (every 15s)** — `state: enabled`,
`last_seen_at` current to the second. The only blip in six days is a dip to
167/hr at Sep 2 15:00, the runtime hiccup that killed the last two serves.

So the controller, the API, and Mint all show a healthy enabled environment while
none of its five tunnels have had a terminator for days. The env heartbeat and
the bind sessions are independent, and only the heartbeat is watched.

Consumer side reports the same lie locally: `agora network status` shows every
connect `running`, `RETRY 0`, `NEXT RETRY 1970-01-01`, no `LAST ERROR`.

## Not the cause of the write storm

Corrected after the 2026-09-08 provider restart. The attach/detach churn on this
controller is **not** failed-dial retry and is not caused by this bug.

The cycle is client-side and continuous: `connecting` -> `connected` (~75ms) ->
`deleted` (~100ms later), about 290ms end to end, repeating across every
connected tunnel. It runs at the same rate whether or not the far side has a
terminator. One consumer was measured at 4.0 attachments/min before, during and
after the outage — identical, and with fabric errors at zero after the fix.

Volume is one misbehaving client, not the fleet: on the Sep 3 peak (170,073
attachments) a single environment `ev_i9by526h2gfr` accounted for **162,727 of
them (96%)**, flat at ~113/min around the clock. It was absent Sep 5-7, which is
the whole of the drop to ~9,000/day, and rejoined 2026-09-08 13:12.

Dead terminators amplify that client only modestly: 29.8/min during the outage
vs 19.3/min after recovery, so roughly a third of its churn was failure-driven
and two thirds is unconditional.

What the churn did cause is separate from this bug: 3 DB writes per cycle against
a controller pinned at `max_open_conns: 4`, where `/ready` is only
`store.DB().PingContext` with a 2s deadline
(`internal/controller/server.go:171`). When that client rejoined at 13:12 the
pool starved, readiness failed five times, Traefik dropped its only backend
(public API `503 no available server`), and liveness killed the container at
13:45:12 after 40 days and 0 restarts. Nothing was wrong with the controller.

The cost this bug does own is silence: `audit_events` holds only four
`event_type` values ever (`environment.heartbeat`, `tunnel.attached`,
`tunnel.detached`, `account.login`). There is no serve-lifecycle event, so
nothing recorded the five serves dying or staying dead for six days.

## Reproduce

**Provider side:**

1. `agora tunnel serve <name> --mode http --backend <target>` from a host,
   confirm a consumer can reach it.
2. Interrupt that host's egress to the router edge (`:3022`) long enough for the
   bind session to lapse; restore it.
3. Query the controller: environment `state=enabled` with a current
   `last_seen_at`, while `tunnel_serves` for that tunnel is `stale` and the
   fabric has no terminators.

**Consumer side:**

1. Enroll, `agora tunnel connect <name> --listen 127.0.0.1:<port>` against an
   `http`-mode tunnel, confirm 200.
2. Cause session loss — sleep the host, or interrupt egress to `:3022`.
3. Restore connectivity and request the loopback endpoint again.

Expected in both: recovery. Actual: dead indefinitely until the runtime is
restarted, with everything reporting healthy. A restart re-authenticates and
repopulates, which is what isolates the fault to the session rather than to
grants, policy, or the far side.

Also reproduces with no interruption at all: grant a tunnel to a consumer whose
runtime is **already running**. It never picks up the new service.

## Signatures

Provider side — ziti controller and router, ~1,400 in 10 minutes:

```
create.circuit  responded with error  service 5pnMqP6UUOkujOnHZwnBET has no terminators
xgress_edge     failed to dial fabric  service 5pnMqP6UUOkujOnHZwnBET has no terminators
```

Consumer side — the runtime's own log, `~/Library/Logs/agora/agora-network.err.log`:

```
http: proxy error: service 'tt_cla6wq5ndaxv' not found
```

A client-side service-list miss, not an authorization failure.

## Ruled out

| Checked | Result |
|---|---|
| Control plane | healthy — heartbeat current, `tunnel list --accessible` returns all grants, API 200 |
| Egress | `:443`, `:1280`, `:3022` reachable from the host |
| Fabric infrastructure | ziti controller + router pods and node up throughout |
| Fabric activity during consumer dials | **none** — no circuits created, no policy denials |
| Provider side (Aug 17 case) | unchanged and serving; same tunnels reachable from another client |
| Identity | unchanged across the failure in every case |

Also reported in **Ziti Desktop Edge**, so the session-recovery gap is likely
client-layer rather than Agora-specific.
