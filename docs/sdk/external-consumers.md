# Agora SDK — External Consumer Spec

Status: implemented in `sdk/agent/catalog`.
Driving consumer: the LLM Gateway and MCP Gateway agora-mode work
(see [`docs/dashboard/design.md`](../dashboard/design.md) "Gateway Integration").

## Purpose

This document specifies the public SDK surface that agora needs to add
so that *external Go modules* — projects living outside the
`github.com/openziti/agora` module path — can build agents that
participate in an agora network. The first concrete consumers are the
LLM Gateway and MCP Gateway, both of which need to publish a single
advertisement on startup when run in agora mode.

The current SDK at `sdk/agent/` is sufficient for agents that live
inside this repository (the Macro Pulse examples, the daemon). It is
**not** sufficient for external consumers because the type surface
required to construct an advertisement leaks `internal/api` types that
Go's internal-package visibility rule forbids external repos from
importing.

This spec describes the minimum public surface that closes that gap for
the dashboard demo and frames the broader path forward.

## Background

### The internal-package barrier

`sdk/agent/app.go` exposes `Agent.Controller()` returning
`*api.Client`, where `api` is `github.com/openziti/agora/internal/api`.
Macro Pulse's agents work because they live under
`github.com/openziti/agora/...` and are allowed to import that internal
package; the `examples/macro-pulse/internal/agentutil/publish.go`
helper is itself internal-rooted.

An external repository — for example
`github.com/openziti/llm-gateway` — cannot import
`github.com/openziti/agora/internal/api`. Go's compiler enforces this.
That means an external consumer:

- can call `agent.Agent.Controller()` and hold the returned value
  through type inference, but
- cannot construct any of the request types
  (`PublishAdvertisementRequest`, `AdvertisementCapability`,
  `AdvertisementInteractionPattern`, `OptString`, etc.) needed to call
  the client's methods, because those types live in `internal/api`,
  and
- cannot interpret the returned `Advertisement` value for the same
  reason.

Result: external consumers can construct an `*Agent` but cannot publish
an advertisement through it. The SDK's primary Layer 2 entry point is
unreachable from outside this repo.

### The module-split context

[`docs/sdk/overview.md`](./overview.md) already calls this out under
"When the SDK does not yet suffice":

> **Public-module consumption.** The SDK lives inside the main
> `github.com/openziti/agora` Go module. Agents in this repo (including
> the Macro Pulse examples) can import
> `github.com/openziti/agora/sdk/agent` directly. External projects
> that want to depend on the SDK without vendoring the full Agora
> source tree will need a future split of the SDK into its own module;
> not in scope for MVP.

Splitting the SDK into a separate module remains the long-term answer.
This spec does **not** require that split. It assumes external
consumers depend on `github.com/openziti/agora` as a normal Go module
dependency, accept the transitive import surface, and use the new
public SDK helpers without ever naming `internal/api`. The split can
land later without breaking the surface defined here.

## Scope

In scope:

- a focused public package, proposed at `sdk/agent/catalog/`, that
  provides the publish/retract/lookup operations external consumers
  need to put a card in the agora catalog
- public Go types that mirror the OpenAPI advertisement shapes without
  exposing `internal/api`
- the idempotency contract for `EnsurePublished` (matches the existing
  `agentutil.EnsureAdvertisement` behavior)
- the error-shape contract that lets callers distinguish transient
  controller failures from permanent client errors

Explicitly out of scope:

