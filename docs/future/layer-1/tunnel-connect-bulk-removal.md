# Bulk Removal of Local Tunnel Connects

Status: draft, 2026-10-19. Implementation plan: [work order](tunnel-connect-bulk-removal-work-order.md).

## Problem and scope

The field user's agora-gateway plugin creates Layer 2 sessions over REST, then invokes `agora tunnel connect` through the shared daemon. Those CLI calls create durable local desired state. When a session ends, its tunnel disappears, but the desired connects survive unless the caller removes them. Failed cleanup and lost connector bookkeeping leave retrying entries behind.

The [investigation](../layer-2/persisted-session-connects-investigation-findings.md) reproduced this persistence. The reporter subsequently identified the plugin as the CLI caller. Its Layer 2 discovery connects by tunnel ID; its Layer 1 discovery can connect to accessible session tunnels by name. Attribution of individual historical name-based rows remains inferred from that code path.

Today per-entry cleanup requires a listen address, while whole-tunnel deletion first resolves the controller tunnel and fails if it is gone. Listen addresses remain recoverable from `agora status`, but requiring one removal per row is unnecessary friction.

Add one explicit local cleanup operation:

```sh
agora tunnel delete connect <name|tt_id> --all
```

This removes matching saved connects across listen addresses and stops their local activity. It does not establish that the target is abandoned: the operator chooses what to remove. It can remove running connections too. It does not delete the controller tunnel or local serves.

## Command contract

The positional reference remains required. Exactly one of `--listen <address>` and `--all` must be supplied. Reject both together and reject neither before performing I/O. An explicit `--all=false` does not select bulk removal. Existing single-address behavior remains unchanged.

Bulk selection uses local records only:

- A tunnel ID selects records with that stored tunnel ID, including records created under different names. It also selects an unresolved record whose saved name is that literal ID, covering connects that failed before resolution.
- A name selects records whose saved name matches case-insensitively, following current name matching. It does not resolve the name through the controller or expand it into other aliases. Use the ID to remove multiple names already associated with one tunnel.
- All listen addresses and runtime states are eligible. There is no prefix, wildcard, port-band, session-name, or error-based matching.

With the daemon running, list its connects and remove the matching snapshot through its existing control API. With the daemon stopped, filter its saved desired state locally. Do not edit the file behind a reachable daemon. No controller lookup or successful controller response is required to select the records. Normal best-effort cleanup of live attachments may still contact the controller through the existing runtime removal path.

No matches is successful and reports that nothing matched. This makes repeated explicit cleanup safe. Report successful removals; return an error if enumeration, persistence, or a removal fails. Online removal is sequential and can partially succeed: report the completed count and the failure rather than claiming the entire target was removed. A later invocation can finish remaining entries.

This is a one-shot operation over the selected local environment. Connections created concurrently or afterward are not covered by a continuing cleanup policy. A successful removal must not return on daemon restart.

## Scenarios

| Situation | Result |
|---|---|
| A deleted tunnel has several saved connects on different ports | One invocation by ID removes all matching entries without resolving the deleted tunnel |
| A tunnel has connects under its ID and `session-ses_…` name | Removal by stored ID selects both; removal by name selects only that saved name |
| An initial connect by ID failed and never acquired a stored tunnel ID | The literal ID saved as its name still permits removal |
| Another tunnel uses the same listen address | Its connection remains |
| A selected connect is currently running | It is explicitly removed and stopped just like a single-address deletion |
| Daemon is stopped, or cleanup is repeated | Saved matches are removed offline; no matches is a successful no-op |

## Boundary decisions

The operator supplies the removal decision. Agora does not infer abandonment from authorization-sensitive lookup failures. The daemon remains the owner of live actors and persistence while running; the CLI composes its existing list/remove operations. The offline path owns only file cleanup. No new controller, session, or ownership model is introduced.

## Deferred (and Why)

Automatic pruning, session-owned attachments, connector ledgers, and SDK actor cleanup are separate changes. This command improves explicit recovery; it does not prevent a connector from abandoning future entries. The connector should retain failed cleanup obligations, but changing that external application is outside this work order.

No new concurrency or transactional bulk-removal guarantee is introduced. Existing daemon/offline lifecycle coordination and runtime teardown behavior remain in place. The independently observed SDK provider recreation defect remains tracked by its [roadmap card](../roadmap/session-provider-re-creates-closed-session-tunnels-as-standalone-tunnels.md).
