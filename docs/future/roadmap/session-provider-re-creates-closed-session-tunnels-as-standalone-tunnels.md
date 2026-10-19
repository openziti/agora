---
title: Session provider re-creates closed session tunnels as standalone tunnels
state: inbox
created: 2026-09-18
tags: [defect, spike]
source: docs/future/layer-2/persisted-session-connects-investigation-findings.md
log: docs/journal/2026-09-18.md
---

After an SDK-accepted session closes, the provider's embedded runtime still holds the serve actor that `attachProviderStream` registered (`sdk/agent/session/transport.go`; `teardownTransport` never calls `RemoveServe`). The actor keeps retrying `StartServe`, which resolves by name through `ensureTunnelCreatedOrReused` (`sdk/agent/controller_client.go`), so once the controller deletes the session tunnel the retry creates a new account-owned standalone proxy tunnel named `session-<ses_id>` with a fresh `tt_` id. Reproduce with any `session.RegisterHandler` provider: run one session to close, then `agora tunnel list` as the provider account. Stop the session transport from leaving a serve actor behind, and decide whether a serve retry should ever be allowed to create a tunnel that carries a session name.

## why

Unlike the consumer-side in-memory leak, this writes durable controller state: the tunnels outlive the provider process, stay `active`, block `agora admin delete user` ("account still owns standalone tunnels"), and let the name `session-<ses_id>` resolve again after the session is closed. Observed for 5 of 5 sessions on 2026-09-18 against a HEAD (`ee1597d`) controller.

## background

Found during cleanup of the persisted-session-connects investigation; recorded there as adjacent defect A and deliberately not pursued. The spike tag covers the open design question (create-on-missing in `StartServe` is intended behavior for `agora tunnel serve`), not the reproduction, which is deterministic.