- session lifecycle (`Propose`, `Accept`, `Close`, `RegisterHandler`).
  Sessions today live at `sdk/agent/session/` and use `internal/api`
  types in their public signatures (`session.go` exposes
  `*api.Session`, `*api.Envelope`, etc.). The same internal-package
  barrier applies, and the same kind of public-types fix is needed
  when the relevant Layer 2 slice promotes them. The gateway
  agora-mode work explicitly does not exercise sessions (per the
  dashboard design's "shallow integration" decision), so this spec
  does not block on it.
- contract management (`CreateContract`, `ListContracts`,
  `GetContract`). The dashboard demo's bootstrap creates the
  `gateway-services-default` contract; gateways receive its ID via the
  integration data file and reference it by ID. No external consumer
  needs to manage contracts in MVP.
- envelope-level publishing or consumption — same as sessions,
  deferred with the relevant Layer 2 slice.
- workgroup name → ID resolution. The bootstrap supplies the
  `gateway-services` workgroup ID directly. A `LookupByName` helper is
  noted as a useful follow-on but not required for this slice.
- the SDK module split itself.

## Package: `sdk/agent/catalog`

### Why `catalog`

"Catalog" is agora's Layer 2 vocabulary for the discovery surface — the
place advertisements live (see
[`docs/layer-2/catalog.md`](../layer-2/catalog.md) and
[`docs/layer-2/advertisements.md`](../layer-2/advertisements.md)). The
package surfaces the calling agent's interactions with that catalog:
publishing into it, retracting from it, looking up entries the agent
owns. Sibling packages already follow this pattern:

- `sdk/agent/session/` — session-lifecycle helpers
- `sdk/agent/catalog/` — advertisement-lifecycle helpers (this spec)

### Package layout

```
sdk/agent/catalog/
  doc.go            — package-level godoc
  publish.go        — EnsurePublished, Retract, Lookup, List
  types.go          — public Advertisement, Capability,
                      InteractionPattern, TunnelMode, PublishSpec
  errors.go         — ErrNotFound, ErrConflict, ErrUnauthorized,
                      ErrForbidden, ErrBadRequest, ErrTransient
  publish_test.go   — unit tests against a fake controller
  example_test.go   — godoc Example for EnsurePublished
```

### Public types

All types live in `package catalog`. They are Go-idiomatic
translations of the OpenAPI shapes — no `OptString`, no `NewOptX`;
zero-valued fields are treated as unset.

```go
package catalog

import "time"

// Capability describes one capability the advertised endpoint
// exposes.
type Capability struct {
    // Name is the capability identifier (e.g., "llm-routing",
    // "anthropic", "markets.equity"). Required, non-empty.
    Name string

    // Description is an optional human-readable summary.
    Description string

    // Metadata is an optional flat map of string-valued attributes.
    // The schema permits only string values today; richer types
    // would require a schema change.
    Metadata map[string]string
}

// InteractionPatternKind identifies how consumers interact with the
// advertised endpoint. The zero value is invalid; use the constants
// below.
type InteractionPatternKind string

const (
    InteractionRequestResponse InteractionPatternKind = "request-response"
    InteractionStream          InteractionPatternKind = "stream"
    InteractionBroadcast       InteractionPatternKind = "broadcast"
    InteractionCustom          InteractionPatternKind = "custom"
)

// InteractionPattern describes how a consumer interacts with the
// advertised endpoint.
type InteractionPattern struct {
    Kind          InteractionPatternKind
    CustomPattern string
}

// TunnelMode is the transport mode the advertised endpoint speaks.
// Empty string means "use the controller's default" (currently tcp).
type TunnelMode string

const (
    TunnelTCP  TunnelMode = "tcp"
    TunnelHTTP TunnelMode = "http"
    TunnelUDP  TunnelMode = "udp"
)

// AdvertisementStatus is the lifecycle state of a published
// advertisement.
type AdvertisementStatus string

const (
    StatusActive    AdvertisementStatus = "active"
    StatusRetracted AdvertisementStatus = "retracted"
)

// PublishSpec describes the advertisement an agent wants to put into
// the catalog. It is the public analog of the OpenAPI
// publishAdvertisementRequest schema.
type PublishSpec struct {
    // Name is the advertisement name. Required, unique within the
    // owning account.
    Name string

    // Description is an optional one-line summary.
    Description string

    // Capabilities is the list of capabilities this endpoint
    // exposes. The OpenAPI schema requires at least one item.
    Capabilities []Capability

    // InteractionPatterns describes how consumers may interact.
    // The OpenAPI schema requires at least one item; if the caller
    // leaves this empty, EnsurePublished defaults to
    // [InteractionRequestResponse] before sending.
    InteractionPatterns []InteractionPattern

    // WorkgroupScopeIDs is the set of workgroup IDs the
    // advertisement is visible within. Required, at least one ID,
    // each matching `^wg_[a-z0-9]{12}$`.
    WorkgroupScopeIDs []string

    // TunnelMode is optional. Empty string falls through to the
    // controller's default.
    TunnelMode TunnelMode

    // ContractID is optional. When set, must match
    // `^con_[a-z0-9]{12}$`.
    ContractID string
}

// Advertisement is the public view of an advertisement record. It is
// the public analog of the OpenAPI advertisement schema.
type Advertisement struct {
    ID                  string
    OrganizationID      string
    OrganizationName    string
    AccountID           string
    Name                string
    Description         string
    Capabilities        []Capability
    InteractionPatterns []InteractionPattern
    WorkgroupScopeIDs   []string
    TunnelMode          TunnelMode
    ContractID          string
    SchemaVersion       int
    Status              AdvertisementStatus
    RetractedAt         time.Time // zero if not retracted
    CreatedAt           time.Time
    UpdatedAt           time.Time
}
```

### Public functions

All functions take `context.Context` plus an authenticated `*agent.Agent`
plus their per-operation arguments. They return public types only;
nothing from `internal/api` appears in any signature.

```go
package catalog

import (
    "context"

    "github.com/openziti/agora/sdk/agent"
)

// EnsurePublished publishes the advertisement described by spec, or —
// when an advertisement with the same Name is already published by the
// calling account — looks the existing record up and returns it
// unchanged. Idempotent against repeated agent startup.
//
// On a name collision EnsurePublished does NOT reconcile differences
// between spec and the existing record. Callers that need to update
// fields should call Retract followed by EnsurePublished, or use a
// future Update helper (not in this spec).
//
// Validation that the controller would otherwise return as 400 is
// performed client-side first when cheap (empty Name, empty
// WorkgroupScopeIDs) so callers get a clear error without a round
// trip.
func EnsurePublished(ctx context.Context, a *agent.Agent, spec PublishSpec) (*Advertisement, error)

// Retract removes a previously-published advertisement owned by the
// calling account. Returns nil for both success and "already gone"
// (HTTP 204 and HTTP 404 are both treated as terminal success), so
// shutdown paths can call Retract idempotently without error
// handling for the second invocation.
func Retract(ctx context.Context, a *agent.Agent, advertisementID string) error

// Lookup returns the advertisement with the given name owned by the
// calling account, or nil with a wrapped ErrNotFound if no such
// advertisement is visible.
func Lookup(ctx context.Context, a *agent.Agent, name string) (*Advertisement, error)

// List returns all advertisements visible to the calling account in
// the given status. Pass StatusActive to filter to active records, or
// the empty string to return all statuses.
func List(ctx context.Context, a *agent.Agent, status AdvertisementStatus) ([]Advertisement, error)
```

### Idempotency contract

`EnsurePublished` is the load-bearing helper for agent startup. Its
behavior must match the existing
`examples/macro-pulse/internal/agentutil/EnsureAdvertisement` so
internal Macro Pulse agents and external gateway agents observe the
same semantics:

- **First run.** Controller returns 201 with the new advertisement;
  helper returns the public `Advertisement` translation.
- **Repeat run with same Name.** Controller returns 409 conflict;
  helper performs a `listAdvertisements` lookup scoped to the calling
  account, finds the existing record by name (case-insensitive), and
  returns it. No fields on the existing record are modified.
- **Repeat run with same Name but different spec fields.** Same as
  above — existing record is returned unchanged. Detecting and
  reconciling drift is explicitly not in scope here. Document this in
  the godoc.
- **Conflict but no record on re-list.** Returned as a wrapped error
  (`%q conflicted but not found on re-list`), surfaced as an
  `ErrConflict`. This matches the existing helper.

`Retract` is idempotent in the other direction: 204 and 404 both return
nil. This lets agent shutdown paths call `Retract(ctx, adID)` without
bookkeeping for whether the record already went away (e.g., from a
parallel CLI operation).

### Error semantics

The package exposes sentinel errors. Underlying controller errors are
wrapped with `fmt.Errorf("...: %w", sentinel)` so callers can use
`errors.Is` cleanly:

```go
var (
    ErrNotFound     = errors.New("catalog: not found")
    ErrConflict     = errors.New("catalog: conflict")
    ErrUnauthorized = errors.New("catalog: unauthorized")
    ErrForbidden    = errors.New("catalog: forbidden")
    ErrBadRequest   = errors.New("catalog: bad request")
    ErrTransient    = errors.New("catalog: transient controller error")
)
```

Mapping from controller responses:

| Controller response                            | Wrapped error     |
| ---------------------------------------------- | ----------------- |
| `*api.PublishAdvertisementBadRequest`          | `ErrBadRequest`   |
| `*api.PublishAdvertisementUnauthorized`        | `ErrUnauthorized` |
| `*api.PublishAdvertisementForbidden`           | `ErrForbidden`    |
| `*api.PublishAdvertisementConflict`            | `ErrConflict` (only surfaced on re-list failure; see idempotency contract above) |
| `*api.PublishAdvertisementInternalServerError` | `ErrTransient`    |
| network/timeout errors                         | `ErrTransient`    |
| `*api.RetractAdvertisementNotFound`            | nil (success)     |
| `*api.RetractAdvertisementUnauthorized`        | `ErrUnauthorized` |
| `*api.RetractAdvertisementForbidden`           | `ErrForbidden`    |
| `*api.RetractAdvertisementInternalServerError` | `ErrTransient`    |

The error message preserves the controller's `Message` field for
operator log readability:

```go
return fmt.Errorf("publish advertisement: %s: %w", typed.Message, ErrBadRequest)
```

External consumers never see the underlying `*api.Publish...` types;
they only see the wrapped sentinel and the formatted string.

### Construction notes

The implementation lives entirely inside this repo and is free to
import `internal/api`. Sketch:

```go
// publish.go
func EnsurePublished(ctx context.Context, a *agent.Agent, spec PublishSpec) (*Advertisement, error) {
    if err := validateSpec(spec); err != nil {
        return nil, err
    }
    req := toAPIPublishRequest(spec) // public -> internal
    res, err := a.Controller().PublishAdvertisement(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("publish advertisement: %w", ErrTransient)
    }
    switch typed := res.(type) {
    case *api.Advertisement:
        return fromAPIAdvertisement(typed), nil // internal -> public
    case *api.PublishAdvertisementConflict:
        return lookupOwnedByName(ctx, a, spec.Name)
    case *api.PublishAdvertisementBadRequest:
        return nil, fmt.Errorf("publish advertisement: %s: %w", typed.Message, ErrBadRequest)
    // ... remaining cases per the table above
    }
}
```

`toAPIPublishRequest` and `fromAPIAdvertisement` are unexported
translation helpers. They are the only place public types touch
internal types. External consumers call only `EnsurePublished` and
friends; the boundary is enforced by the package layout.

## Design considerations

### Why string-typed enums instead of typed constants

The OpenAPI generator emits string-backed types
(`AdvertisementInteractionPatternKind` etc.) and the controller
validates them as enums. The public package mirrors that — defines a
`type InteractionPatternKind string` plus exported constants — rather
than introducing a richer enum mechanism. The public `InteractionPattern`
struct keeps the OpenAPI shape's optional `customPattern` field without
exposing generated optional wrappers.

### Why separate public types from `internal/api` types

Three reasons.

First, the internal-package barrier requires it: external consumers
cannot name `internal/api` types in their own code, so the public
package cannot return them.

Second, the OpenAPI generator's idioms (`OptString`,
`NewOptString(...)`, the `IsSet`/`Get` accessors) are awkward for
end-user code. Public types use Go-idiomatic optionality — empty
string and zero `time.Time` mean "unset" — which is what callers
expect.

