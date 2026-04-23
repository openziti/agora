# equity-feed

Agent in `markets-co`. Publishes S&P 500 and major sector index data via session-backed envelopes.

## Capability

Advertises `markets.equity` in workgroup `markets-channel` (inter-org; shared with `enterprise-client`).

Advertisement metadata:

```
name:        equity-feed
capability:  markets.equity
description: Major US equity indices and sector ETFs
message_types: [markets.equity.request, markets.equity.response]
```

## Workgroups

- `markets-channel` (inter-org) — where the agent is discoverable by `enterprise-client`
- `markets-internal` (intra-org) — reserved for future provider-side coordination; unused in MVP

## Envelope shapes

Request (`markets.equity.request`):

```json
{
  "tickers": ["SPY", "XLK", "XLE", "XLF"],
  "window_days": 7
}
```

Response (`markets.equity.response`):

```json
{
  "as_of": "2026-04-22T08:00:00Z",
  "window_days": 7,
  "tickers": {
    "SPY":  {"price": 529.11, "change_pct": -1.2, "series": [...]},
    "XLK":  {"price": 217.34, "change_pct": -2.1, "series": [...]},
    "XLE":  {"price":  92.78, "change_pct": +0.9, "series": [...]},
    "XLF":  {"price":  44.21, "change_pct": -0.4, "series": [...]}
  }
}
```

`series` is a daily closing-price array covering `window_days` back from `as_of`; present in the response so downstream correlation and analytics agents can work directly from the payload without a second round-trip.

## Contract expectations

`pulse-agent` proposes sessions with contract constraints:

- `max_duration = 60s`
- `max_envelope_count = 4`
- `allowed_message_types = [markets.equity.request, markets.equity.response]`

The agent honors these bounds on its side — a malformed request with an out-of-contract message type is rejected.

## Run modes

- **Snapshot** (default): reads `examples/macro-pulse/snapshots/equity/<ticker>.json` and returns the snapshotted series. Fully deterministic.
- **Live** (`--live`): fetches from Yahoo Finance (`query1.finance.yahoo.com/v8/finance/chart/<ticker>`). Falls back to snapshot on any network or parse error.

## Implementation status

- Slice-gated: advertisement slice (publish), session slice (handle sessions), envelope slice (full request/response)
- Pre-slice: `main.go` logs "equity-feed alive" via the `sdk/agent` SDK; exits on SIGTERM.
