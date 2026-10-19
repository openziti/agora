# Persisted Session Connects: Bounded Investigation

Status: investigation plan, 2026-09-18. This is a diagnostic handoff, not an implementation work order or an accepted fix design.

## Objective

Explain how the field user's session-related connects entered the daemon's persisted desired state and why they survived session closure. Establish the causal chain before proposing a fix:

`creating application or command → daemon desired-state write → session closure → surviving connect → restoration after daemon restart`

A demonstration of the SDK's separate in-memory actor leak does not satisfy this objective. Neither does manually declaring a durable connect and showing that it retries after its tunnel disappears; that is already documented Layer 1 behavior.

## Inputs and starting evidence

Read the repository's `AGENTS.md`, [onboarding](../../current/maintainers/onboarding.md), [runtime contract](../../current/layer-1/agent.md), and [session contract](../../current/layer-2/sessions.md). Follow the repository's process cleanup rules.

The original field report is `/home/michael/Sandbox/agora-dead-connect-entries-never-pruned.md`. If it is unavailable on your host, request it from the operator; the following summary is enough to begin local tracing, but is not a replacement for the original evidence.

The reporter observed 18 persisted connects on 2026-09-16, eight showing `tunnel not found`. Two had `session-ses_…` names; others used bare `tt_…` identifiers. Entries survived daemon restart. The runtime was reportedly built from `646cdbf`; the reporter compared relevant source with `ea45285` (v0.1.7). The creating command or application was not established. Names alone do not prove who created a row or who owns its lifecycle.

Initial source inspection in this repository used HEAD `ee1597d` (v0.1.8) with a clean working tree. Recheck these findings against your checkout:

- `sdk/agent/session/transport.go` calls `EnsureConnect` for the consumer and `EnsureServe` for the provider. `teardownTransport` closes the stream and listener without removing either actor. An initial ensure failure can also leave desired state registered before failure is returned.
- Both `agent.App` with `WithRuntime` and `agent.NewStandalone` with `WithRuntime` construct `NewEmbedded`. That runtime has `root: nil`; `persistNetwork` does nothing. `agora session send` uses the embedded path too. Normal session SDK usage therefore does not explain writes to the daemon's `network.json`.
- `agora tunnel connect` delegates to the daemon and records persistent desired state. It passes its positional argument as `Name`, including when that argument is a tunnel ID.
- Per-entry `agora tunnel delete connect … --listen …` bypasses controller tunnel resolution. Whole-tunnel deletion resolves the controller resource before draining local state, so it cannot drain an already-missing tunnel that way.
- `tunnel not found` conflates deletion with authorization failure. Keep retry policy and desired state intact during diagnosis.
- The report calls tunnel deletion a hard delete. At the inspected HEAD, `TunnelsRepository.Delete` in `internal/persistence/repositories.go` sets `deleted = true`. This correction does not explain the local persistence, but do not build a hypothesis on physical row removal.

These are source findings, not a reproduction of the field incident. The newer attachment recovery changes do not by themselves establish that this incident is fixed.

## Scope and operating constraints

Investigate creation, ownership, persistence, and teardown of the reported connects. Do not implement cleanup, change retry semantics, introduce ownership metadata, migrate existing state, or draft a fix spec during this task. Record adjacent defects separately without following them into implementation.

Use isolated environment roots and dedicated test resources. Do not purge or edit the user's real `~/.agora`, reset shared databases, or disrupt an existing daemon. Any state-changing reproduction must use an operator-designated test deployment or an isolated fixture. Postgres and OpenZiti are external prerequisites; request missing access or resources rather than assuming a shared controller is disposable. Keep tokens, private keys, enrollment material, and unsanitized environment files out of evidence logs. Stop all processes you start and verify their PIDs or ports are gone before handoff.

Temporary diagnostic instrumentation or focused reproduction fixtures are allowed in an isolated checkout or temporary directory. Identify them in the handoff; they are not a proposed production patch. Do not commit changes.

## Stage 1: Establish provenance

Record the checkout revision and dirty state. For any executable used in reproduction, record its resolved path and available build metadata; a version string alone may not identify a developer build. Distinguish the daemon executable, CLI executable, and the SDK dependency revision inside the consuming application. Check for module replacements or local modifications when that application is available.

Prepare this concise evidence request for the operator to relay to the reporter; do not contact the reporter directly:

> Which application or command opens these sessions? Please provide its version or SDK dependency revision, the relevant session setup/teardown code or shell commands, and how the daemon is started. Does it invoke `agora tunnel connect`, call the daemon's local gRPC API, or use a wrapper that does either? If practical, capture sanitized `network.json` connect records immediately before and after one session, alongside the matching session/tunnel IDs and daemon status. Please omit credentials and identity files.

Continue the local source trace and control cases while this evidence is pending. Do not treat silence as confirmation of any creation path.