Third, decoupling lets the OpenAPI spec evolve (regenerate
`internal/api`) without breaking the public surface. The mapping
helpers in `publish.go` absorb the churn.

### Why this lives at `sdk/agent/catalog/` and not at `sdk/catalog/`

The publish operation is per-agent — it requires the calling
environment's account token, which only an `*agent.Agent` has. Putting
the package under `sdk/agent/` keeps that dependency direction
explicit and matches the existing precedent of `sdk/agent/session/`.

A future `sdk/catalog/` (peer of `sdk/agent/`) might exist for
unauthenticated catalog operations — public discovery, browsing — but
those are not in scope here and probably do not exist in agora's
intended security model anyway.

### Why no Update helper in this spec

The dashboard demo's gateway agora-mode does not exercise update — the
gateway publishes once on startup and retracts on shutdown. The
controller does support `PATCH /advertisements/{id}` (see
[`internal/api/specs/advertisements/paths.yml`](../../internal/api/specs/advertisements/paths.yml)),
so adding `Update(ctx, a, id, spec)` is a natural extension when a
consumer needs it. Out of scope here to keep the surface small.

### Why no workgroup name → ID resolution

The dashboard demo bootstrap supplies workgroup IDs to gateways
directly via the integration data file at
`$AGORA_DEMO_ROOT/gateways/<name>.yaml`. Resolving names is
unnecessary on the hot path. A future
`catalog.LookupWorkgroupByName(ctx, a, name) (workgroupID string, err error)`
is straightforward to add when a consumer that needs it appears.

