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

# Snapshot mode (default): every brief reproducible, no network beyond ziti.
for name in equity-feed fx-feed commodities-feed weather-feed \
            search-trends news-pulse correlator narrator; do
  AGORA_ENV_ROOT=/tmp/mp-roots/$name/.agora \
    "$(go env GOBIN)/macro-pulse-$name" > /tmp/mp-logs/$name.log 2>&1 &
done

# OR — live mode: each data feed hits a free public API and falls
# back to the embedded snapshot on any upstream failure.
for name in equity-feed fx-feed commodities-feed weather-feed \
            search-trends news-pulse correlator narrator; do
  AGORA_ENV_ROOT=/tmp/mp-roots/$name/.agora \
    "$(go env GOBIN)/macro-pulse-$name" --live > /tmp/mp-logs/$name.log 2>&1 &
done
```

Each provider:
1. Loads its env, opens a controller client.
2. Ensures a shared `macro-pulse-provider-default` contract (`max_duration_seconds=60`, `allowed_message_types=["*"]`, `access_mode=approval_required`).
3. Publishes its advertisement (idempotent on `409 name_in_use`).
4. Registers a typed request/response session handler. In snapshot mode the handler reads from the embedded JSON; in `--live` mode it tries the upstream first (Yahoo Finance / Open-Meteo / Wikipedia Pageviews / GDELT) and falls back to the snapshot on any error.

`correlator` and `narrator` ignore `--live` (pure compute, no upstream).

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

The pulse-agent's stdout is the brief itself. Operational logs go to stderr and stay out of the way.

```bash
# Capture the brief and inspect it:
cat /tmp/mp-logs/brief.txt

# Controller-side: 8 tunnels provisioned, 8 sessions accepted, 8 closed:
grep -c 'created tunnel'        /tmp/agora-ctrl.log
grep -c 'accepted session id='  /tmp/agora-ctrl.log
grep -c 'closed session id='    /tmp/agora-ctrl.log

# Each provider served one capability call:
for n in equity-feed fx-feed commodities-feed weather-feed search-trends news-pulse correlator narrator; do
  echo "$n: served=$(grep -c '"served request"\|"served weather request"' /tmp/mp-logs/$n.log)"
done

# No controller-side errors:
grep -c '"level":"ERROR"' /tmp/agora-ctrl.log
```

A green run produces a brief like this on pulse-agent's stdout:

```
=== Agora Macro Pulse — 2026-04-28 18:02 UTC ===

MARKETS (7d)
  S&P 500            529.11       -1.2%
  XLK (tech)         218.42       -1.8%
  XLE (energy)       92.18        +2.4%
  XLF (financials)   47.32        -0.7%
  USD/EUR            1.0843       +0.3%
  USD/JPY            150.42       -0.1%
  WTI crude          $78.42       -3.5%
  Gold               $2418.30     +2.1%
  Nat gas            $2.18        +1.4%

WEATHER (current + 72h, economic hubs)
  new-york               clear, cold snap, -8.4°F vs seasonal
  houston                tropical depression forming, Gulf coast, +2.1°F vs seasonal
  frankfurt              above-average heat, low humidity, +11.6°F vs seasonal
  singapore              partly cloudy, seasonal, +0.2°F vs seasonal
  houston (72h forecast) tropical depression intensifying → tropical storm, heavy rain → storm landfall, sustained winds

SIGNALS (search 30d, news 7d)
  Search "layoffs":          +34.0%
  Search "gulf-storm":       +212.1%
  Search "housing-market":   +7.0%
  News tone (financial):     -0.18
  News tone (energy):        +0.21

CORRELATIONS (30d, |r| > 0.4)
  WTI crude vs gulf-storm search:      r = -0.82
  S&P 500 vs layoffs search:           r = -0.91

