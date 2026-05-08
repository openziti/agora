# commodities-feed

Agent in `markets-co`. Publishes commodity price data (WTI crude, gold, Henry Hub natural gas) via session-backed envelopes.

## Capability

Advertises `markets.commodities` in workgroup `markets-channel`.

Advertisement metadata:

```
name:        commodities-feed
capability:  markets.commodities
description: Front-month commodity spot prices and recent history
message_types: [markets.commodities.request, markets.commodities.response]
```

## Workgroups

- `markets-channel` (inter-org)
- `markets-internal` (intra-org; unused in MVP)

## Envelope shapes

Request (`markets.commodities.request`):

```json
{
  "symbols": ["CL=F", "GC=F", "NG=F"],
  "window_days": 7
}
```

`CL=F` = WTI crude front-month, `GC=F` = gold front-month, `NG=F` = Henry Hub natural gas front-month.

Response (`markets.commodities.response`):

```json
{
  "as_of": "2026-04-22T08:00:00Z",
  "window_days": 7,
  "symbols": {
    "CL=F": {"price":  78.42, "change_pct": -3.5, "series": [...]},
    "GC=F": {"price": 2418.70, "change_pct": +2.1, "series": [...]},
    "NG=F": {"price":   2.18, "change_pct": +1.4, "series": [...]}
  }
}
```

## Contract expectations

Same shape as other `markets-co` feeds.

## Run modes

- **Snapshot** (default): reads `examples/macro-pulse/snapshots/commodities/<symbol>.json`.
- **Live** (`--live`): Yahoo Finance futures endpoints. Falls back to snapshot on failure.

## Implementation status

- Shipped. Publishes its advertisement on startup, accepts governed sessions, and serves typed request/response envelopes from snapshot data or Yahoo Finance live data.
