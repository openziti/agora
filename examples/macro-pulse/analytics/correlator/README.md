# correlator

Agent in `analytics-co`. Computes Pearson correlation between two time series supplied by the caller. A pure-function analytics tool.

## Capability

Advertises `analytics.correlate` in workgroup `analytics-channel` (inter-org; shared with `enterprise-client`).

Advertisement metadata:

```
name:        correlator
capability:  analytics.correlate
description: Pearson correlation coefficient between two aligned time series
message_types: [analytics.correlate.request, analytics.correlate.response]
```

## Workgroups

- `analytics-channel` (inter-org)

No intra-org workgroup for `analytics-co` in MVP.

## Envelope shapes

Request (`analytics.correlate.request`):

```json
{
  "series_a": {
    "label": "a",
    "points": [{"t": "2026-03-23", "v": 5291.4}, ...]
  },
  "series_b": {
    "label": "b",
    "points": [{"t": "2026-03-23", "v": 78.42}, ...]
  }
}
```

Both series must cover the same dates. The agent aligns by `t` and drops any non-overlapping dates before computing correlation; if fewer than 3 aligned points remain, the agent responds with an error envelope (same `message_type`, with an `error` field populated).

Response (`analytics.correlate.response`):

```json
{
  "pearson_r": +0.52,
  "n": 30,
  "label_a": "a",
  "label_b": "b"
}
```

The agent deliberately does not receive the underlying tickers, cities, or terms — only the `label` the caller assigns. This is part of the governance narrative: `correlator` sees two abstract series, nothing about what they represent.

## Contract expectations

- `max_duration = 10s`
- `max_envelope_count = 2` (one request, one response)
- `allowed_message_types = [analytics.correlate.request, analytics.correlate.response]`

## Run modes

Not data-dependent. One mode: computes correlation from request payload. No snapshot, no `--live`.

## Implementation status

- Slice-gated: advertisement, session, envelope slices
- Pre-slice: `main.go` skeleton only.
