# Agora Documentation

Documentation is split into two buckets that describe different things and are maintained on different rhythms. The split exists so a reader can trust each bucket for what it claims to be — `current/` as a description of running code, `future/` as a record of intent that has not yet landed.

## `docs/current/` — what exists today

Describes Agora as it exists in the running code. A reader (human or agent) should be able to verify any statement here against the implementation. Updated when the system changes.

- Specs are normative: `current/layer-1/spec.md`, `current/layer-2/spec.md`, and the per-concept Layer 2 docs describe the behavior the controller and SDK actually implement.
- Status docs are factual current-state: `current/layer-1/status.md` and `current/layer-2/status.md` track what is shipped and what remains.
- Architecture, SDK, examples, and maintainer-facing context also live here.

If a statement in `current/` is no longer true of the code, the statement is wrong — fix the doc, or fix the code.

## `docs/future/` — intent that has not landed

Vision, specs in progress, work orders, and explicitly deferred work. The reader should not expect any of it to be verifiable against the code today.

- `future/roadmap/post-mvp.md` — work deliberately deferred past the current milestone.
- `future/dashboard/` — design, work order, and reference material for dashboard work in progress.
- `future/sdk/` — proposed external-consumer SDK surfaces not yet implemented.

A doc in `future/` is allowed to disagree with the running system — that is the point.

## Where new docs go

- Describing built behavior → `current/<area>/`.
- Vision, design-in-progress, work orders, deferred work → `future/<area>/`.
- Do not author a doc in `current/` that describes work not yet done; it makes `current/` untrustworthy.
- Do not strand a vision doc in `current/` because the work is progressing — keep it in `future/` until the implementation matches it, then promote.

## Promotion: `future/` → `current/`

A `future/` doc moves to `current/` when the work it describes ships. Promotion is a rewrite, not just a `git mv`:

1. Move the file to the matching location under `current/`.
2. Strip vision and intent language; restate the doc as a description of the built system.
3. Fix cross-references inside and outside `docs/` so they point at the new path.
4. If the original `future/` artifact was a work order or scratch design with no surviving normative content, delete it instead of promoting.

The reverse direction (`current/` → `future/`) is rare and only used when a description in `current/` is being intentionally regressed and replaced with vision-mode language.

## Layout inside each bucket

Subdirectories group by area: `architecture/`, `layer-1/`, `layer-2/`, `sdk/`, `dashboard/`, `examples/`, `maintainers/`, `roadmap/`. Any of these may appear in either bucket — the same area can have docs on both sides of the split when some of its work has shipped and some has not.

## Start here

- [`current/architecture/overview.md`](./current/architecture/overview.md) — cross-layer architecture
- [`current/maintainers/current-state.md`](./current/maintainers/current-state.md) — repo-shape, workflows, conventions
- [`current/layer-1/`](./current/layer-1/) and [`current/layer-2/`](./current/layer-2/) — per-layer spec, status, and per-concept docs
- [`current/sdk/overview.md`](./current/sdk/overview.md) — supported SDK integration path
- [`current/examples/index.md`](./current/examples/index.md) — reference/demo agents
- [`future/roadmap/post-mvp.md`](./future/roadmap/post-mvp.md) — intentionally deferred work

The root [README.md](../README.md) is the quick-start entry point; [AGENTS.md](../AGENTS.md) is the contributor guidance file.
