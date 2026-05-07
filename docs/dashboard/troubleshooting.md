# Agora Dashboard Demo Troubleshooting

Use this when the demo does not start cleanly or the dashboard does not show the expected activity.

## Fast Reset

For most failed starts:

```sh
./bin/demo-down.sh
./bin/demo-up.sh
```

If the demo state is stale or mismatched with the current code:

```sh
./bin/demo-down.sh --purge
./bin/demo-up.sh
```

`--purge` deletes `$AGORA_DEMO_ROOT` and forces fresh enrollment on the next run. It is slower, but it removes drift.

## Port 18080 Is Already In Use

Symptom:
- `demo-up.sh` reports that port `18080` is already listening.
- The browser opens a page that is not the Agora dashboard.

Fix:
1. Stop the managed demo stack:

   ```sh
   ./bin/demo-down.sh
   ```

2. If something else owns the port, find it:

   ```sh
   ss -ltnp 'sport = :18080'
   ```

   If `ss` is unavailable:

   ```sh
   lsof -iTCP:18080 -sTCP:LISTEN -Pn
   ```

3. Stop the conflicting process, or run the demo on another controller bind address by editing `etc/demo-controller.yaml` and setting `AGORA_DEMO_CONTROLLER_URL` consistently.

## Docker Or External Services Are Not Running

Symptom:
- Migration fails before the controller starts.
- Controller log shows Postgres connection failures.
- Bootstrap or worker startup fails while talking to OpenZiti.

Fix:
1. If your Postgres or OpenZiti services run in containers, start Docker Desktop or the Docker daemon first.
2. Start the external Postgres and OpenZiti services used by your local demo environment.
3. Confirm `etc/demo-controller.yaml` points at the right endpoints:
   - Postgres DSN under `store.dsn`
   - OpenZiti controller under `open_ziti.api_endpoint`
4. Re-run:

   ```sh
   ./bin/demo-up.sh
   ```

The demo scripts manage Agora, Macro Pulse workers, optional gateways, PID files, and logs. They do not start or stop Postgres or OpenZiti.

## Controller Does Not Become Ready

Symptom:
- `demo-up.sh` reports that the controller did not become ready within 10 seconds.

Fix:
1. Read the controller log:

   ```sh
   tail -120 "${AGORA_DEMO_ROOT:-$HOME/.agora-demo}/logs/controller.log"
   ```

2. Check for the common causes:
   - bad Postgres DSN
   - Postgres not running
   - OpenZiti endpoint or credentials wrong
   - port conflict

3. Stop any partial run and try again:

   ```sh
   ./bin/demo-down.sh
   ./bin/demo-up.sh
   ```

## Missing Go Binaries On PATH

Symptom:
- `demo-up.sh` says `go install` completed but one or more binaries are not on `PATH`.

Fix:
1. Add your Go install directory to `PATH`. Common locations:

   ```sh
   export PATH="$PATH:$(go env GOPATH)/bin"
   ```

   If `GOBIN` is set:

   ```sh
   export PATH="$PATH:$(go env GOBIN)"
   ```

2. Re-run:

   ```sh
   ./bin/demo-up.sh
   ```

## UI Build Fails

Symptom:
- `npm ci` or `npm run build` fails.
- `demo-up.sh` reports that `ui/dist` is missing or empty.

Fix:
1. Run the UI build directly to see the full error:

   ```sh
   cd ui
   npm ci
   npm run build
   ```

2. Return to the repo root and re-run the demo:

   ```sh
   cd ..
   ./bin/demo-up.sh
   ```

## Browser Opens But Shows The Login Screen Again After Login

Symptom:
- Valid credentials appear to submit, but the app returns to `/login`.

Fix:
1. Confirm the credentials from the terminal output:
   - Email: `demo@agora.local`
   - Password: `Agora-Demo-1`
2. Confirm the controller is still ready:

   ```sh
   curl -fsS http://localhost:18080/ready
   ```

3. If the controller is ready, clear site cookies for `localhost:18080` and log in again.
4. If the controller is not ready, stop and restart the demo:

   ```sh
   ./bin/demo-down.sh
   ./bin/demo-up.sh
   ```

## Dashboard Loads But Has Empty Activity

Symptom:
- Dashboard renders, but Sessions and activity charts stay empty.
- Catalog has fewer than the expected Macro Pulse advertisements.

Fix:
1. Give the loop 60-90 seconds after startup.
2. Check worker logs:

   ```sh
   ls "${AGORA_DEMO_ROOT:-$HOME/.agora-demo}/logs"/macro-pulse-*.log
   tail -80 "${AGORA_DEMO_ROOT:-$HOME/.agora-demo}/logs/macro-pulse-pulse-agent.log"
   ```

3. Confirm worker PID files exist:

   ```sh
   ls "${AGORA_DEMO_ROOT:-$HOME/.agora-demo}/run/workers"
   ```

4. If workers failed, stop and restart:

   ```sh
   ./bin/demo-down.sh
   ./bin/demo-up.sh
   ```

## Gateway Cards Are Missing

Symptom:
- Catalog shows Macro Pulse cards, but no LLM Gateway or MCP Gateway cards.

Fix:
- This is acceptable for the core demo. `llm-gateway` and `mcp-gateway` are optional external binaries.
- To include them, install the gateway binaries from their repositories and ensure they are on `PATH`, or set:

  ```sh
  export AGORA_LLM_GATEWAY_BIN=/path/to/llm-gateway
  export AGORA_MCP_GATEWAY_BIN=/path/to/mcp-gateway
  ```

Then re-run `./bin/demo-up.sh`.

## Stop Or Clean Up After A Failed Run

Always stop demo processes before changing branches, committing, or starting another run:

```sh
./bin/demo-down.sh
```

Check for leftovers only if something looks wrong:

```sh
pgrep -af 'agora controller|macro-pulse|llm-gateway|mcp-gateway'
```

The script preserves `$AGORA_DEMO_ROOT/envs` and `$AGORA_DEMO_ROOT/logs` by default. That is expected and supports fast restarts and post-failure log inspection.
