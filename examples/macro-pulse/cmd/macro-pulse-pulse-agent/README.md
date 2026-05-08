# pulse-agent

Agent in `enterprise-client`. The orchestrator. Takes a configuration, discovers providers and tools via the catalog, establishes bounded sessions under contracts, exchanges envelopes, produces the morning brief.

## Capability

`pulse-agent` does **not** advertise any capability. It is a consumer-only agent. Nothing in the catalog points to it; it is the party doing the pointing.

## Workgroups

Member of all four `enterprise-client`-side workgroups:

- `markets-channel`
- `weather-channel`
- `signals-channel`
- `analytics-channel`

Membership lets `pulse-agent` discover advertisements from each provider/tool. Providers and tools in the other four organizations do not see `pulse-agent` as an advertisement — they only see it when it opens a session with them.

## Orchestration flow

Given a configuration (which tickers, cities, search terms, etc.), the agent runs the following pipeline. Each step is a separate session over the provider's advertisement.

1. **Discover & fetch markets** — catalog search for `markets.*`; open sessions with `equity-feed`, `fx-feed`, `commodities-feed`; request current + 7d-series data; close sessions.
2. **Discover & fetch weather** — catalog search for `weather.*`; open session with `weather-feed`; request current + 72h forecast for the configured cities; close.
3. **Discover & fetch signals** — catalog search for `signals.*`; open sessions with `search-trends` and `news-pulse`; request tracked terms / topics; close.
4. **Select interesting pairs** — internal logic: combinations of series most likely to reveal useful correlations (e.g. energy commodities vs weather-derived signals; equity indices vs search-volume deltas). Pair construction is local to `pulse-agent`.
5. **Run correlations** — for each selected pair, open a session with `correlator` with a per-pair contract (`max_envelope_count=2`, `max_duration=10s`), send both series, receive `pearson_r`. Close session.
6. **Narrate** — open a session with `narrator`, send a summary payload (markets + weather + signals + significant correlations), receive the brief paragraph block. Close session.
7. **Emit** — print the full Macro Pulse brief to stdout.

Every session has a contract-bounded envelope count. The agent does not keep long-lived sessions; one session per logical request.

## Configuration

CLI flags (finalized during implementation):

- `--tickers` — default `SPY,XLK,XLE,XLF`
- `--fx-pairs` — default `USD/EUR,USD/JPY,USD/GBP`
- `--commodities` — default `CL=F,GC=F,NG=F`
- `--cities` — default `New York,Houston,Frankfurt,Singapore`
- `--search-terms` — default `layoffs,gulf storm,housing market`
- `--news-topics` — default `financial,energy,supply chain`
- `--live` — propagate `--live` to data agents? (No — that is a provider-side flag; the client doesn't know nor care whether the provider is using snapshots or live data.)

## Output

Human-readable text to stdout. Format documented in [../../README.md](../../README.md) "Expected output." Pass `--json` to emit the same brief data as JSON.

## Contract expectations

As a consumer, `pulse-agent` is not advertising and has no own contract. It proposes contracts on each session it opens; the specific contract varies per provider (see each provider's README).

## Run modes

One mode. The agent is ignorant of whether any given data provider is running in snapshot or live mode — that is the provider's choice and is opaque to sessions.

## Implementation status

- Shipped. Discovers the visible catalog, opens governed sessions to every provider/tool, exchanges typed request/response envelopes, computes cross-domain correlations through `correlator`, asks `narrator` for deterministic prose, and prints the full brief.
