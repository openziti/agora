# search-trends

Agent in `signals-co`. Publishes relative search-interest data for tracked terms, using Wikipedia Pageviews as the free/public proxy for search interest.

## Capability

Advertises `signals.search` in workgroup `signals-channel` (inter-org; shared with `enterprise-client`).

Advertisement metadata:

```
name:        search-trends
capability:  signals.search
description: Relative search-interest signals (via Wikipedia pageview volume)
message_types: [signals.search.request, signals.search.response]
```

## Workgroups

- `signals-channel` (inter-org)
- `signals-internal` (intra-org; unused in MVP)

## Envelope shapes

Request (`signals.search.request`):

```json
{
  "terms": ["layoffs", "gulf storm", "housing market"],
  "window_days": 7
}
```

Each term resolves to a Wikipedia article title. Exact article mapping is configured agent-side (not client-side) so the client does not need to know the underlying data source's structure.

Response (`signals.search.response`):

```json
{
  "as_of": "2026-04-22T08:00:00Z",
  "window_days": 7,
  "terms": {
    "layoffs":       {"change_pct": +34, "baseline_avg": 1820, "current_avg": 2438, "series": [...]},
    "gulf storm":    {"change_pct": +212,"baseline_avg":   41, "current_avg":  128, "series": [...]},
    "housing market":{"change_pct":  -7, "baseline_avg": 4201, "current_avg": 3907, "series": [...]}
  }
}
```

`change_pct` is the percent change from the preceding `window_days` baseline to the current window. `series` supports downstream correlation.

## Contract expectations

- `max_duration = 60s`
- `max_envelope_count = 4`
- `allowed_message_types = [signals.search.request, signals.search.response]`

## Run modes

- **Snapshot** (default): reads `examples/macro-pulse/snapshots/search/<term-slug>.json`.
- **Live** (`--live`): Wikipedia Pageviews API (`wikimedia.org/api/rest_v1/metrics/pageviews/...`). Falls back to snapshot on failure.

## Implementation status

- Slice-gated: advertisement, session, envelope slices
- Pre-slice: `main.go` skeleton only.
