# Bulk Removal of Local Tunnel Connects: Work Order

Status: draft, 2026-10-19. Implements the [spec](tunnel-connect-bulk-removal.md). Grounded at `33a85e3`; recheck the tree before editing. One implementation stage is sufficient.

## Existing integration points

| File | Relevant behavior |
|---|---|
| `cmd/agora/tunnelDelete.go` | `newTunnelDeleteConnectCommand` requires `--listen`; `run` chooses daemon removal or offline fallback. `drainManagedTunnelState` already demonstrates list-and-remove composition, but also removes serves and is unsuitable for reuse wholesale |
| `cmd/agora/tunnel.go` | `isTunnelID`, `removeLocalDesiredConnect`, and `removeLocalDesiredTunnel`; the latter removes serves too and must not be used for this command |
| `cmd/agora/network.go` | `openNetworkClient` probes the local daemon and supplies its client |
| `sdk/agent/agent.go` | `ListConnects` returns current managed-connect statuses |
| `sdk/agent/tunnel_runtime.go` | `RemoveConnect` removes desired state, persists it, and stops the actor; removal can identify by tunnel/name plus listen address |
| `sdk/agent/networkpb/network.proto` | Existing `ListConnects` and `RemoveConnect` RPCs suffice; no protocol regeneration needed |

## Stage 1: Command, offline removal, and verification

### Command and selection

Add the boolean `--all` flag to the existing connect-delete leaf. Replace unconditional Cobra `MarkFlagRequired("listen")` with validation of the two modes before loading an environment or probing the daemon. Reject both flags together, including an explicitly supplied `--listen` with bulk mode, and neither mode; preserve existing listen-address validation. Keep existing command-group wiring. Use `RunE`/returned errors for this leaf if needed to report bulk failures cleanly; do not refactor unrelated commands.

Put a small reference-matching helper in the CLI package and use it for both the daemon snapshot and offline records. For an ID token, match the stored tunnel ID; only when that ID is absent, also match the exact literal token in the saved name. For a name token, use `strings.EqualFold` against the saved name. Do not map a name to an ID by looking up another row or the controller. When converting daemon statuses, use their saved desired fields and resolved tunnel ID; do not depend on a live attachment ID for selection.

Keep all-address selection explicit. Do not overload an empty listen address in the existing SDK/RPC removal API to mean all addresses.

### Daemon running

After the existing initial daemon probe succeeds, call `ListConnects` once. For every selected record, invoke `RemoveConnect` with its listen address and exactly one identifier: its resolved tunnel ID if present, otherwise its saved name. Leave `AttachmentId` empty: heartbeat recovery can replace the attachment between listing and removal. Do not pass both tunnel ID and name: the runtime's matcher treats them as alternatives, which can select another tunnel under a reused name. Use each record's own fields rather than blindly reusing the user's token, so unresolved ID-as-name records remain removable.

Count completed removals. Stop and return a descriptive error on a failed removal, retaining the completed count in the output. Do not suppress errors by matching their text. If a concurrent operation has already removed an entry, returning the current removal error is acceptable; rerunning bulk cleanup is safe. Do not fall back to editing disk after a successful probe followed by a listing/removal failure.

Use a bounded context for each list/remove RPC (10 seconds). A timeout may occur after the daemon has already changed state, so report that completion is unconfirmed for the failed request and that the command can be rerun. Do not claim rollback. Existing `stopConnectActor` can make a controller cleanup request even after local removal; this change does not redesign runtime teardown or guarantee a healthy controller. Controller errors already logged by that cleanup must not restore removed desired state.

### Daemon stopped

Use the same initial unavailable-daemon routing as the existing single-address command. Add a dedicated local connect-only bulk helper; do not call `removeLocalDesiredTunnel`, which also removes serves.

Clone the loaded network state, filter selected connects, and write once. Preserve every retained record in full, including its attachment recovery token, and preserve all serves. Delete `network.json` only when both serves and connects are empty. No matches requires no write and returns zero. Propagate persistence errors and only report success after the write/delete succeeds. Do not modify the loaded slices while constructing the filtered result.

### Output

Bulk success reports `deleted N managed connect(s) for '<reference>'`; zero reports `no managed connects matched '<reference>'`. Errors identify the failed operation and, for a partial online removal, how many removals were confirmed. Keep the existing single-address success message. Explain in flag/help text that `--all` means every matching listen address, including active connections, and that selection is local.

### Tests

Add focused CLI tests in `cmd/agora/tunnelDelete_test.go`. Use narrow helpers or an interface over the generated local client where needed; no generalized CLI service framework. Follow the existing temporary-root tests for offline persistence, and keep tests that change the environment package's global root non-parallel.

Cover the behavior that could remove too much or leave rows behind:

- Flag validation rejects both/neither without accessing the environment; valid single-address and bulk forms route correctly, including `--all=false`.
- Multiple ports and saved aliases sharing a resolved ID are selected by ID. Name selection is case-insensitive and does not expand to other aliases. An unresolved row named by the literal ID is selected too.
- A different tunnel on the same port, unrelated connects, and all serves survive. Retained attachment recovery tokens are unchanged.
- Online enumeration sends the expected per-row `RemoveConnect` requests, without attachment IDs or controller resolution. Listing failure performs no removals; failure after one successful removal reports partial progress and does not write the local file. No matches succeeds.
- Offline filtering persists exactly the retained state, handles a missing network file, preserves a serve-only file, deletes a truly empty file, and surfaces persistence failure. Repeat cleanup to prove no-op success.

Add one runtime-backed regression using the existing `sdk/agent/agent_test.go` fake controller/runtime host or an equivalent local gRPC fixture: remove several selected connects, including a retrying one; observe that their retries stop and they are absent after restoration from persisted state. Preserve an unrelated connect. Make controller tunnel lookup unavailable and attachment cleanup return an error promptly to prove cleanup does not require controller success. Do not change production SDK behavior to make the test easier.

Run the narrow tests first: `go test ./cmd/agora ./sdk/agent`. Run `go vet ./cmd/agora ./sdk/agent`. If implementation changes shared packages or project wiring, also run `go test ./...` per repository policy. The full suite includes Docker-backed persistence tests; report unavailable external dependencies honestly. Never emit a build binary into the working tree. Stop any verification listeners/processes and verify cleanup.

### Documentation and handoff

Update the command-level contract in `docs/current/layer-1/agent.md` and relevant CLI examples to include the two removal modes and local matching semantics. Add an Unreleased CHANGE entry describing bulk local-connect removal, without claiming automatic cleanup or a fix for SDK session teardown.

Run `unfurl -i` on edited markdown and `git diff --check`. Review this single stage through Terminus under the repository's workflow before handoff. Leave changes uncommitted for Michael. On accepted completion, synthesize the built behavior into current docs and retire this spec/work order following the docs convention.

## Scope guard

No new protobuf or REST fields, database migration, persistence schema, runtime ownership abstraction, background pruning, session polling, tunnel-list filtering, or connector changes. No modification of retry policy. Avoid unrelated repair of SDK teardown, existing offline/daemon concurrency, or whole-tunnel deletion. If a concrete failure prevents this command's contract from working, surface that evidence before expanding the work.