## Module structure

This spec assumes external consumers depend on
`github.com/openziti/agora` as a normal Go module:

```go
// in the external consumer's go.mod
require github.com/openziti/agora vX.Y.Z
```

```go
// in the external consumer's code
import (
    "github.com/openziti/agora/sdk/agent"
    "github.com/openziti/agora/sdk/agent/catalog"
)
```

This pulls all of agora's transitive dependencies into the consumer.
That is acceptable for the dashboard-demo class of consumer (the
gateways already depend on the OpenZiti SDK and zrok); it is not
necessarily acceptable for a slim client that only wants the catalog
surface.

The follow-on SDK module split (deferred per
[`docs/sdk/overview.md`](./overview.md)) is what fully closes that
concern. It does not need to land before this spec ships. When the
split happens:

- the public `sdk/agent/...` packages move to a separate module,
- the public types defined here move with them,
- the `internal/api` translation helpers stay on the controller side
  of the split (or move to a controller-internal module), and
- external consumers' import paths do not change because the package
  paths are preserved.

The split is therefore non-breaking from the external-consumer
perspective. Implementing the split before defining the public surface
would just defer the surface decision; defining the surface now is
strictly compatible with either order.

## What's not in scope

These are explicitly deferred to follow-on slices. Each will need
similar public-types treatment when its driving consumer appears.

