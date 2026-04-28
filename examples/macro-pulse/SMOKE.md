# Macro Pulse Smoke Procedure

End-to-end run of the Macro Pulse demo against a live Agora controller and OpenZiti fabric. Confirms that all five Layer 2 slices (workgroups, catalog/advertisements, sessions, contracts, envelopes) work in practice — not just under the mocked-lifecycle integration tests.

## Prerequisites

- **PostgreSQL** reachable at `localhost:5432` with role `agora` and database `agora`.
- **OpenZiti controller** reachable at `https://localhost:1280` (admin/admin in the example config below). The fabric must have at least one enrolled edge router. The official `openziti/ziti-controller` quickstart container is sufficient.
- **`agora` binary** built from this repo (`go install ./cmd/agora`).
- **Macro Pulse agent binaries** built from this repo (`go install ./examples/macro-pulse/cmd/...`). The 9 binaries are: `macro-pulse-equity-feed`, `macro-pulse-fx-feed`, `macro-pulse-commodities-feed`, `macro-pulse-weather-feed`, `macro-pulse-search-trends`, `macro-pulse-news-pulse`, `macro-pulse-correlator`, `macro-pulse-narrator`, `macro-pulse-pulse-agent` — plus `macro-pulse-bootstrap` for topology provisioning.

The procedure assumes `$GOBIN` is on `$PATH`.

## Procedure

### 1. Apply schema migrations

```bash
agora admin store migrate etc/dev-agora-controller.yaml up
```

If the database has an older schema, drop and recreate first:

```bash
psql 'postgres://agora:agora@localhost:5432/agora?sslmode=disable' \
  -c "drop schema public cascade; create schema public; grant all on schema public to agora;"
agora admin store migrate etc/dev-agora-controller.yaml up
```

### 2. Start the controller

```bash
agora controller etc/dev-agora-controller.yaml &
curl -sf http://127.0.0.1:18081/health   # expect: 200
curl -sf http://127.0.0.1:18081/ready    # expect: 200
```

### 3. Bootstrap OpenZiti

```bash
agora admin bootstrap etc/dev-agora-controller.yaml
# expect: "bootstrap complete!"
```

### 4. Bootstrap the Macro Pulse topology

```bash
export AGORA_ADMIN_TOKEN=dev-admin-token
macro-pulse-bootstrap -controller http://127.0.0.1:18081 | tee /tmp/macro-pulse-bootstrap.log
```

The bootstrap creates 5 organizations, 9 accounts, 7 workgroups (3 intra-org + 4 inter-org with auto-accepted invitations), and adds the non-admin agents in each org as workgroup members. It prints one account token per agent at the end of its log; capture them.

### 5. Enroll one environment per agent

Each agent needs its own enrolled environment with a distinct ziti identity, so each is given its own root directory. The CLI uses `$HOME/.agora/` as the env root, so isolating by `$HOME` is the simplest approach:

```bash
declare -A TOKENS=( … )    # populate from step 4

for name in "${!TOKENS[@]}"; do
  root=/tmp/mp-roots/$name
  mkdir -p "$root"
  HOME="$root" agora config set api_endpoint http://127.0.0.1:18081
  HOME="$root" agora enable "${TOKENS[$name]}" --description "$name"
done
```

After this each `/tmp/mp-roots/<name>/.agora/` contains the agent's enrolled identity and account token.

### 6. Launch the 8 provider/tool agents

The macro-pulse agent binaries use the SDK's `--env-root` / `AGORA_ENV_ROOT` flag for isolation:

```bash
mkdir -p /tmp/mp-logs
for name in equity-feed fx-feed commodities-feed weather-feed \
            search-trends news-pulse correlator narrator; do
  AGORA_ENV_ROOT=/tmp/mp-roots/$name/.agora \
    "$(go env GOBIN)/macro-pulse-$name" > /tmp/mp-logs/$name.log 2>&1 &
done
```

Each provider:
1. Loads its env, opens a controller client.
2. Ensures a shared `macro-pulse-provider-default` contract (`max_duration_seconds=60`, `allowed_message_types=["*"]`, `access_mode=approval_required`).
3. Publishes its advertisement (idempotent on `409 name_in_use`).
4. Spins up an `agentutil.EchoSessionHandler` via `session.RegisterHandler` to accept inbound proposals, read one envelope, and reply with `.request` → `.response`.

Verify each published its ad:

```bash
for name in equity-feed fx-feed commodities-feed weather-feed \
            search-trends news-pulse correlator narrator; do
  ad=$(grep -oE 'adv_[a-z0-9]+' /tmp/mp-logs/$name.log | head -1)
  echo "$name: ad=$ad"
done
```

### 7. Run the pulse-agent

```bash
AGORA_ENV_ROOT=/tmp/mp-roots/pulse-agent/.agora \
  "$(go env GOBIN)/macro-pulse-pulse-agent" > /tmp/mp-logs/pulse-agent.log 2>&1
```

Pulse-agent searches the catalog (8 advertisements visible across the four inter-org channels), and for each one: proposes a session, sends a `<capability>.request` envelope with a small JSON payload, receives the `<capability>.response` reply, logs the snapshot of the contract terms it ran under, and closes the session. The full demo completes in ~15 seconds.

### 8. Validate

```bash
# 8 sessions reached active and exchanged a reply envelope:
grep -c 'received reply envelope' /tmp/mp-logs/pulse-agent.log     # → 8

# 8 providers each accepted exactly one session and echoed:
for n in equity-feed fx-feed commodities-feed weather-feed search-trends news-pulse correlator narrator; do
  echo "$n: echoes=$(grep -c 'received request; echoing response' /tmp/mp-logs/$n.log)"
done

# Controller-side: 8 tunnels provisioned, 8 sessions accepted, 8 closed:
grep -c 'created tunnel'  /tmp/agora-ctrl.log
grep -c 'accepted session id=' /tmp/agora-ctrl.log
grep -c 'closed session id='   /tmp/agora-ctrl.log

# No controller-side errors:
grep -c '"level":"ERROR"' /tmp/agora-ctrl.log
```

A green run looks like:

```
provider:  equity-feed (and 7 more) → ad=adv_…
sessions:  8 received reply envelope
providers: 8 echoes
controller: 8 tunnels, 8 accepts, 8 closes, 0 errors
```

## Troubleshooting

- **`environment is not enabled (run agora enable …)`** — `AGORA_ENV_ROOT` is unset or wrong. Each agent must point at its own `/tmp/mp-roots/<name>/.agora` directory.
- **`workgroup "X" not visible to this agent`** — bootstrap step 4 didn't add the agent as a member of that workgroup. Verify against `agora workgroup list` from that agent's env. Re-running bootstrap on a stale DB skips member-add because account tokens are only returned on first creation; in that case wipe the DB and re-bootstrap.
- **`session propose failed: attach transport: ensure connect: tunnel not found`** — usually means the controller and agents were built from different commits, or the tunnel-grant cross-org lookup didn't take effect. Restart both controller and agents from the latest binary.
- **Controller error `decode application/json: alias: invalid: code (field required), message (field required)`** — historical bug where `Root.Client()` returned an unauthenticated client and ogen mis-decoded the controller's `{error_message}` shape. Fixed in this repo via `accountTokenSecuritySource`. If you see this on a stale binary, rebuild.

## Single-command smoke

A `bin/smoke-macro-pulse.sh` script that wraps steps 1–8 is a useful follow-up. The current procedure is documented at the granularity needed to debug each phase.