BRIEF
  Markets show gold bid suggests defensive positioning and equity softness.
  Energy complex faces near-term upside risk from Gulf weather. Rising
  employment-anxiety signals align with equity softness. Strongest
  cross-domain signal: S&P 500 vs layoffs search (r=-0.91).

CATALOG
  commodities-feed     adv_…  caps=[markets.commodities]
  correlator           adv_…  caps=[analytics.correlate]
  equity-feed          adv_…  caps=[markets.equity]
  fx-feed              adv_…  caps=[markets.fx]
  narrator             adv_…  caps=[analytics.narrate]
  news-pulse           adv_…  caps=[signals.news]
  search-trends        adv_…  caps=[signals.search]
  weather-feed         adv_…  caps=[weather.current weather.forecast]
```

`macro-pulse-pulse-agent --json` emits the same data as a JSON document on stdout instead of formatted text.

`macro-pulse-pulse-agent --markdown <path>` writes a GitHub-flavored Markdown rendering of the brief to `<path>` (in addition to whatever it prints on stdout). The markdown form uses tables for Markets and Catalog and bullet lists for Weather, Signals, and Correlations — handy for committing a daily snapshot to a repo, pasting into an internal page, or publishing through a `README.md`-style render path.

Example markdown output (snapshot mode):

```markdown
# Agora Macro Pulse — 2026-04-28 20:13 UTC

Composed across independent provider and tool agents over Agora-governed sessions. …

## Markets (7d)

| Asset | Value | Change |
| --- | ---: | ---: |
| S&P 500 | 529.11 | -1.2% |
| WTI crude | $78.42 | -3.5% |
| Gold | $2418.30 | +2.1% |
| …

## Correlations (30d, |r| > 0.4)

| Pair | Pearson r |
| --- | ---: |
| WTI crude vs gulf-storm search | -0.82 |
| S&P 500 vs layoffs search | -0.91 |

## Brief

Markets show gold bid suggests defensive positioning and equity softness. Energy
complex faces near-term upside risk from Gulf weather. …

## Catalog

| Advertisement | ID | Capabilities |
| --- | --- | --- |
| equity-feed | `adv_…` | markets.equity |
| weather-feed | `adv_…` | weather.current, weather.forecast |
| …

---
_Generated by `macro-pulse-pulse-agent` over 8 advertisements; deterministic narrator (no LLM)._
```

Every section lands because nine real OpenZiti tunnels were provisioned, nine real Agora sessions were proposed/accepted/closed, and nine real envelope round-trips carried the per-capability JSON payloads through the fabric.

## Live mode

When the data feeds are launched with `--live`, each pulls from its public-API upstream first and falls back to the embedded snapshot on any error. The 9-agent topology, the catalog, the workgroups, the contract, the session lifecycle, and the envelope wire format are all unchanged — only the bytes inside the response payloads differ.

| Capability | Upstream | Notes |
| --- | --- | --- |
| `markets.equity` | Yahoo Finance `query1.finance.yahoo.com/v8/finance/chart/<symbol>?interval=1d&range=...` | `SPY`, `XLK`, `XLE`, `XLF` map 1:1 |
| `markets.fx` | Yahoo Finance same endpoint | `USD-EUR` → `EURUSD=X` (inverted), `USD-JPY` → `JPY=X`, `USD-GBP` → `GBPUSD=X` (inverted) |
| `markets.commodities` | Yahoo Finance same endpoint | `CL_F` → `CL=F`, `GC_F` → `GC=F`, `NG_F` → `NG=F` |
| `weather.current` + `weather.forecast` | Open-Meteo `api.open-meteo.com/v1/forecast` | One call per requested city; coordinates hand-curated for `new-york`, `houston`, `frankfurt`, `singapore` |
| `signals.search` | Wikipedia Pageviews `wikimedia.org/api/rest_v1/metrics/pageviews/per-article/...` | Each Macro-Pulse term maps to a representative article (`layoffs` → `Layoff`, `gulf-storm` → `Tropical_cyclone`, `housing-market` → `United_States_housing_bubble`) |
| `signals.news` | GDELT DOC API `api.gdeltproject.org/api/v2/doc/doc?mode=timelinetone` | One request per topic, 6 s between requests to respect GDELT's 1-per-5-second free-tier limit |

Each adapter takes the existing typed request and returns the existing typed response (`internal/payloads`), so nothing downstream needs to know which mode produced the bytes. `analytics.correlate` and `analytics.narrate` are pure compute — they accept `--live` for uniformity but ignore it.

A real live-mode run captured against the running OpenZiti fabric:

```
=== Agora Macro Pulse — 2026-04-28 19:59 UTC ===

