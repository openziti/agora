# Reference Examples

This directory catalogs the reference/demo agent implementations that live under [`examples/`](../../examples/) in the repository. Examples serve three purposes:

1. **Demonstrate** Agora's value proposition end-to-end, in scenarios plausible enough to stand up to executive scrutiny
2. **Exercise** each Layer 2 slice as it lands, so no slice is "done" without a working reference agent using it
3. **Inform** implementation — what a real agent needs influences how the Layer 2 surfaces are designed

## Current examples

| Example | Status | Summary |
| --- | --- | --- |
| [Macro Pulse](macro-pulse.md) | in progress | Cross-domain morning market briefing. Five orgs, eight agents. Composes market + weather + internet-activity signals. |

Macro Pulse is the primary reference example. Additional examples may be added as the need arises (for instance, a smaller "getting started" scenario, or an intentionally external-consumer demo on a separate `go.mod` that uses only public Agora surfaces). These do not yet exist.

## Where examples live

- `examples/<example-name>/` at the repo root. Subdirectories are part of the main `github.com/openziti/agora` Go module, which means example agents can import from `internal/` packages. This is a deliberate choice for reference examples — a future "external consumer" demo would sit in a sibling directory with its own `go.mod` and would only import public surfaces.
- Each example has a `README.md` at its root with the operator-facing narrative and running instructions.
- Each agent within an example has its own `README.md` describing the agent's capability, workgroup, envelope shapes, and behavior.
- Per-example formal architecture docs live here under `docs/examples/`.

## How examples roll out alongside slices

Examples grow incrementally with Layer 2 implementation. Each Layer 2 slice (workgroups → catalog + advertisements → sessions → contracts → envelopes) lands with a corresponding advancement to the current example(s). See each example's dedicated doc for its slice-by-slice dependency matrix.

Running examples end-to-end requires the slices they depend on to be implemented. Until then, each example's per-agent `README.md` documents what the agent _will_ do; `main.go` skeletons exist and compile, but do not yet exercise the gated behavior.
