# CHANGELOG

## Unreleased

- FIX: Setup wizard entry points (user-menu button, zero-environment login redirect) are now gated behind the same flag as the /setup route, so nothing links to the route while the wizard is disabled.

- CHANGE: Redesigned the dashboard with a data-driven network topology, a live activity feed sourced from audit events, a persistent activity rail, and honest degraded/error states.

- CHANGE: Added a tabbed detail drawer surfacing per-session trace lifecycle (proposal through close) and contract-violation detail, including the violation reason inline.

- CHANGE: Added a swimlane activity strip to the Audit screen, visualizing event clusters (sessions, violations, envelopes, tunnels, advertisements, account/environment) on a shared, filter-aware time axis.

- CHANGE: Condensed table density and aligned the dashboard's visual styling with the Customer Connect design system.

- CHANGE: Added dark mode across the UI, with IBM Plex typography and a full design-token system.

- CHANGE: Added a five-step setup wizard at `/setup` (gated off pending backend support).

## v0.1.5

- FEATURE: Standalone tunnels are now owned by the account rather than by the environment that created them. A tunnel survives retirement of any environment, and any of the owning account's environments may host (serve) it. The new `agora tunnel takeover <name>` command, and `agora tunnel serve --takeover`, reclaim a tunnel by evicting whatever is currently hosting it (deleting its fabric terminators and, for a proxy tunnel, clearing the serve record). Layer 2 session tunnels remain environment-owned.

- FEATURE: New `agora tunnel create` command provisions a durable tunnel resource (direct or proxy, via `--backend`) without serving it, and `agora tunnel serve <name>` now serves an already-created proxy tunnel with `--mode`/`--backend` optional.

## v0.1.4

- FEATURE: New unauthenticated `GET /v1/version` API endpoint for interrogating the controller's build version.

- CHANGE: Adopted `github.com/michaelquigley/push` for the `agora version` command and build-metadata stamping. `agora version` now reports full build detail (version, commit, build date, branch, builder), and CI builds are version-stamped.

- FIX: Updated dashboard ui dependencies, including a major upgrade of the `vite` build tooling from 7.3.2 to 8.0.16.

## v0.1.3

- FEATURE: New SDK primitives supporting low-level `Dial` and `Listen` access to layer 1 tunnels.

## v0.1.2

- CHANGE: Do not push docker images.

## v0.1.1

- CHANGE: `agora enable` now supports token-less enablement; if you do not provide a token it will prompt for email/password.

- FIX: Corrected issue where multiple environments in the same account were unable to share layer 1 resources (`agora tunnel connect`, `agora tunnel serve`).

## v0.1.0

Initial release.