## Stage 2: Trace all candidate writers

Start with `rg` across the repository for `EnsureConnect`, `SetNetwork`, `persistNetwork`, `network.json`, `session-`, and CLI/gRPC client construction. Follow actual call paths rather than assuming the report's writer list is exhaustive.

Inspect these boundaries:

| Boundary | Starting files | Evidence needed |
|---|---|---|
| Session to runtime | `sdk/agent/session/transport.go`, `session.go`, `transport_enforcement.go` | How the runtime is obtained; actor registration and teardown paths |
| Runtime construction | `sdk/agent/app.go`, `standalone.go`, `embedded.go`, `agent.go` | Which construction paths can carry a persistence root or serve local gRPC |
| Desired state to disk | `sdk/agent/state.go`, `tunnel_runtime.go`, `environment/` | Exact write path and fields before/after connection resolution |
| CLI to daemon | `cmd/agora/tunnelConnect.go`, `tunnel.go`, `internal/network/daemon/` | Arguments and requests that create rows; selection of environment root and socket |
| Session closure | `internal/controller/closeSession.go`, session reaping code | Session/tunnel transition and what, if anything, notifies the local owner |
| Field application | Reporter-supplied source, scripts, or invocation trace | The caller that actually reaches the persistent write path |

Compare the relevant paths at `646cdbf`, `ea45285`, and your HEAD where available. Do not infer the application's embedded SDK version from the daemon's revision. If a commit or application source is unavailable, record that limit.

Produce a compact candidate table: caller, runtime construction, disk write possible, supporting evidence, and unresolved point. A `session-` name could be supplied by a CLI caller or custom gRPC caller; it is not evidence that the built-in session SDK used the daemon.

## Stage 3: Run discriminating reproductions

Use one session per initial case. Record before/after connect records, the runtime whose status you queried, session/tunnel IDs, process identity, and timestamps. Observe both session closure and subsequent daemon restart where persistence occurs. Preserve evidence before cleanup.

| Case | Purpose | What counts as a result |
|---|---|---|
| Built-in session SDK with embedded runtime; daemon absent | Test the claimed automatic persistence path | Whether this operation creates or changes `network.json`; separately record retained in-memory actors |
| Same SDK flow with a daemon present against the same isolated enrollment | Test the claimed isolation boundary | Whether the daemon acquires a row or its file changes during SDK session activity |
| Explicit CLI connect through the daemon to a test session's tunnel | Establish the known durable-connect control | Persistent write, retained row after session closure, and restoration after restart; this alone does not explain the field caller |
| Reporter's actual application or exact command sequence | Identify the incident's creation path | Trace from its session action to the specific persistent write, followed by the surviving row |

If live dependencies are unavailable, use focused fixtures to test constructor/persistence boundaries, but label mocked results and leave the field reproduction unverified. Do not present a synthetic gRPC request you authored as evidence that the field application makes that request.

If a supposedly embedded flow writes to disk, instrument the persistence boundary narrowly to identify the process, runtime construction, root path, and caller. If only the explicit CLI control persists, focus further investigation on the application integration rather than broadening the SDK cleanup design.

After identifying a creation path, repeat a short bounded sequence (for example, three sessions) to determine whether rows accumulate. Test only the close path relevant to the report initially. Extend to another close path only if the result requires that distinction. Long churn runs, retry-rate benchmarks, and broad failure injection are unnecessary here.

## Stage 4: Report and stop

Write findings beside this plan as `persisted-session-connects-investigation-findings.md`. Keep observations, source conclusions, and hypotheses distinguishable. Include:

1. Revisions, binary/application provenance, and environment used.
2. The candidate-writer table and results of cases actually run, including negative results.
3. A minimal reproduction and the demonstrated causal chain, if established.
4. Whether the persistent row was session-owned intent, an explicit durable connect, or still of unknown origin; explain the evidence for that classification.
5. The component responsible and the smallest justified next action. Keep the SDK in-memory cleanup defect separate unless the evidence connects it to this incident.
6. Remaining uncertainty and the exact evidence needed to resolve it; process/resource cleanup status and any diagnostic files left for review.

Finish with one of these outcomes:

- **Reproduced and attributed:** the application/command and persistent writer are identified, closure leaves the row, and restart restores it. A fix design can now target that demonstrated path.
- **Integration behavior established:** the application explicitly requests a durable connect, and its ownership/teardown behavior explains persistence. Recommend the responsible integration change or API design discussion without treating the SDK cleanup gap as the cause.
- **Unresolved pending field evidence:** local paths and controls have been checked, but no evidence identifies the creator of the reported rows. State the precise missing input and stop. Do not substitute adjacent fixes or claim the report is disproven.

Run `unfurl -i` on authored markdown before handoff. Leave findings and any authorized diagnostic artifacts uncommitted. This investigation ends at attribution or a concrete evidence limit; implementation is a subsequent task.