- **Sessions.** `sdk/agent/session/` today exposes `*api.Session`,
  `*api.Envelope`, and `*api.Contract` in its public functions
  (`Propose`, `RegisterHandler`). The same internal-package barrier
  applies. The dashboard demo does not exercise this from external
  consumers (gateways do not run session handlers), so the gap is
  visible but not blocking. The fix mirrors this spec: introduce
  public types in the package, translate at the boundary.
- **Contracts.** Same story. The dashboard bootstrap creates the
  `gateway-services-default` contract via internal code paths;
  gateways reference its ID without managing contracts themselves.
- **Envelopes.** Layer 2 envelope publish/consume is not yet on the
  external-consumer path.
- **Catalog browsing.** `List` here returns the calling account's
  *own* advertisements only (sufficient for "did I publish?" checks
  and shutdown reconciliation). Browsing the broader catalog —
  advertisements visible to the caller across orgs — is a separate
  read-only surface that fits naturally as a future
  `catalog.ListVisible(ctx, a, filter) ([]Advertisement, error)`.

A separate doc should be written for each of the above when its
driving consumer is ready, following the structural template of this
one.

## Acceptance criteria

The implementation closes the gap when all of the following are true:

1. A new package exists at `sdk/agent/catalog/` exporting the types
   and functions defined in this spec.
2. No public function in the `catalog` package names any type from
   `github.com/openziti/agora/internal/...` in its signature.
3. A unit test verifies that an external-style consumer can build
   against `catalog.EnsurePublished` without importing
   `internal/api`. (A `go vet` or compile check from a small test
   package whose imports are restricted to public agora paths is
   sufficient.)
4. The existing
   `examples/macro-pulse/internal/agentutil/EnsureAdvertisement`
   helper is rewritten to delegate to `catalog.EnsurePublished`, so
   the two surfaces share one implementation. The macro-pulse worker
   binaries continue to pass `go test ./...` and the demo
   walkthrough.
5. Idempotency tests cover: first publish, repeat publish with same
   spec, repeat publish with diverged spec, retract on missing
   record, retract on existing record.
6. Error tests cover the full mapping table in §Error semantics, with
   `errors.Is` matching the documented sentinels.
7. Godoc on every exported symbol, with at least one runnable example
   on `EnsurePublished` (Example function in `example_test.go`).
8. [`docs/sdk/overview.md`](./overview.md)'s "When the SDK does not yet
   suffice" section is updated to retire the advertisement clause and
   point readers at the new package.

## Related documentation

- [`docs/sdk/overview.md`](./overview.md) — SDK shape and the existing
  "When the SDK does not yet suffice" notes that this spec begins to
  retire
- [`docs/dashboard/design.md`](../dashboard/design.md) "Gateway
  Integration" — the driving consumer's requirements
- [`docs/dashboard/work-order.md`](../dashboard/work-order.md) Track E
  note — the work-order side of the gateway integration
- [`docs/layer-2/advertisements.md`](../layer-2/advertisements.md) —
  Layer 2 advertisement design
- [`docs/layer-2/catalog.md`](../layer-2/catalog.md) — Layer 2 catalog
  design
- [`internal/api/specs/advertisements/`](../../internal/api/specs/advertisements/) —
  the OpenAPI source of truth for the shapes this spec mirrors
- [`examples/macro-pulse/internal/agentutil/publish.go`](../../examples/macro-pulse/internal/agentutil/publish.go) —
  the existing internal helper whose behavior this spec preserves and
  exposes
