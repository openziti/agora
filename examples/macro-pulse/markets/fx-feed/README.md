# fx-feed

Agent in `markets-co`. Publishes major-pair FX rate data via session-backed envelopes.

## Capability

Advertises `markets.fx` in workgroup `markets-channel` (inter-org; shared with `enterprise-client`).

Advertisement metadata:

```
name:        fx-feed
capability:  markets.fx
description: Major currency pair spot rates and recent history
message_types: [markets.fx.request, markets.fx.response]
```

## Workgroups

- `markets-channel` (inter-org)
- `markets-internal` (intra-org; unused in MVP)

## Envelope shapes

Request (`markets.fx.request`):

```json
{
  "pairs": ["USD/EUR", "USD/JPY", "USD/GBP"],
  "window_days": 7
}
```

Response (`markets.fx.response`):

```json
{
  "as_of": "2026-04-22T08:00:00Z",
  "window_days": 7,
  "pairs": {
    "USD/EUR": {"rate": 1.0842, "change_pct": +0.3, "series": [...]},
    "USD/JPY": {"rate": 148.12, "change_pct": -0.6, "series": [...]},
    "USD/GBP": {"rate": 0.7912, "change_pct": +0.1, "series": [...]}
  }
}
```

`series` is a daily rate array supporting downstream correlation.

## Contract expectations

Same contract shape as `equity-feed` (`max_duration=60s`, `max_envelope_count=4`, `allowed_message_types=[markets.fx.request, markets.fx.response]`).

## Run modes

- **Snapshot** (default): reads `examples/macro-pulse/snapshots/fx/<pair>.json`.
- **Live** (`--live`): fetches from Yahoo Finance FX endpoints. Falls back to snapshot on failure.

## Implementation status

- Slice-gated: advertisement, session, envelope slices
- Pre-slice: `main.go` skeleton only.
