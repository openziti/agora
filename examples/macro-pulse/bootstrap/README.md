# bootstrap

A small Go program that sets up the full Macro Pulse deployment topology against a running Agora controller: five organizations, one account per agent, enabled environments per account, and seven workgroups with all inter-org invitations pre-accepted.

`bootstrap` is a reference piece, not a demo agent. It does not stay resident; it runs to completion and exits.

## What it creates

Five organizations:

- `markets-co`
- `weather-co`
- `signals-co`
- `analytics-co`
- `enterprise-client`

One account per agent (so each agent runs under its own identity):

- `markets-co`: `equity-feed@markets-co`, `fx-feed@markets-co`, `commodities-feed@markets-co`
- `weather-co`: `weather-feed@weather-co`
- `signals-co`: `search-trends@signals-co`, `news-pulse@signals-co`
- `analytics-co`: `correlator@analytics-co`, `narrator@analytics-co`
- `enterprise-client`: `pulse-agent@enterprise-client`

Seven workgroups, configured as described in [../README.md](../README.md):

- `markets-channel` (inter-org): `markets-co` owner, invites `enterprise-client`
- `weather-channel` (inter-org): `weather-co` owner, invites `enterprise-client`
- `signals-channel` (inter-org): `signals-co` owner, invites `enterprise-client`
- `analytics-channel` (inter-org): `analytics-co` owner, invites `enterprise-client`
- `markets-internal` (intra-org): `markets-co`
- `weather-internal` (intra-org): `weather-co`
- `signals-internal` (intra-org): `signals-co`

For each inter-org workgroup, `bootstrap` also accepts the invitation from the invited org, so all four inter-org workgroups transition to `active`. Initial workgroup admin seeding happens at creation and acceptance:

- `markets-channel` owner-side admin: `equity-feed@markets-co`; client-side admin (on acceptance): `pulse-agent@enterprise-client`
- `weather-channel` owner-side admin: `weather-feed@weather-co`; client-side admin: `pulse-agent@enterprise-client`
- `signals-channel` owner-side admin: `search-trends@signals-co`; client-side admin: `pulse-agent@enterprise-client`
- `analytics-channel` owner-side admin: `correlator@analytics-co`; client-side admin: `pulse-agent@enterprise-client`

After bootstrap, all non-admin agents (e.g. `fx-feed`, `news-pulse`) can be added as members by the initial workgroup admin — either manually or by a follow-up script.

## What it needs

- `AGORA_ADMIN_TOKEN` in the environment — same convention as existing admin CLI commands
- A running controller reachable at a URL provided via `--controller` (default: `http://127.0.0.1:8080`)

## What it produces

On success, emits one line per created resource, then prints the account tokens for each of the nine agent accounts. The tokens need to be distributed to the machines (or environment roots) where each agent will run; `bootstrap` itself does not enroll environments — that is done separately via `agora enable` per agent (or per environment root when running multiple agents on one host).

A future enhancement can have `bootstrap` also perform `agora enable` for each agent against locally-rooted `~/.agora-<agent>` directories so the whole demo can run on a single machine. That is a post-MVP convenience, not required for the demo to work.

## Implementation status

- Slice-gated: **workgroup slice**
- Pre-slice: not implemented. The `main.go` for `bootstrap` exists but only logs "bootstrap requires workgroup slice" and exits.
- After workgroup slice: full implementation; runnable against a clean controller to produce the full demo topology in under a second.