MARKETS (7d)
  S&P 500            711.77       +0.2%
  XLK (tech)         157.92       +2.3%
  XLE (energy)       57.72        +4.9%
  XLF (financials)   51.86        -1.1%
  USD/EUR            0.8538       +0.6%
  USD/JPY            159.65       +0.3%
  WTI crude          $99.95       +19.2%
  Gold               $4612.10     -5.0%
  Nat gas            $2.69        +0.6%

WEATHER (current + 72h, economic hubs)
  new-york               overcast, +6.7°F vs seasonal
  houston                thunderstorm, +7.8°F vs seasonal
  frankfurt              clear sky, -1.9°F vs seasonal
  singapore              overcast, -12.2°F vs seasonal
  houston (72h forecast) thunderstorm → drizzle → overcast

SIGNALS (search 30d, news 7d)
  Search "layoffs":          +7.5%
  Search "gulf-storm":       +11.1%
  Search "housing-market":   +2.5%
  News tone (financial):     -0.18
  News tone (energy):        +0.21

CORRELATIONS (30d, |r| > 0.4)
  WTI crude vs gulf-storm search:      r = -0.78

BRIEF
  Energy complex faces near-term upside risk from Gulf weather. Strongest
  cross-domain signal: WTI crude vs gulf-storm search (r=-0.78).

CATALOG
  ...
```

In this run, equity / fx / commodities / weather / search-trends served live data; `news-pulse`'s GDELT request hit the free-tier rate limit and the provider gracefully fell back to its snapshot (the news-tone lines are snapshot values). That is the documented behavior — the demo never breaks because of upstream issues. Confirm which feed served live vs. fell back:

```bash
for n in equity-feed fx-feed commodities-feed weather-feed search-trends news-pulse; do
  succ=$(grep -c 'live fetch succeeded' /tmp/mp-logs/$n.log)
  fail=$(grep -c 'live fetch failed'    /tmp/mp-logs/$n.log)
  echo "$n: live=$succ fallback=$fail"
done
```

## Troubleshooting

- **`environment is not enabled (run agora enable …)`** — `AGORA_ENV_ROOT` is unset or wrong. Each agent must point at its own `/tmp/mp-roots/<name>/.agora` directory.
- **`workgroup "X" not visible to this agent`** — bootstrap step 4 didn't add the agent as a member of that workgroup. Verify against `agora workgroup list` from that agent's env. Re-running bootstrap on a stale DB skips member-add because account tokens are only returned on first creation; in that case wipe the DB and re-bootstrap.
- **`session propose failed: attach transport: ensure connect: tunnel not found`** — usually means the controller and agents were built from different commits, or the tunnel-grant cross-org lookup didn't take effect. Restart both controller and agents from the latest binary.
- **Controller error `decode application/json: alias: invalid: code (field required), message (field required)`** — historical bug where `Root.Client()` returned an unauthenticated client and ogen mis-decoded the controller's `{error_message}` shape. Fixed in this repo via `accountTokenSecuritySource`. If you see this on a stale binary, rebuild.

## Single-command smoke

A `bin/smoke-macro-pulse.sh` script that wraps steps 1–8 is a useful follow-up. The current procedure is documented at the granularity needed to debug each phase.
