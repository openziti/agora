# Persisted Session Connects: Investigation Findings

Status: findings, 2026-09-18. Companion to the [investigation plan](persisted-session-connects-investigation.md). Diagnostic only; no fix is proposed or implemented here.

**Updated outcome: integration behavior established.** After the investigation, the reporter identified its agora-gateway Claude Code plugin as the caller. It uses controller REST APIs and `agora tunnel connect` against the shared daemon, not the session SDK. Catalog discovery connects by tunnel ID; Layer 1 discovery can connect accessible session tunnels by their `session-<ses_id>` names. Individual historical name-based rows were not attributed to captured runs. The reporter describes cleanup failures being forgotten, ledgers being lost across data-directory changes, and no ledger in Layer 1 mode. These explain how durable CLI connects are abandoned. The local live control below reproduces their persistence and retry behavior; the reporter's application itself was not run by the investigator.

On 2026-10-19 Michael chose a narrow follow-up: explicit local connect removal across all listen addresses for a supplied tunnel reference. Implemented on 2026-10-20 as `agora tunnel delete connect <name|tt_...> --all`; the realized spec and work order have been retired into the [current command contract](../../current/layer-1/agent.md#bulk-local-connect-removal), with review decisions in the [project journal](../../journal/2026-10-20.md). Automatic pruning, session ownership, and SDK lifecycle fixes remain separate. A lost connector ledger does not erase the saved listen address: `agora status --json` exposes local desired-connect records.

The numbered sections below preserve the original investigation as of 2026-09-18. Their unknown-caller classification and request to wait for attribution are superseded by this addendum.

Sections below keep three registers apart: **observed** (I ran it and saw it), **source** (read from code, not executed), and **hypothesis** (inference, untested).

## 1. Provenance

| Item | Value |
|---|---|
| Checkout | `ee1597d` (v0.1.8), `main`, clean except the untracked plan document |
| Revisions compared | `646cdbf` (2026-07-02), `ea45285` (v0.1.7), `ee1597d`; all present locally |
| CLI + daemon used in reproduction | built from an isolated detached worktree at `ee1597d` with `-tags no_agora_ui` (the worktree has no `ui/dist`). `vcs.modified=true` because the worktree held two untracked diagnostic files (below); no tracked source was modified |
| SDK in the consuming application | same worktree; the harness is a ~100-line `agent.New(..., agent.WithRuntime())` program using only `session.Propose` / `session.RegisterHandler` / `Send` / `Receive` / `Close`. No `replace` directives in `go.mod` |
| Controller | the operator's dev controller on `:18081`, designated by the operator for this task. At session start it was pid 1028997 and the installed `agora` binary reported `6fc13b4` (v0.1.7). It was restarted (not by me) at 10:44:26 local as pid 628584 from a rebuilt binary stamped `ee1597d`, `vcs.modified=true`. Every live case below ran after that restart, so against a HEAD controller |
| External services | Postgres 16.8 and OpenZiti 2.0.0 quickstart, both already running; untouched |
| Isolation | environment roots under the session scratchpad, selected with `AGORA_ENV_ROOT` / `--env-root`. `~/.agora` was never read or written (mtimes unchanged). One dedicated org, two accounts, one workgroup, one advertisement; all deleted afterward |

One deviation: the scratchpad path is longer than the 108-byte Unix socket limit, so `agora network start` failed with `bind: invalid argument`. I used a symlink `/tmp/zzi` pointing at the scratchpad roots for the daemon cases and removed it afterward.

Relevant-path diff across the three revisions (source): `sdk/agent/session/*`, `embedded.go`, `app.go`, `standalone.go`, `agent.go`, `cmd/agora/tunnelConnect.go`, `cmd/agora/sessionSend.go`, and `internal/network/daemon/` are **identical** at `646cdbf`, `ea45285`, and `ee1597d`. `state.go` and `tunnel_runtime.go` differ only by log lines and the attachment-recovery token. The writer structure analysed here is the same code the reporter ran, *for the in-tree callers*. The SDK revision embedded in the reporter's application is unknown.

## 2. Candidate writers

Every path that can put a connect into a `network.json`:

| Caller | Runtime construction | Disk write possible | Evidence | Unresolved |
|---|---|---|---|---|
| `session.Propose` → `attachConsumerStream` (`sdk/agent/session/transport.go:54`) | `a.Runtime()`, which is only ever assigned from `NewEmbedded` (`app.go:179`, `standalone.go:88`); `root: nil` | **No.** `persistNetwork` returns early on nil root (`state.go:222`) | source at all three revisions; mocked fixture; live cases 1 and 2 | none in-tree |
| `agora session send` (`cmd/agora/sessionSend.go:56`) | `agent.New(..., WithRuntime())` → embedded | **No** | source; live run with daemon present | none |
| `tunnel.EnsureConnected` (`sdk/agent/tunnel/connect.go:34`) | `a.Runtime()` → embedded | **No** | source | sends `Name` only; an application using this public API still cannot reach the daemon's file |
| `agora tunnel connect <arg> --listen` (`cmd/agora/tunnelConnect.go:69`) | daemon gRPC over `network.sock` → `NewDaemon` runtime with a real root | **Yes** | source; mocked fixture; live case 3 | sends `Name: args[0]` only. Whatever string the caller typed becomes the row name |
| Any custom gRPC client of `network.sock` | daemon | **Yes** | source (`EnsureConnect` is an exported RPC; the socket is filesystem-permission secured only) | could supply `TunnelId` + any `Name`. No in-tree caller does |
| `Runtime.ReloadEnvironment` (`agent.go:236`) | assigns `a.root = root` unconditionally | would give an embedded runtime a root | source | exported method, but its only caller is the daemon client over gRPC (`internal/network/daemon/client.go:96`). An embedded runtime serves no gRPC, so this is unreachable unless an application calls it in-process. **Hypothesis only**; worth one question to the reporter |
| Offline CLI mutators (`cmd/agora/tunnel.go:439,468,500`) | direct `root.SetNetwork` | removal/drain paths only | source | do not add connects |

### The `session-` name is not an SDK fingerprint

**Source:** the controller names every session tunnel `session-<ses_id>` (`internal/controller/acceptSession.go:81`). `resolveConsumerTunnel` (`tunnel_helpers.go:140`) accepts either that name or the `tt_…` id. So `agora tunnel connect session-ses_X --listen …` and `agora tunnel connect tt_X --listen …` both resolve the session's tunnel, and `updateConnectDesired` (`tunnel_runtime.go:439`) backfills `tunnel_id` on first success. The SDK happens to use the same string for its in-memory actor name, which is what made the report's attribution look natural.

A row with `name: session-ses_…` and a populated `tunnel_id` is therefore exactly what a **CLI caller connecting by tunnel name** produces. A bare `tt_…` row is what a CLI caller connecting by id produces. The report's "one dead tunnel under two names" pair (`tt_mw0nekz7bokx` on `:8473`, `session-ses_0sgs8dj3csiu` on `:8413`) is the same session tunnel addressed both ways.

### The listen ports in the field evidence argue against the SDK path

**Source + observed:** `pickEphemeralLoopbackPort` binds `127.0.0.1:0`, so the SDK's listen addresses come from the kernel's ephemeral range. Live, I saw `46101`, `36295`, `32963`. On the reporter's darwin host that range is 49152–65535 by default. Every dead row in the report listens on `8413`–`8479`, clustered and near-sequential, with `:8473` reused across two tunnels. **Hypothesis:** those addresses were chosen by a caller (a script or wrapper allocating from an `84xx` block), not by `pickEphemeralLoopbackPort`. This would be overturned by a non-default `net.inet.ip.portrange` on the reporter's host; it is a cheap thing to ask.

The report's §2 growth claim ("N attaches produce N permanent entries" via fresh ephemeral ports) is correct as a description of the SDK's *in-memory* actor map, but does not apply to `network.json`, which the SDK path never writes.

## 3. Reproductions actually run

### Mocked fixture (fake controller and runtime host; no live services)

`zz_investigation_test.go`, package `agent`, run in the isolated worktree; both tests pass.

- **Embedded, SDK request shape** (`TunnelId` + `Name: session-ses_…` + ephemeral listen), with and without a pre-seeded daemon `network.json`: file not created / byte-identical afterward. One in-memory actor retained, since session teardown issues no runtime call.
- **Daemon, CLI request shape** (`Name` only), once by session-tunnel name and once by `tt_` id: two rows persisted with backfilled `tunnel_id`; after the fake controller starts answering `tunnel not found` and the daemon restarts, both rows are restored in `error` with climbing `retry` and `last_error='tunnel not found'`.

### Live cases (HEAD controller, real OpenZiti)

All timestamps UTC, 2026-09-18.

| Case | What ran | Result |
|---|---|---|
| 1. SDK session, daemon absent | harness consumer pid 630648, session `ses_j7dvarn3aa7s` / `tt_4dkhar2s67vh`, active 14:46:35, `Session.Close` 14:46:44 | **No `network.json`** in the consumer root during the session, after close, or after process exit. Embedded runtime held 1 actor (`session-ses_j7dvarn3aa7s`, `127.0.0.1:46101`); 40s after close it was `error`, `retry=6`, `tunnel not found`. It died with the process |
| 2. SDK session, daemon present on the same enrollment | daemon pid 631882; consumer pid 632055, session `ses_5cqrn5s4301f` / `tt_b2flbwcflwxd` | Daemon reported `no desired serves or connects configured` during the session and 35s after close. **No `network.json`.** Embedded actor retained in memory as in case 1. The isolation boundary in `agent.md` holds |
| 2b. `agora session send`, daemon present | session `ses_ifmal4p1ck8w` | Same: daemon state and root unchanged |
| 3. Explicit CLI connects to a live session tunnel | session `ses_f1317crodzsp` / `tt_avkcn18c5c7n`; 14:49:49 `agora tunnel connect session-ses_f1317crodzsp --listen 127.0.0.1:8413` and `agora tunnel connect tt_avkcn18c5c7n --listen 127.0.0.1:8473` | Two rows written to `network.json`, both `running`, shape identical to the field rows. Session closed 14:53:35 (`consumer_close`). At 14:55:14 both rows `error`, `retry=8`, `tunnel not found`, attachment `-`. Daemon restarted (pid 635056 → 638655): both rows restored, retry counters reset and climbing |
| 3, accumulation | three sessions proposed via CLI, one `tunnel connect` each on `:8421`–`:8423`, each session then closed | 2 → 3 → 4 → 5 rows. One permanent row per connect invocation; nothing removes any of them |
| Cleanup paths | `agora tunnel delete tt_avkcn18c5c7n` | `panic: tunnel not found`; rows untouched. `agora tunnel delete connect <name> --listen <addr>` removed each row. Confirms the report's §3 |

A caveat on case 3's first pass, recorded because it nearly produced a wrong observation: the harness consumer hit `connection reset by peer` on its first `Receive` and exited without calling `Close`, so the session stayed `active`. For ~4 minutes the two CLI rows were legitimately `running` and even re-attached across a daemon restart. I only got the result above after closing the session with `agora session close`. The first-dial reset is listed under adjacent observations.

In the accumulation run the connects were issued while the sessions were still `proposed` (`agora session propose` returns before acceptance), so the tunnels did not exist yet. Each `tunnel connect` returned `tunnel not found` to the caller **and still left a persisted, retrying row** — with TUNNEL ID `-`, since it never resolved. This confirms live, at the daemon, the plan's note that an initial ensure failure leaves desired state registered. It also gives a discriminator: **every dead row in the field evidence has a populated tunnel id, so each of those connects succeeded at least once** (or came from a gRPC caller that supplied `TunnelId`).

## 4. Demonstrated causal chain, and what it does and does not cover

Established live, as a control:

`agora tunnel connect <session-tunnel name or id> --listen <addr>` → daemon `EnsureConnect` → `prepareConnectEnsure` → `upsertConnect` + `persistNetwork` → `network.json` → session closes, controller soft-deletes the tunnel → row survives, retries `tunnel not found` at the backoff ceiling → daemon restart re-arms it via `startConfiguredActorsLocked`.

Minimal reproduction: with a daemon running and any session `active`, run `agora tunnel connect session-<ses_id> --listen 127.0.0.1:8413`, then `agora session close <ses_id>`, then `agora network stop && agora network start`.

This satisfies the plan's chain from "daemon desired-state write" onward. It does **not** supply the first link. I ran that command myself; nothing shows the reporter's application does. Per the plan, this control "alone does not explain the field caller."

## 5. Classification of the persisted rows

**Explicit durable connects of unknown origin.** Not session-owned intent.

- Not session-owned: the only code that treats a connect as belonging to a session is the SDK transport, and it cannot write the daemon's file. The daemon has no concept of a session; to it these are ordinary operator-declared connects, behaving exactly as `agent.md` documents.
- Explicit durable connect: the row shapes, the backfilled tunnel ids, the name/id duality, and the caller-chosen listen ports all match the daemon `EnsureConnect` path reached with a caller-supplied name. The CLI is the only in-tree client of that path.
- Unknown origin: whether that caller was a person, a shell script, an application shelling out to `agora tunnel connect`, or a custom gRPC client is not determinable from here.

## 6. Responsible component and smallest justified next action

On current evidence the responsible component is **the integration on the reporter's side that connects to session tunnels through the daemon**, not the session SDK and not the daemon's retry policy. The smallest justified next action is to **send the evidence request below and wait**. If the answer is "our application runs `agora tunnel connect` against the session's tunnel," the outcome becomes *integration behavior established*, and the real conversation is an API-design one: that application wants a session-scoped, non-durable consumer attach, and is reaching for the only durable-by-design tool available. That discussion should not start until the caller is known.

Kept separate, because nothing connects it to this incident: the SDK's in-memory actor leak (`teardownTransport` never calls `RemoveConnect` / `RemoveServe`) is real and was observed in every live SDK session. It is bounded by process lifetime on the consumer side. Its provider-side consequence is more serious than previously understood; see adjacent defect A.

## 7. Remaining uncertainty and the evidence that resolves it

For the operator to relay (the plan's request, plus three discriminators this investigation earned):

> Which application or command opens these sessions? Please provide its version or SDK dependency revision, the relevant session setup/teardown code or shell commands, and how the daemon is started. Does it invoke `agora tunnel connect`, call the daemon's local gRPC API, or use a wrapper that does either? If practical, capture sanitized `network.json` connect records immediately before and after one session, alongside the matching session/tunnel IDs and daemon status. Please omit credentials and identity files.
>
> Three specific questions: (1) Where do the listen ports in the `8413`–`8479` range come from — does something on your side allocate them? Is `net.inet.ip.portrange.first` changed from its default? (2) Do the dead rows in `network.json` carry an `attachment_id` field? (3) Does your application ever call `ReloadEnvironment` on a runtime it constructed itself, or construct a runtime any way other than `agent.New(..., agent.WithRuntime())` / `agent.NewStandalone`?

What each answer settles: a port allocator or wrapper script identifies the caller directly. `attachment_id` exists only in builds after the attachment-recovery change, so its presence or absence dates the daemon build that last wrote each row. Question 3 closes the one source-level route by which an embedded runtime could acquire a root.

Not checked: the reporter's application (unavailable); any SDK revision other than the three named; darwin behavior (all runs were linux/amd64); close paths other than `consumer_close` via `Session.Close` and `agora session close`. Provider-initiated close and controller reaping were not exercised, because no tested path made the consumer-side result depend on who closed.

## 8. Adjacent defects (recorded, not pursued)

**A. Provider-side retained serve actors re-create session tunnels as standalone tunnels after close.** Observed during cleanup: the provider account owned five `active` standalone (no environment) proxy tunnels named `session-ses_…`, one for every SDK-accepted session in this investigation, each with a **new** `tt_` id different from its session's original tunnel (e.g. `ses_f1317crodzsp` ran on `tt_avkcn18c5c7n`; afterward `session-ses_f1317crodzsp` existed as `tt_e9bracr3z1w0`). Source: the provider's retained serve actor keeps retrying `StartServe`, which goes through `ensureTunnelCreatedOrReused` by name with the stored mode and backend (`sdk/agent/controller_client.go:87`), so once the session tunnel is deleted the retry creates a fresh account-owned tunnel under the same name. These outlive the provider process, block `agora admin delete user` ("account still owns standalone tunnels"), and mean the name `session-<ses_id>` can resolve again after the session is closed. Unlike the consumer-side leak this writes durable controller state. 5 of 5 sessions. Roadmap card added.

**B. A failed daemon `EnsureConnect` returns an error and still persists the row.** Anticipated by the plan; confirmed live (accumulation run). A caller that sees `tunnel not found` has no reason to suspect it just created permanent desired state.

**C. `sessions.md` CLI drift.** The doc specifies `agora session close -y/--yes` and `agora session propose --json/--timeout`; none exist at HEAD. `session propose` also returns at `proposed` rather than waiting for `active` as documented. Roadmap card added.

**D. Consumer first-dial reset.** One of four harness sessions failed its first `Receive` with `connection reset by peer`, alongside a tunnelruntime warning `failed to open local tcp tunnel session`. Seen once, not investigated; possibly the consumer dialing before the provider's serve is bound.

**E.** `agora tunnel delete <tt_…>` on a missing tunnel surfaces as a raw `panic: tunnel not found`. Already in the report.

## 9. Cleanup status and diagnostic files

Processes I started — provider harness (630602), consumer harnesses (630648, 631490, 632055, 632729), isolated daemons (631882, 635056, 638655) — are all stopped and verified gone by PID; no `zz-invest` or scratchpad-binary processes remain, and the isolated `network.sock` is removed. The dev controller on `:18081` was left running and was never signalled by me.

Controller-side test resources are deleted and verified: org `org_zduphrlnp6wt`, accounts `ac_9vz7zglp058o` and `ac_mdvlvy3elowu`, workgroup `wg_prxdr4i9ju4w`, advertisement `adv_yw6o0xhmfgye`, three environments, and the five leaked tunnels from defect A. The closed session rows remain, as sessions are never hard-deleted.

Locally: the detached worktree is removed and pruned, the `/tmp/zzi` symlink and the throwaway environment roots (including the files that held test account tokens) are deleted. The repository working tree contains only this document, the plan, a journal entry, and two roadmap cards — all uncommitted.

Left for review in the session scratchpad (session-scoped; copy out anything worth keeping) under `/tmp/claude-1000/-home-michael-Repos-nf-agora/34bce8f3-3e3c-4818-9523-d4f29eeabd31/scratchpad/`:

- `diagnostics/zz_investigation_test.go` — the mocked fixture (drops into `sdk/agent/`)
- `diagnostics/zz-invest-main.go` — the provider/consumer harness (drops into `cmd/zz-invest/`)
- `evidence/` — harness and daemon logs, the `network.json` snapshot after accumulation, and the leaked-tunnel listing. Checked for account tokens; none present. Logs contain local paths and test identity names only

Neither diagnostic file is a proposed production patch.
