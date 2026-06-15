# CHANGELOG

## Unreleased

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