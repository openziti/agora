# weather-feed

Agent in `weather-co`. Publishes current and forecast weather for configurable economic-hub cities.

## Capabilities

Advertises two capabilities on one advertisement in workgroup `weather-channel` (inter-org; shared with `enterprise-client`):

- `weather.current` — current conditions
- `weather.forecast` — N-hour forecast

Advertisement metadata:

```
name:        weather-feed
capability:  weather.current, weather.forecast
description: Current conditions and short-horizon forecasts for economic hubs
message_types:
  - weather.current.request, weather.current.response
  - weather.forecast.request, weather.forecast.response
```

## Workgroups

- `weather-channel` (inter-org)
- `weather-internal` (intra-org; unused in MVP)

## Envelope shapes

Current-conditions request (`weather.current.request`):

```json
{"cities": ["New York", "Houston", "Frankfurt", "Singapore"]}
```

Current-conditions response (`weather.current.response`):

```json
{
  "as_of": "2026-04-22T08:00:00Z",
  "cities": {
    "New York":  {"temp_f": 52, "condition": "clear",      "anomaly_f": -8},
    "Houston":   {"temp_f": 68, "condition": "thunderstorm","anomaly_f": +3},
    "Frankfurt": {"temp_f": 71, "condition": "clear",      "anomaly_f": +9},
    "Singapore": {"temp_f": 82, "condition": "rain",       "anomaly_f": +1}
  }
}
```

`anomaly_f` is the delta from 30-year seasonal norm (negative = colder than normal).

Forecast request (`weather.forecast.request`):

```json
{"cities": ["New York", "Houston"], "horizon_hours": 72}
```

Forecast response (`weather.forecast.response`):

```json
{
  "as_of": "2026-04-22T08:00:00Z",
  "horizon_hours": 72,
  "cities": {
    "New York": {"daily": [
      {"date": "2026-04-22", "temp_high_f": 54, "temp_low_f": 41, "condition": "clear"},
      {"date": "2026-04-23", "temp_high_f": 58, "temp_low_f": 45, "condition": "cloudy"},
      {"date": "2026-04-24", "temp_high_f": 62, "temp_low_f": 48, "condition": "clear"}
    ]},
    ...
  }
}
```

## Contract expectations

- `max_duration = 60s`
- `max_envelope_count = 8` (allows for current + forecast in the same session)
- `allowed_message_types = [weather.current.*, weather.forecast.*]`

## Run modes

- **Snapshot** (default): reads `examples/macro-pulse/snapshots/weather/<city>.json`. Snapshots cover the same cities `pulse-agent` queries by default.
- **Live** (`--live`): Open-Meteo (free, no key). Falls back to snapshot on any failure.

## Implementation status

- Shipped. Publishes its advertisement on startup, accepts governed sessions, and serves typed request/response envelopes from snapshot data or Open-Meteo live data.
