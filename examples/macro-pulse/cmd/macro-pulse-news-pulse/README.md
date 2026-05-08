# news-pulse

Agent in `signals-co`. Publishes news volume and sentiment ("tone") data per topic, backed by GDELT's free public event database.

## Capability

Advertises `signals.news` in workgroup `signals-channel`.

Advertisement metadata:

```
name:        news-pulse
capability:  signals.news
description: News volume and sentiment by topic (GDELT-backed)
message_types: [signals.news.request, signals.news.response]
```

## Workgroups

- `signals-channel` (inter-org)
- `signals-internal` (intra-org; unused in MVP)

## Envelope shapes

Request (`signals.news.request`):

```json
{
  "topics": ["financial", "energy", "supply chain"],
  "window_days": 7
}
```

Response (`signals.news.response`):

```json
{
  "as_of": "2026-04-22T08:00:00Z",
  "window_days": 7,
  "topics": {
    "financial":    {"volume": 4521, "tone": -0.18, "series": [...]},
    "energy":       {"volume": 2104, "tone": +0.04, "series": [...]},
    "supply chain": {"volume": 1287, "tone": -0.33, "series": [...]}
  }
}
```

`tone` is GDELT's average tone metric for the topic in the window, ranging roughly -10 (very negative) to +10 (very positive); values near zero indicate neutral/mixed coverage. `volume` is the article count. `series` is the daily breakdown.

## Contract expectations

- `max_duration = 60s`
- `max_envelope_count = 4`
- `allowed_message_types = [signals.news.request, signals.news.response]`

## Run modes

- **Snapshot** (default): reads `examples/macro-pulse/snapshots/news/<topic-slug>.json`.
- **Live** (`--live`): GDELT 2.0 DOC API (`api.gdeltproject.org/api/v2/doc/doc`). Falls back to snapshot on failure.

## Implementation status

- Shipped. Publishes its advertisement on startup, accepts governed sessions, and serves typed request/response envelopes from snapshot data or GDELT live data.
