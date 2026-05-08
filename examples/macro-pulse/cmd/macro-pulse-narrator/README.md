# narrator

Agent in `analytics-co`. Template-driven natural-language summary of structured numeric inputs. Deterministic; no LLM.

## Capability

Advertises `analytics.narrate` in workgroup `analytics-channel`.

Advertisement metadata:

```
name:        narrator
capability:  analytics.narrate
description: Template-based prose summary of numeric structured data
message_types: [analytics.narrate.request, analytics.narrate.response]
```

## Workgroups

- `analytics-channel` (inter-org)

## Envelope shapes

Request (`analytics.narrate.request`):

```json
{
  "template": "macro_pulse_brief",
  "inputs": {
    "markets":       { ... flattened market summary ... },
    "weather":       { ... flattened weather summary ... },
    "signals":       { ... flattened signals summary ... },
    "correlations":  [{"label_a": "WTI", "label_b": "gulf_storm", "pearson_r": 0.52}, ...]
  }
}
```

The `template` string selects a template definition registered inside the agent. MVP ships with one template, `macro_pulse_brief`, which produces the three-paragraph "BRIEF" section seen at the bottom of the example output in [../../README.md](../../README.md).

Response (`analytics.narrate.response`):

```json
{
  "text": "Energy complex faces near-term upside risk from Gulf weather.\nEquity softness aligns with rising employment-anxiety signals.\nCurrency block subdued; gold bid suggests defensive positioning."
}
```

## Template definition

Templates are Go `text/template` strings with simple helpers for formatting numbers, thresholds, and conditional phrases. A template produces a fixed paragraph set; inputs drive which clauses fire. The result is deterministic: identical inputs produce identical output.

This is how most production "AI briefings" work under the hood anyway. Leaving the LLM path as a documented post-MVP extension keeps the demo fast, reproducible, and zero-dependency.

## Contract expectations

- `max_duration = 10s`
- `max_envelope_count = 2`
- `allowed_message_types = [analytics.narrate.request, analytics.narrate.response]`

## Run modes

Not data-dependent. Single mode: template expansion against request payload.

## Implementation status

- Shipped. Publishes its advertisement on startup, accepts governed sessions, and returns deterministic prose summaries for typed request envelopes.

## Post-MVP extension

An optional `--llm` mode could replace the template with a real LLM call (via `llm-gateway` or a direct provider API), demonstrating that Agora can gate LLM access through a session with a contract. That is tracked as a future extension, not part of the first Macro Pulse delivery.
