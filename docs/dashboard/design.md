# Agora Dashboard — Design Specification

This document is the canonical design for the Agora dashboard, the time-series substrate that backs it, the gateway integration that populates the catalog, the continuous Macro Pulse activity loop that gives it a heartbeat, and the demo orchestration that ties everything together.

It is written to be implementation-ready. The companion document, `work-order.md`, decomposes this design into discrete work units that coding agents can execute against.

## Purpose

The Agora dashboard exists to make the value of Agora visible to a visual-first audience — the c-suite and director-level executives who respond to design mockups in pitch decks more than to architecture diagrams or CLI sessions. The LLM Gateway and MCP Gateway have been pitched to this audience using high-fidelity dashboard mockups; those mockups establish the visual language of the family of products even though the dashboards themselves are not yet built. Agora has been invisible to that audience because its primitives — workgroups, advertisements, sessions, contracts, envelopes — only surface today through CLI, REST, and the existing reference example. The Agora dashboard puts those primitives on screen, in the same visual language the gateway mockups established, and positions Agora as the governance layer that sits *above* the gateways rather than alongside them. Agora's dashboard is the first running dashboard in the family; the gateway dashboards will follow as a separate slice and will inherit the design system this work establishes.

The dashboard is not a shipped product surface. It is a demo deliverable: it does not need to be production-ready, scalable, multi-tenant, or hardened against adversarial input. It does need to be sales-executable on a localhost laptop, presentation-friendly in a group setting, and driven by real activity flowing through real Agora primitives. Synthetic numbers behind charts are acceptable only where time-series infrastructure cannot yet support real ones; everywhere else, data honesty is the rule.

## Audience and Use Cases

- **Demo by a salesperson, on stage or one-to-one.** The salesperson runs a single launch script on their laptop, the dashboard comes up at `http://localhost:<port>`, and they walk a known click-path through the screens. The login screen is the demo's first frame; the salesperson types pre-provisioned demo credentials and proceeds.
- **Recorded walkthrough video.** Same launch script, same click-path, recorded once and reused.
- **Internal exec briefing.** Engineering shows the dashboard to leadership to ground broader Agora pitch material in a working artifact.

The shared posture: the audience is judging Agora through what they see on the screen. Visual fidelity to the gateway mockups is therefore non-negotiable, not because the demo audience knows the difference, but because the family-of-products story falls apart at the first glance that breaks the pattern.

## Scope

In scope for the dashboard work:

- a single-page React application (`ui/`) served from the Agora controller, matching the visual language established by the LLM Gateway and MCP Gateway mockups
- a login screen and cookie-based session authentication for the dashboard, layered onto Agora's existing account-token authenticator
- a time-series substrate in PostgreSQL that records session and envelope lifecycle events with enough fidelity to drive the dashboard's activity views and historical charts
- aggregation REST endpoints on the controller that return the shapes the dashboard reads
- a continuous loop mode for the Macro Pulse `macro-pulse-pulse-agent` that drives a realistic stream of activity through the system
- integration with `llm-gateway` and `mcp-gateway` (which live in their own repositories): each gateway gains an agora-mode flag in its own repo and, when running in agora mode, publishes an Agora advertisement on startup so it appears in the catalog. The dashboard's only direct gateway work is to provision the demo's gateway accounts, contracts, and workgroups, and to invoke the gateway binaries from the launch script.
- a `demo-bootstrap` binary and accompanying launch script that provisions a complete demo environment from a clean slate, including a known-credentials demo account
- pre-seeded historical event data so charts that read "last 7 days" do not show a single bar

Explicitly out of scope:

- production hardening of any kind: error budgets, observability of the dashboard itself, fault tolerance, retry policies on the time-series ingest path
- multi-tenant isolation on the dashboard surface beyond what falls out of the existing organization-scoped authentication; the dashboard is a single-user view per browser, served on localhost
- registration, password reset, email verification, MFA, SSO, identity provider integration; the demo bootstrap creates the demo account directly and the existing CLI continues to be the path for any other account-management operations
- per-account or per-IP rate limiting on login attempts; session expiry and refresh
- environment-scoped aggregations: dashboard data is organization-scoped in MVP. Per-environment filtering (e.g., "envelopes today, this environment only") is deferred — it requires adding `environment_id` to several event types in the audit-events emission path and adding optional `environmentId` query parameters to the dashboard endpoints. The org indicator in the chrome is read-only in MVP.
- deep gateway integration where tool calls are routed as Agora envelopes through Agora sessions; the demo uses shallow integration (advertisement-only) and the existing gateway transports continue to operate as they do today
- mobile or responsive layouts beyond what falls out of the desktop-first design naturally
- internationalization
- the post-MVP metrics and limits work tracked in `docs/roadmap/post-mvp.md`; the time-series substrate this work introduces is a precursor that accelerates that roadmap item but does not satisfy it

## Visual Design System

The visual language is decoded from the existing LLM Gateway and MCP Gateway mockups. The mockup repository is a screenshot factory — each page hand-rolls its own styles inline — so the visual system has not previously been codified. Agora's dashboard codifies it.

### Palette

All colors are Tailwind-named shades. The mockups use Tailwind colors throughout even though they do not depend on Tailwind as a library; this design honors that choice and uses Tailwind v4 (CSS-first, configured via `@theme` blocks rather than a JavaScript config file) to declare the tokens.

**Neutrals (Zinc family)** carry the entire chrome:

| Token | Tailwind | Hex | Usage |
| --- | --- | --- | --- |
| `--color-page` | zinc-50 | `#fafafa` | page background |
| `--color-panel` | white | `#ffffff` | card and panel surfaces |
| `--color-panel-subtle` | zinc-100 | `#f4f4f5` | nested panels, inactive selectors |
| `--color-border` | zinc-200 | `#e4e4e7` | card and panel borders |
| `--color-border-strong` | zinc-300 | `#d4d4d8` | input borders, separators that need to read |
| `--color-text-mute-2` | zinc-400 | `#a1a1aa` | tertiary text |
| `--color-text-mute` | zinc-500 | `#71717a` | secondary text, labels |
| `--color-text-mute-strong` | zinc-600 | `#52525b` | descriptions, table headers |
| `--color-text` | zinc-900 | `#18181b` | primary text |

**Brand colors** identify the three products in the family. Each product has a primary shade and a gradient pair used in logo tiles and accent surfaces.

| Product | Primary | Gradient end | Usage |
| --- | --- | --- | --- |
| LLM Gateway | cyan-600 (`#0891b2`) | sky-600 (`#0284c7`) | logo tile, route highlights, accent borders |
| MCP Gateway | violet-600 (`#7c3aed`) | purple-500 (`#a855f7`) | logo tile, accent borders |
| Agora | indigo-600 (`#4f46e5`) | blue-700 (`#1d4ed8`) | logo tile, accent borders |

The "above both gateways" gesture appears in the existing mockups as a `cyan-600 → violet-600` cross-product gradient. The Agora dashboard does not claim that gradient as its own brand mark — Agora's identity is indigo — but the cross-product gradient remains available for moments where the dashboard explicitly references the gateway family (for example, the catalog rows that show the LLM Gateway and MCP Gateway advertisements published in agora mode).

**Status colors** are shared across all three dashboards:

| State | Color | Hex |
| --- | --- | --- |
| healthy / success / active | green-500 / green-600 | `#22c55e` / `#16a34a` |
| degraded / warning | amber-600 / orange-500 | `#d97706` / `#f97316` |
| blocked / failed / rejected | red-600 | `#dc2626` |
| info | blue-600 | `#2563eb` |
| highlight | yellow-500 | `#eab308` |

### Typography

- **UI font:** Inter, weights 400 / 500 / 600 / 700, bundled locally via `@fontsource-variable/inter` (imported from the SPA entry point) so the demo does not depend on Google Fonts being reachable at runtime
- **Monospace font:** JetBrains Mono, weights 400 / 500, bundled locally via `@fontsource-variable/jetbrains-mono` (same rationale), for resource IDs, code, terminal-style values
- **Type scale:**
  - stat-card numbers: 28–32px / weight 700 / `letter-spacing: -0.02em`
  - section headings: 18–20px / weight 600
  - body: 14px / weight 400
  - labels and small caps: 11–12px / weight 500 / `letter-spacing: 0.04em` / uppercase
  - table cell: 13px / weight 400

### Layout, spacing, and surfaces

- 8px and 12px are the base spacing increments; gaps between siblings, padding inside cards
- 16px to 24px between sibling sections
- 32px page-edge gutters in the chrome
- card border-radius: 8px
- pill border-radius: 6px (rectangular pills) or fully-rounded (status dots in pills)
- borders: 1px solid `--color-border`; cards never use shadow alone, always with a 1px border
- elevation: hover/focus may add a subtle shadow but the resting state is borders-only

### Component primitives

These are the components the dashboard ships on day one. Every screen composes from this set; new screens should not introduce one-off styles outside the system.

- **AppShell** — the chrome: brand mark + name, org indicator, top nav, right-side status pill and user badge. Accepts a `product` prop (`'agora' | 'llm' | 'mcp'`) that drives the brand color and logo tile.
- **BrandMark** — the gradient-tile-plus-name logo, parameterized by product. Includes a small, muted "by NetFoundry" tagline beneath the product wordmark — present in every product variant since all three are NetFoundry products. Typography for the tagline: 10–11px, weight 400, `--color-text-mute` (zinc-500), no letter-spacing change. Layout: left-aligned under the wordmark, with the gradient tile spanning the full height of wordmark-plus-tagline.
- **OrgIndicator** — the rounded pill to the right of the brand mark, displaying the calling account's organization name. In MVP this is read-only (no dropdown, no selection state) — see "Identity scoping" in the Information Architecture section. The component renders an icon, the organization name, and a chevron, matching the gateway mockups' instance-switcher visual without exposing interaction. The component shape supports a future `selectedId` plus `onChange` API (e.g., for a future organization switcher) without chrome rework. (Earlier drafts of this spec called this primitive `EnvSwitcher` and had it display an environment name; that name and source were retired when DASH-WO-022 surfaced that the cookie-auth principal carries no environment identity. The primitive is now named after what it actually shows.)
- **NavTabs** — the primary navigation row. Active tab is brand-colored, inactive tabs are zinc-500.
- **StatusPill** — pill with a colored dot and a label. Used in the chrome ("All systems operational") and in tables (e.g., environment status: `online`, `stale`, `unknown`, `disabled`; session close reasons: `consumer_close`, `provider_close`, `contract_violation`, etc.).
- **StatCard** — large number, small label, optional delta with up/down arrow, optional icon corner.
- **SectionPanel** — bordered card with a header row (title + actions) and a body. Used everywhere a screen needs to group content.
- **BarChart** — hand-rolled CSS-and-SVG bar chart with the gradient-into-fade fill the mockups use. Accepts an array of `{label, value}` and a single accent color.
- **SidebarBreakdown** — the "Cost by Provider" pattern: vertical list of label + small horizontal bar + value. Accepts a list of `{label, value, accent}`.
- **DataTable** — the table primitive used by Sessions, Workgroups, Catalog. Supports sortable columns, status-pill cells, monospace ID cells, action menus.
- **EmptyState** — the panel content shown when a list has nothing in it.
- **KeyValueGrid** — two-column grid for resource describe surfaces (used inside drill-down drawers).

Anything not in this list waits for a second screen to need it before being lifted into the kit.

### Iconography

Icons come from `lucide-react`, matching the gateway mockups exactly. The icons used in the mockups — `Activity`, `Shield`, `Server`, `Zap`, `Layers`, `ChevronDown`, `ChevronRight`, `Check`, `X`, `Plus`, `MoreHorizontal`, `ArrowUpRight`, `ArrowDownRight`, `Wifi`, `WifiOff`, `Clock`, `Users`, `Wrench`, `Search`, `Filter`, `Eye`, `Settings`, `RefreshCw`, `AlertTriangle`, `ExternalLink`, `Lock`, `Fingerprint`, `Key`, `GitBranch`, `Cpu`, `ArrowRight`, `Globe`, `Terminal`, `FileText`, `User` — are the working set; new icons are picked from the same pack.

## Information Architecture

The dashboard is a six-tab application, of which four are day-one scope and two are stretch.

```
┌────────────────────────────────────────────────────────────────────────┐
│  Agora   [environment-prod ▾]   Dashboard  Sessions  Workgroups        │
│                                 Catalog  · · ·  Audit       [● Active] │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│   <active screen>                                                      │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

The chrome:

- **Brand mark** in the upper-left (indigo gradient tile + "agora" wordmark, with a small "by NetFoundry" tagline beneath the wordmark)
- **Org indicator** to the right of the brand mark, displaying the calling account's organization name. In MVP this is a read-only display, not a switcher: the cookie-authenticated principal identifies the calling account and its organization, and the indicator surfaces that organization identity in the chrome. Visual placement matches the gateway mockups' instance-switcher slot. The component renders as a rounded pill containing an icon, the organization name, and (visually only, non-interactive in MVP) a chevron, so a future iteration that adds real organization switching does not require chrome rework. See "Identity scoping" below for why the indicator displays organization rather than environment, and why it is decorative in MVP.
- **Top nav** with Dashboard, Sessions, Workgroups, Catalog as day-one tabs; Contracts and Audit as stretch
- **Right-side controls**: an "All systems operational" StatusPill driven by controller `/ready`, plus a user badge showing the logged-in account's initials (derived from email). Clicking the user badge opens a small menu with the user's email and a Logout action.

### Identity scoping

The dashboard's data is scoped to the calling account's **organization**, not to any individual environment. Aggregations (sessions, envelope flow, workgroup activity, environment status table) read every event for the organization the caller belongs to, and screens that list workgroups, advertisements, and contracts use the calling account's visibility — not a per-environment filter. This matches the existing API surface: `GET /v1/sessions`, `GET /v1/workgroups`, `GET /v1/catalog/advertisements`, and the new `GET /v1/dashboard/*` endpoints all scope by the authenticated principal's organization.

The org indicator in the chrome surfaces that organization identity — it tells the demo audience which organization they are viewing data for. The cookie-authenticated principal carries account ID, organization ID, email, and role (see `internal/controller/service.go` `accountPrincipal`); the indicator displays the organization's *display name*, sourced from the SPA's auth store, which is populated by `GET /v1/account/whoami` on initial mount and on 401 recovery (see `work-order.md` G.13 for the endpoint). Both `whoami` and `/v1/dashboard/summary` resolve the name via the same server-side `Organizations.GetByID(principal.OrganizationID)` lookup documented in A.5 Notes' "Principal vs display name", so the chrome's org indicator and the Dashboard tab's "current account" callout always display the same name without coordinating data sources directly. There is no "calling environment" in a cookie-authenticated session — the cookie is account-scoped, not environment-scoped — so the chrome does not display an environment name. Environment-level information lives in the Dashboard tab's environment status table, which is the right surface for "show me every environment in my org."

What this means for screen behavior:

- The Dashboard tab's "current account" callout displays the calling account's email, organization name, and role. The four stat cards and the activity chart aggregate over the entire organization. The environment status table at the bottom of the Dashboard tab lists every environment in the organization, which is the natural place to surface multi-environment activity in MVP.
- The Sessions tab lists every session the calling account participates in (provider or consumer), regardless of which environment proposed or accepted them.
- The Workgroups tab lists every workgroup the calling account is a member of.
- The Catalog tab lists every advertisement visible to the calling account.

Implementing agents should treat "environment-scoped" as out of scope for MVP: no `environmentId` query parameters on dashboard endpoints, no per-environment filtering in aggregation helpers, no client-side filtering by environment. The chrome's org indicator reads its value from the SPA's auth store (populated by `/v1/account/whoami`), specifically `account.organizationName`; it does not drive any data fetches.

The org indicator is the dashboard's visual analog to the gateway mockups' instance switcher, but unlike that mockup-level affordance it is read-only in MVP. Organization is *not* a top-level switcher either: the dashboard implicitly shows the calling account's organization, and any cross-organization activity surfaces through workgroups (intra-org and inter-org workgroups appear in the same Workgroups tab, distinguished by an inline badge).

(Earlier drafts of this spec named this section "Environment scoping" and described the chrome as displaying an environment name, with the value to come from a `getDashboardSummary().environment` object. DASH-WO-022 surfaced that the cookie-authenticated principal carries no environment identity — there is no deterministic "calling environment" to display for an account-cookie session — so the chrome label and the summary response field both moved to organization/account, which the principal does carry. Per-environment filtering and a real environment switcher remain deferred to a follow-on slice that adds `environment_id` to event emissions and `environmentId` query parameters to the dashboard endpoints.)

The chrome described above wraps every authenticated screen. The **login screen** lives outside the chrome — when the SPA detects an unauthenticated state (no `agora-session` cookie, or a `401` response from any API call), it routes to a full-page `/login` view that renders only the indigo brand mark and the credential form. Authentication-aware routing is the responsibility of the top-level `App` component; individual screens assume they only render under an authenticated session.

### Day-one tabs

#### Dashboard

The landing screen. Mirrors the gateway mockups' four-stat-cards-plus-chart-plus-breakdown-plus-status-table layout.

**Above-the-fold "current account" callout** — a wide rounded panel that announces the calling account's email and its organization. Pulled directly from the gateway mockups' "CURRENT INSTANCE" panel, repurposed: where the gateway mockups identified the deployment instance, agora's dashboard identifies the user (account) and their org, since the cookie-auth principal carries that identity directly. Right side carries a four-or-five number ribbon: workgroups, advertisements, sessions today, environments-in-org, and a "Zero-Trust Active" badge. The numbers in the ribbon aggregate over the calling account's organization (per "Identity scoping" above), not over any individual environment.

**Four stat cards:**

1. *Active Sessions* — current snapshot count of sessions in the calling account's organization with `state IN ('proposed', 'accepting', 'active', 'closing')`. Delta `activeSessionsDelta7d` = current snapshot minus the count of sessions that were *historically* in this in-flight cohort at `now - 7 days` (reconstructed from `proposed_at` and `closed_at` rather than current `state`, so a session that was in-flight a week ago and has since closed correctly registers as a one-unit drop in the delta). The delta reads as "how many more in-flight sessions exist now versus a week ago." See `work-order.md` A.4's "Snapshot delta semantics" Note for why current-state filtering produces silently wrong numbers and how the historical reconstruction predicate fixes it. Negative deltas render in red/orange, positive in green.
2. *Envelopes Today* — total envelopes flowed in the calling account's organization in the rolling 24h window `[now - 24h, now)`. Delta `envelopesYesterday` = the same total for the prior rolling 24h window `[now - 48h, now - 24h)`; the dashboard renders the delta as `envelopesToday - envelopesYesterday`. Both windows are deliberately rolling (not UTC-day-aligned) so the comparison is apples-to-apples regardless of wall-clock time.
3. *Active Workgroups* — count of distinct workgroup memberships held by the calling account. No delta in MVP — "how many groups am I in" is a structural number, not a trending one.
4. *Active Tunnels* — current snapshot count of live runtime records in the calling account's organization. Counts the union of `tunnel_attachments` (consumer-side runtime records) and `tunnel_serves` (provider-side runtime records) with `state = 'active'`. This counts runtime records, not distinct tunnels — a single tunnel with one provider-side serve and two consumer-side attachments counts as 3 (one serve plus two attachments). The Layer 1 spec separates these as two parallel runtime records (`tunnel_serves` for provider-side hosting, `tunnel_attachments` for consumer-side connection — see `docs/layer-1/spec.md` for the architectural model); both are first-class for the dashboard's "what's actively running" semantic, so the StatCard counts both. No delta companion in MVP — neither table can reliably reconstruct historical "state=active" at an arbitrary past instant, because the `active`/`stale` flap has no per-state-transition timestamp. The StatCard renders as a plain number (no up/down arrow). See `work-order.md` A.4 Notes ("Snapshot delta semantics") for the full reasoning, and `work-order.md` A.4's `activeTunnels` formula for the exact UNION query.

Each card uses the StatCard primitive and is colored neutrally — no brand color in the stat cards themselves; the brand color appears in deltas (green for up, red or orange for down). Exact aggregation formulas for every field (event types counted, time-window semantics, ordering conventions) live in `work-order.md` A.4's "Aggregation formulas" Notes section, which is the authoritative reference; the prose above is a high-level sketch.

**Activity bar chart** — Envelopes per hour over the last 24 hours, in 1-hour buckets. Uses the BarChart primitive with the indigo accent. Below the chart, a "Last 24 hours" / "Last 7 days" / "Last 30 days" range selector pinned to the upper-right of the chart panel, mirroring the gateway mockups' selector.

**Workgroup breakdown sidebar** — SidebarBreakdown to the right of the chart, listing the top five workgroups by envelope volume in the same window. Each row: workgroup name, horizontal bar with width proportional to share, count.

**Environment status table** — DataTable at the bottom listing every environment in the calling account's organization. Columns: name (a server-side `coalesce(host, description, id)` fallback chain — the `environments` table has no `name` column, so the dashboard resolves a display string from existing columns; see `work-order.md` A.4 Notes "Per-environment attribution" for the source chain and the architectural principle that retired the per-environment `activeSessions` field), status (`online` / `stale` / `unknown` / `disabled` per the convention shared with the existing `agora status` CLI — `online` if heartbeat within 45s, `stale` if the latest heartbeat is older, `unknown` if there's been no heartbeat at all, `disabled` if the environment is administratively disabled), last heartbeat, owning account. Mirrors the gateway mockups' "Provider Status" table — minus a per-environment session count, which has no per-environment source at the current schema level (sessions carry provider/consumer account IDs, not environment IDs).

#### Sessions

The live-activity screen. Mirrors the MCP Gateway mockup's Sessions tab plus the LLM Gateway mockup's Audit Log.

**Four stat cards** at the top: Active Sessions, Sessions Today, Avg Session Duration, Closed-by-Contract-Violation count.

**Active Sessions panel** — DataTable listing every session in `proposed`, `accepting`, `active`, or `closing` state for the calling account (provider or consumer). Columns: session ID (`ses_…`, monospace), role (provider / consumer), counterparty account + organization, advertisement, workgroup, tunnel mode pill, state pill, duration (live-updating), envelope count.

Clicking a row opens a side drawer with the full session detail: KeyValueGrid for the session resource fields, contract snapshot in a code-blocked SectionPanel, and an envelope timeline if the time-series substrate has events for it.

**Recent Sessions panel** — DataTable listing the last 50 closed sessions in the calling account's view. Same columns as Active Sessions plus close reason (StatusPill: `consumer_close`, `provider_close`, `contract_violation`, `tunnel_failed`, `admin_close`) and `close_detail`. Rejected sessions never transition to `state=closed` (their terminal state is `rejected`, surfaced via the distinct `session.rejected` audit event), so they do not appear in this panel; the demo's varied-outcome flow does not exercise the rejection path in MVP — see `work-order.md` D.2 for the visible outcome set.

A search bar above the panels filters by session ID, account, advertisement name, or workgroup name. Filter pills next to the search bar narrow by state and by role.

#### Workgroups

The teams analog. Mirrors the LLM Gateway mockup's Teams & Access tab.

**Top-of-page Structural Security panel** — full-width rounded panel announcing the zero-trust posture, mirroring the gateway mockup's "Structural Security" panel. Right side carries member count, MFA-equivalent badge ("Identity-bound 100%"), and "Zero-Trust Active" badge.

**Workgroup cards** — a three-column grid of cards, one per workgroup the calling account is a member of. Each card shows: workgroup name, intra-org or inter-org badge, member count, the account's role in this workgroup (member / admin), 24h envelope total (a single number sourced from `GET /v1/dashboard/workgroups-activity` — a per-visible-workgroup aggregation that returns one row for every workgroup the caller is a member of, including zero-activity rows; *not* the same shape as the Dashboard tab's SidebarBreakdown which uses `GET /v1/dashboard/activity`'s top-N `byWorkgroup` slice — see work-order.md C.6 Notes "Per-card totals source" for the architectural distinction and the project-wide principle), and the advertisements scoped to this workgroup as small pills at the bottom.

The workgroup card is the dashboard's analog of the gateway mockup's "Teams" card — same shape, same density of information.

**Members table** — DataTable below the workgroup cards listing the union of members across all workgroups the calling account belongs to. Columns: name (initials avatar + email for same-org members; falls back to `accountId · organizationId` for cross-org members whose email is redacted by the API's visibility predicate — see work-order.md C.6 Notes "Cross-org member display fallback" for the architectural rationale and the project-wide principle), workgroup memberships (comma-joined list of workgroup names where this account appears), role pill. Identity status and last-active columns are deferred — they would require joining workgroup memberships with `environment.heartbeat` events at the server, which is out of scope for MVP. See work-order.md C.6 notes for the rationale.

The dashboard does not in MVP support workgroup creation, member add/remove, or invitation acceptance. Those operations live in the CLI and stay there; the dashboard is read-mostly. A single exception: clicking the workgroup card opens a drill-down drawer that includes a copy-to-clipboard for `agora workgroup describe <wg_id>` so an exec can hand the ID to engineering.

#### Catalog

The discovery surface. Mirrors the MCP Gateway mockup's Backends and Tools tabs combined.

**Top-of-page panel** — full-width rounded panel announcing visibility scope. "This catalog shows N advertisements visible to your account across M workgroups." Right side carries advertisement count, workgroup-scope count, and a "Visibility Enforced" badge. The wording deliberately reads "your account" rather than "your environment" — the dashboard's catalog visibility is keyed by the calling account's workgroup memberships, not by any environment-level filter (per the "Identity scoping" section above and `work-order.md` A.5 Notes' explicit rejection of an `environmentId` query parameter).

**Advertisement cards** — list of advertisement cards, one per visible advertisement. Each card shows: advertisement name, owning organization, tunnel mode pill, workgroup scope pills, contract reference pill (or "no contract"), and a "Propose Session" affordance — the affordance is non-functional in MVP and shows a tooltip pointing the user to the CLI verb. The card layout mirrors the MCP Gateway mockup's "Backends" card. Per-advertisement recent session and envelope counts are *not* shown — that data would require new server-side aggregation against `audit_events` keyed by `advertisement_id`, and the activity story belongs on the Sessions tab anyway. The Catalog is a discovery surface (what's available, who provides it, what contract governs it); see work-order.md C.7 notes for the rationale. Owner-account identity (e.g., the producer's email) is intentionally not rendered on the card — the catalog's visibility predicate spans organizations, so leaking owner-account emails cross-org would mirror the same concern A.6 addresses for the Sessions tab. The "owning organization" display is sufficient for the demo's discovery story.

The LLM Gateway and MCP Gateway, when running in agora mode from their own repositories, publish advertisements that appear here alongside the Macro Pulse providers, with their gateway-product gradient as the card's accent (cyan for LLM, violet for MCP). This is the visual moment where the family-of-products story reads in a single screenshot: three branded surfaces, one catalog, one governance root. The gateways do not yet have their own dashboards; drilling into a gateway card from agora's catalog is out of scope for this slice. The card is the representation, and that is enough to land the family-of-products signal honestly.

**Side panels** to the right of the advertisement list:
- **Tunnel modes** breakdown (`tcp`, `http`, `udp` counts)
- **Contracts** breakdown (advertisements per referenced contract)

Filter pills at the top of the list narrow by tunnel mode, by workgroup, or by owning organization.

### Stretch tabs

#### Contracts

A read-only contracts surface. Mirrors the LLM Gateway mockup's Routing Policies tab.

Lists every contract owned by the calling account or referenced by any advertisement visible to the calling account. Each contract shows its terms inline (max duration, max envelope count, max envelope bytes, allowed message types, required workgroup memberships, maturity requirements, access mode) and the advertisements that reference it. Contract creation lives in the CLI; the dashboard is read-only.

#### Audit Log

The full audit surface. Mirrors the LLM Gateway mockup's Audit Log tab. Filterable timeline of every event — session lifecycle, envelope flow (aggregated, not per-envelope), tunnel attach and disconnect, contract violations. Filter by event type, by workgroup, by account, by time range. Designed to be exportable: a button in the upper right kicks off a CSV download. The Compliance Report button from the LLM Gateway mockup is also present and renders a single-page summary.

## Time-Series Substrate

The dashboard requires a real history of activity. The current Agora persistence layer stores state, not history: a session row records its terminal close reason but not its envelope flow over time, and there is no record at all of envelope-level events. This work introduces the substrate.

The implementation is intentionally narrow — exactly enough to drive the dashboard, no more. It is a precursor to the post-MVP metrics work tracked in `docs/roadmap/post-mvp.md`, not a substitute for it: the metrics work calls for ingest and query APIs, org-scoped visibility, automatic emission, and TimescaleDB-backed storage. The dashboard's substrate uses plain PostgreSQL, in-process emission, and a single read path.

### Schema

A single new table, `audit_events`, append-only:

```
CREATE TABLE audit_events (
  id              bigserial PRIMARY KEY,
  occurred_at     timestamptz NOT NULL,
  event_type      text NOT NULL,
  organization_id text NOT NULL,
  account_id      text,
  workgroup_id    text,
  session_id      text,
  advertisement_id text,
  contract_id     text,
  envelope_id     text,
  data            jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_audit_events_org_occurred       ON audit_events (organization_id, occurred_at DESC);
CREATE INDEX idx_audit_events_org_type_occurred  ON audit_events (organization_id, event_type, occurred_at DESC);
CREATE INDEX idx_audit_events_account_occurred   ON audit_events (account_id, occurred_at DESC);
CREATE INDEX idx_audit_events_session_occurred   ON audit_events (session_id, occurred_at DESC);
CREATE INDEX idx_audit_events_workgroup_occurred ON audit_events (workgroup_id, occurred_at DESC);
```

Names follow the existing `idx_<table>_<columns>` convention used across every prior migration (verified against `internal/persistence/migrations/0002_core_indexes.sql` through `0007_layer2_contracts.sql`). The `(organization_id, event_type, occurred_at)` composite is the load-bearing index for the dashboard aggregation queries documented in `work-order.md` A.4: most stat-card and chart formulas filter by org × event-type × time-window, and the composite supports an index-only scan rather than forcing a join across two single-column indexes. The work-order's A.1 migration creates these exact five names and drops them in reverse-create order in its `+migrate Down` section.

The table is bounded in practice by the demo cadence; no rollup or retention policy ships in MVP. A trivial nightly delete would be added if the demo runs long enough to need it.

### Event taxonomy

The MVP event types and their semantic shape:

| Event type | Triggered by | Required keys | Notable `data` | Tenant attribution |
| --- | --- | --- | --- | --- |
| `session.proposed` | controller `proposeSession` | session_id, organization_id, account_id (the attributed-tenant's), workgroup_id, advertisement_id | provider_account_id, provider_organization_id, consumer_account_id, consumer_organization_id, tunnel_mode | **two-party** |
| `session.accepted` | controller `acceptSession` | session_id, organization_id, account_id (the attributed-tenant's), workgroup_id, advertisement_id | provider_account_id, provider_organization_id, consumer_account_id, consumer_organization_id, tunnel_id, contract_id (nullable) | **two-party** |
| `session.rejected` | controller `rejectSession` | session_id, organization_id, workgroup_id, advertisement_id | provider_organization_id, consumer_organization_id, reason | **two-party** |
| `session.closed` | controller close paths and reaper | session_id, organization_id, workgroup_id | provider_organization_id, consumer_organization_id, close_reason, close_detail, duration_seconds, violation_dimension (when `close_reason=contract_violation`: `max_duration` / `envelope_count` / `envelope_bytes` / `message_type`) | **two-party** |
| `envelope.flowed` | controller `reportSessionEnvelopeCount` | session_id, organization_id, workgroup_id, advertisement_id | provider_organization_id, consumer_organization_id, count_delta, total_count | **two-party** |
| `tunnel.attached` | controller observes a new consumer-side tunnel attachment | session_id (when applicable), organization_id, account_id | tunnel_id, final_state (`disconnected` / `stale`, on detached only) | one-party (the consumer-side organization owning the attachment row) |
| `tunnel.detached` | controller observes a consumer-side tunnel attachment leaving the Active state | session_id (when applicable), organization_id, account_id | tunnel_id, final_state | one-party (the consumer-side organization owning the attachment row) |
| `advertisement.published` | controller `publishAdvertisement` | organization_id, account_id, advertisement_id | name, tunnel_mode, workgroup_scopes | one-party (the publishing org) |
| `advertisement.retracted` | controller `retractAdvertisement` | organization_id, advertisement_id | reason | one-party (the retracting org) |
| `environment.heartbeat` | controller `heartbeatEnvironment` | organization_id, account_id | environment_id, latency_ms | one-party (the environment's owning org) |
| `account.login` | controller `Login` (success path) | organization_id, account_id | email | one-party (the logging-in account's org) |
| `account.login_failed` | controller `Login` (unauthorized path, only when the email matched a known account) | organization_id, account_id (of the matched account) | email_attempted | one-party (the matched account's org) |
| `account.logout` | controller `Logout` | organization_id, account_id | email | one-party (the logging-out account's org) |

The `envelope.flowed` event is intentionally aggregate, not per-envelope: it fires whenever the controller receives an envelope-count heartbeat from a provider runtime, and records the delta versus the previous report. This keeps event volume bounded by heartbeat cadence (every 10 seconds per active session) rather than by raw envelope traffic. The dashboard's envelope aggregations sum the `count_delta` field across rows in the window — not the row count — so the displayed values are actual envelope totals, not heartbeat counts. `count_delta` is therefore a required field on every `envelope.flowed` row's `data` jsonb; missing values are coerced to zero by the aggregation formulas but indicate a controller-side bug in `reportSessionEnvelopeCount`'s emission path.

**Two-party event attribution.** A session is a transaction between two organizations: a provider and a consumer. Both organizations have a legitimate dashboard interest in every event the session emits — the provider sees "I served N envelopes today," the consumer sees "I consumed N envelopes today," and both numbers should be honest from each org's vantage point. Five event types in the taxonomy above carry two-party attribution: `session.proposed`, `session.accepted`, `session.rejected`, `session.closed`, and `envelope.flowed`. For these events, the controller emits **one row per attributed tenant**, where the row's `organization_id` is the attributed tenant's. When provider and consumer belong to different organizations (the inter-org case — every Macro Pulse demo session, plus the gateway-services traffic), the controller emits two rows per logical occurrence with the same `(session_id, event_type, occurred_at)` tuple but different `organization_id` values, one keyed on `provider_organization_id` and one on `consumer_organization_id`. When provider and consumer belong to the same organization (the intra-org case — the three intra-org workgroups in the demo: `markets-internal`, `weather-internal`, `signals-internal`), the controller emits exactly one row, because two rows would double-count for that org. The `data` jsonb on every two-party event row carries both `provider_organization_id` and `consumer_organization_id` regardless of which side this row was attributed to, so any downstream consumer that needs the full transaction shape can recover it from a single row. This dual-row-at-emit-time pattern — rather than a single-row-with-OR-predicate pattern — is what lets the dashboard's aggregation queries stay simple (`where organization_id = $callerOrgID` everywhere, no joins, no OR clauses); the per-tenant denormalization is handled at emit time so query time stays fast and uniform. See `work-order.md` A.3 Notes' "Two-party event attribution" subsection for the per-handler emission rules and the project-wide architectural principle that anchors this pattern.

The dashboard's `envelopesToday` and per-workgroup envelope totals therefore work correctly for any caller's vantage point on the demo: the orchestrator (`demo@agora.local`, in `enterprise-client`) is the consumer in every Macro Pulse session, so each `reportSessionEnvelopeCount` heartbeat from a Macro Pulse worker emits two `envelope.flowed` rows — one keyed on the worker's provider-org (`markets-co`, `weather-co`, etc.) and one keyed on `enterprise-client`. The demo dashboard's queries against `organization_id = enterprise-client_org_id` see the consumer-side rows and report non-zero envelope activity. A worker-account login (e.g., `equity-feed@markets-co`) would see the provider-side rows for the same flow events, and report the same envelope counts from its own vantage point. Without this attribution rule, the natural single-row-at-provider-org emission would leave `demo@agora.local`'s dashboard showing live sessions but zero envelopes — the bug DASH-WO-039 caught.

The `account.login_failed` event is intentionally restricted to the case where the email matched a known account but the password was wrong. This is the dashboard-useful case — "someone tried your.account@org.com with the wrong password" — and it has full organization context, satisfying the schema's `organization_id NOT NULL` invariant. The other failure case (the email did not match any known account) is genuinely unattributable to any tenant: there is no organization to scope it to, and no per-tenant dashboard could ever surface it. Unknown-email attempts are logged via the controller's normal application logger (`dl.Warnf(...)`) but are *not* written to `audit_events`. Aggregate cross-tenant security signals like "lots of unknown-email scans" are out of MVP scope; if they become a real need later, the answer is a separate `system_audit_events` table that is orthogonal to the per-tenant substrate, not a relaxation of the per-tenant invariant.

Contract violations do not have their own event type. They are surfaced through `session.closed` events with `close_reason=contract_violation`. Three controller paths produce this close reason — accept-time contract checks in `acceptSession.go`, the `RunSessionDurationReaper` for max-duration overruns, and runtime-reported violations via A.7's `closeSession` extension (envelope-size or message-type enforcement). All three converge on the same logical occurrence — a `session.closed` event with `close_reason=contract_violation`, the `close_detail` carrying the descriptive string ("envelope 4096 > 1024", "max_duration_exceeded after 32s", etc.), and the `data` jsonb optionally carrying a structured `violation_dimension` key for filtering. The audit-substrate row count for that occurrence follows the dual-row emission rule from A.3's "Two-party event attribution" — one row for intra-org sessions, two rows for inter-org sessions, one row per attributed tenant — exactly the same shape every other `session.closed` event uses regardless of which path produced it. The Audit Log stretch screen (C.9) and the Sessions tab's "Closed-by-Contract-Violation" tally both read `close_reason` directly; neither needs to UNION across event types. Earlier drafts of this taxonomy defined a separate `contract.violation` event type emitted by the reaper, which would have introduced an event-type asymmetry (the reaper path emitting both `session.closed` AND `contract.violation` for the same logical occurrence, while the other paths emit only `session.closed`, forcing every consumer to UNION across event types from one path but not the others) and had a required-keys list that omitted `organization_id` despite the schema's NOT NULL invariant on that column. That separate event is retired; see `work-order.md` A.3's "Per-event details" Note for how `close_reason=contract_violation` is wired uniformly across the three paths, and A.7's Notes for the runtime-emission shape.

### Emission

Events are emitted in-process by the controller, in the same transaction as the state mutation that produced them. The emission helper lives in `internal/persistence/audit_events.go` and exposes a single function whose signature matches the existing repository convention:

```go
func (r *AuditEventsRepository) Record(ctx context.Context, q persistence.Queryer, event AuditEvent) error
```

The helper takes a `persistence.Queryer` — the same interface every existing repository method uses — so it composes naturally with both transactional and non-transactional call sites. Inside a `Store.WithTx(ctx, func(tx persistence.Queryer) error { ... })` block, the helper is called with the `tx` variable; in a handler that has no existing transaction, the handler wraps both its single state mutation and the audit insert in a fresh `Store.WithTx` so they commit atomically.

Every controller handler that mutates Layer 2 state gains a `Record` call alongside its mutation. The wrapping policy depends on what the handler already does:

- **Single-mutation handlers** that today call a repository method directly against `s.store.DB()` (`proposeSession`, `rejectSession`, `retractAdvertisement`, `publishAdvertisement`, `heartbeatEnvironment`, `reportSessionEnvelopeCount`) gain a `Store.WithTx` wrap that contains both the existing mutation and the audit insert. The cost is one extra BEGIN/COMMIT per request, which is acceptable for the request volumes involved.
- **Handlers that already use `Store.WithTx`** for multi-step persistence (`acceptSession`'s success path, `closeSession`) add the audit insert inside the existing block, using the `tx` variable already in scope.
- **Handlers with external side effects between mutations** (`acceptSession` provisioning a Ziti tunnel between `MarkAccepting` and `MarkActive`, `enableEnvironment` enrolling a Ziti identity, etc.) record the audit event for the *final persisted state*, not the pre-side-effect intent. The audit insert lives in the same `WithTx` that commits the final mutation. Compensation paths (e.g., `MarkClosed` with reason `tunnel_failed` when provisioning fails) get their own `WithTx` wrapping the compensating mutation and a corresponding audit event (e.g., `session.closed` with `close_reason='tunnel_failed'`). A single user action therefore emits exactly one audit event per terminal outcome — never both the success event and a compensation event for the same attempt.
- **Auth-path handlers that are read-only at the persistence layer except for the audit insert** (`Login`, `Logout`) call `Record` directly with `s.store.DB()` — no `WithTx` wrap, because there is no other write to commit alongside. Audit-insert failure on these handlers is logged via `dl.Warnf(...)` and does *not* fail the user's request: the user still gets their `AccountTokenResponse` (login) or empty `200` (logout) regardless. Failing a user's authentication because the audit row didn't write would be the wrong trade-off for a demo-grade substrate; if a future hardening pass wants stricter auth-audit guarantees, that's a separate decision.

There is no out-of-process emission, no message queue, no buffering; emission failure inside a `WithTx` block rolls back the surrounding transaction, which is the correct behavior for a substrate this narrow.

### Aggregation REST endpoints

All controller HTTP routes are served under the `/v1` prefix (the controller mounts the generated ogen router with `api.WithPathPrefix("/v1")`). Every URL in this document — existing endpoints, new endpoints introduced by this work, login/logout, the health probes — is given in its full `/v1/...` form. The browser, the CLI, and any verification `curl` use that same prefix verbatim. There is no SPA-side path rewriting and no separate browser-facing path; `/v1` is the one prefix that matters. (The two exceptions: the controller's `/health` and `/ready` probes sit outside `/v1`, intentionally.)

Four new endpoints, all account-token authenticated, all scoped to the calling account's organization (per "Identity scoping" above — the dashboard does not filter by environment in MVP). They are added to a new OpenAPI module `internal/api/specs/dashboard/`.

**`GET /v1/dashboard/summary`** — the Dashboard tab's top-of-page numbers. Returns aggregations over the calling account's organization, plus the calling account's identity (used by the Dashboard tab's "current account" callout; the chrome's org indicator sources the same identity from the SPA's auth store via `/v1/account/whoami`, per G.13).

Response:
```
{
  "account": { "accountId": "acc_...", "email": "demo@agora.local", "organizationId": "org_...", "organizationName": "...", "role": "admin" },
  "stats": {
    "activeSessions": <int>,
    "activeSessionsDelta7d": <int>,
    "envelopesToday": <int>,
    "envelopesYesterday": <int>,
    "activeWorkgroups": <int>,
    "activeTunnels": <int>
  },
  "ribbon": {
    "workgroupCount": <int>,
    "advertisementCount": <int>,
    "sessionsToday": <int>,
    "environmentCount": <int>
  }
}
```

The `account` object identifies the *caller* — the cookie-authenticated account whose session opened this request — not a filter applied to the stats. Most fields map directly onto the existing `accountPrincipal` struct (`internal/controller/service.go` line 35): `accountId`, `email`, `organizationId`, and `role` are read off the principal. The exception is `organizationName`, which is not a field on the principal — the principal carries the organization's opaque ID but not its display name. The handler resolves the name via a single primary-key lookup against the `organizations` table (`s.store.Organizations.GetByID(ctx, db, principal.OrganizationID).Name`); this is one indexed read per summary request. See `work-order.md` A.5 Notes ("Principal vs display name") for the implementation detail and the project-wide principle this surfaces. Stats and ribbon counts are organization-scoped per "Identity scoping" in the Information Architecture section.

**`GET /v1/dashboard/activity?window=24h&bucket=1h`** — the Dashboard activity chart.

Response:
```
{
  "buckets": [
    { "start": "<RFC3339>", "envelopes": <int>, "sessions": <int> },
    ...
  ],
  "byWorkgroup": [
    { "workgroupId": "wg_...", "workgroupName": "...", "envelopes": <int> },
    ...
  ]
}
```

`window` accepts `24h`, `7d`, `30d`. `bucket` accepts `1h`, `6h`, `1d` and is validated to be smaller than `window`.

**`GET /v1/dashboard/environments`** — the environment status table.

Response: array of `{ id, name, accountId, status, lastHeartbeatAt }` where `status` is one of `online` / `stale` / `unknown` / `disabled` per the convention shared with the existing `agora status` CLI (45-second heartbeat-staleness threshold; see `work-order.md` A.4 Notes for the full derivation). The `name` field is a server-side `coalesce(host, description, id)` fallback chain (the `environments` table has no `name` column); see `work-order.md` A.4 Notes "Per-environment attribution" for the source chain and the architectural principle that retired the per-environment `activeSessions` field from MVP.

**`GET /v1/dashboard/workgroups-activity?window=24h`** — the per-visible-workgroup activity totals consumed by the Workgroups screen's per-card 24h envelope number.

Response:
```
{
  "byWorkgroup": [
    { "workgroupId": "wg_...", "workgroupName": "...", "envelopes": <int> },
    ...
  ]
}
```

`window` accepts `24h`, `7d`, `30d`. The row set is **every** workgroup the calling account is a member of, including workgroups with zero `envelope.flowed` events in the window (which appear with `envelopes: 0`). The row schema matches `GET /v1/dashboard/activity`'s `byWorkgroup` array intentionally — both are `{workgroupId, workgroupName, envelopes}` — but the *population rule* differs: `getDashboardActivity.byWorkgroup` is top-N by activity (sized for the Dashboard tab's "top 5 workgroups" SidebarBreakdown), while `getWorkgroupsActivity.byWorkgroup` is per-visible-set (sized for the Workgroups tab's per-card grid). See `work-order.md` C.6 Notes "Per-card totals source" for why this is a separate endpoint rather than a parameter on the existing one, and `work-order.md` A.5 Notes "Endpoint scope discipline" for the project-wide principle that anchors when sibling endpoints are warranted.

The other dashboard tabs read directly from the existing Layer 2 endpoints (`GET /v1/sessions`, `GET /v1/workgroups`, `GET /v1/catalog/advertisements`, `GET /v1/contracts`) plus the four dashboard endpoints described above, with screen-level breakdowns (tunnel-mode counts, contract counts, per-workgroup totals) computed client-side from those responses. The Sessions screen needs two additional query parameters on the existing `GET /v1/sessions` operation — `sort` and `limit` — to support its "Recent 50 closed by `closed_at`" panel; that small extension is captured as work-order unit A.6. No other new server-side aggregation is needed for the day-one screens; per-advertisement activity counts, member identity status, and per-workgroup time-series sparklines are either reduced in scope or cut from MVP. See work-order.md C.6, C.7, and C.8 notes for the per-screen rationale.

## Gateway Integration

The LLM Gateway and MCP Gateway live in their own repositories. They are existing products that the company is actively selling; agora does not own them. The integration is bidirectional and minimally invasive: the gateways gain an agora-mode flag, and the agora dashboard surfaces them in its catalog when they are running in that mode.

### The agora-mode flag

Each gateway gains a `--network` flag (or equivalent in its existing config style) that selects the network fabric it operates on:

- `--network=zrok` (or unset) — the existing behavior. The gateway runs as it does today over zrok.
- `--network=agora` — the gateway initializes the agora SDK and, after its existing initialization, publishes an advertisement that announces its presence on the agora fabric.

The agora-mode advertisement carries:

- name: the gateway instance name from configuration (e.g., `engineering-prod`, `engineering-tools`)
- tunnel mode: `tcp`
- description: a one-line product summary
- workgroup scopes: configured per gateway instance, defaulting to a single `gateway-services` workgroup that the demo bootstrap provisions
- contract reference: the advertisement's `contractId` field references the opaque ID (matching `^con_[a-z0-9]{12}$`) of a contract whose `name` is `gateway-services-default` — a default contract with conservative bounds, also bootstrap-provisioned. (The opaque ID is generated at provisioning time; the bootstrap creates the contract with the name `gateway-services-default` and seats its generated ID into the gateway advertisement. Throughout this design and the work order, when contract names appear in prose — `gateway-services-default`, `macro-pulse-provider-default` — those are values of `Contract.name`, not values of `Contract.id` or `Advertisement.contractId`. See `work-order.md` C.7 Notes for the schema distinction the screens have to honor.)
- capabilities (jsonb on the advertisement): a list of high-level capability tags (`["llm-routing", "anthropic", "openai"]` for the LLM Gateway, `["mcp-tools", "filesystem", "github", "postgres"]` for the MCP Gateway)

The gateways do not accept or respond to Agora sessions in MVP. The advertisement is descriptive, not active. This is the "shallow integration" decision: the gateways' existing transports continue to operate as they do today; Agora sees them, governs their visibility, and shows them in the catalog, but does not route their tool calls. Deep integration — routing tool calls as Agora envelopes through Agora sessions — is out of scope for this slice and is a separate post-MVP design conversation.

### Implementation lives in the gateway repositories

The agora-mode flag, the SDK initialization, and the advertisement publication all live in the LLM Gateway and MCP Gateway repositories — not in agora. Agora's repository stays free of gateway-specific code. This decision keeps agora self-contained and OSS-publishable without dragging gateway code along, and keeps the gateways as standalone products that interoperate with agora rather than subsystems of it. Customers continue to deploy the gateways the same way they do today; agora-mode is an additive option, not a fork.

The agora dashboard work expects the gateway binaries to be installed and runnable separately. The demo orchestration (Track F in `work-order.md`) invokes them from their installed locations via configurable paths, defaulting to a `$PATH` lookup with `AGORA_LLM_GATEWAY_BIN` and `AGORA_MCP_GATEWAY_BIN` env vars as overrides. If the gateways are not installed, the launch script warns and continues without them — the demo still runs end-to-end, the catalog simply omits those cards.

### Gateway dashboards are out of scope

The LLM Gateway and MCP Gateway do not currently have dashboards. The mockups for both products exist as design references but are not shipping product. Building those dashboards is a separate slice that comes after agora's dashboard work, after the shared component kit is extracted from agora's `ui/` into a reusable package, and after an orchestration layer is designed that lets a gateway dashboard integrate with agora's. This sequencing is deliberate: agora's dashboard is the proving ground for the design system, and once it has shipped, the kit can be lifted out and the gateway dashboards become its next consumers.

The dashboard representation of the gateways in this slice is therefore appropriately modest: each gateway appears in agora's Catalog as a card with the gateway-product gradient on the accent. That is enough to land the family-of-products signal without pretending to portal into dashboards that do not yet exist.

## Continuous Macro Pulse

The current `macro-pulse-pulse-agent` runs once: it iterates the catalog, exchanges one envelope per advertisement, and exits. The dashboard needs the system to feel alive across an entire walkthrough — counters tick, sessions transition, "12s ago" entries scroll in.

The change is small. `macro-pulse-pulse-agent` gains a `--loop` flag (and an `AGORA_PULSE_LOOP=1` environment variable) that switches it from one-shot to continuous mode. In loop mode:

- the agent picks a random pause between iterations, drawn from a configurable interval distribution (default: uniform between 20 and 60 seconds)
- each iteration picks a random subset of visible advertisements rather than iterating all of them, drawn from a configurable count (default: uniform between 2 and 5 advertisements per iteration)
- each session has a small probability of hitting a contract violation (by sending a too-large envelope, a disallowed message type, or holding past the duration cap) or of being left active for a long-tail duration before closing — these probabilities are configurable but default to roughly 5% contract-violated, 15% long-tail, and 80% normal close. The orchestrator's loop runs a deterministic warm-up phase at startup that guarantees at least one of each visible outcome appears in the first few sessions before switching to weighted random (see `work-order.md` D.2). Rejection (the provider's session handler returning an error) is supported by the underlying audit substrate but is not exercised by the demo loop in MVP — the visible outcome set is `contract_violation`, `consumer_close`, and `provider_close`.
- the loop runs until SIGINT/SIGTERM and reports a structured summary on shutdown

Configuration is parsed from a YAML file (`--profile <path>`) so the demo bootstrap can tune the activity profile without touching code, and so different walkthroughs can use different cadences (a high-speed "executive briefing" profile that ticks every 5–15 seconds, a calm "sales conversation" profile that runs every minute, etc.).

## Demo Bootstrap and Orchestration

A new binary, `cmd/demo-bootstrap/`, provisions a complete demo environment from a clean slate. It is a one-shot tool: it talks to the controller's admin and account-token APIs and exits. It does not stay resident.

The bootstrap provisions:

- six organizations (Macro Pulse's existing five, plus a sixth `gateway-services-org` for the LLM Gateway and MCP Gateway when running in agora mode)
- eleven accounts (Macro Pulse's existing nine, plus two additional accounts for the gateway services)
- eight workgroups (Macro Pulse's existing seven, plus one `gateway-services` workgroup)
- the `macro-pulse-provider-default` contract that the existing Macro Pulse providers reference
- a new `gateway-services-default` contract with bounds tuned for gateway-style traffic (longer max duration, larger envelope budget)
- a new `demo-contract-tight` contract with deliberately tight bounds (`max_envelope_bytes=1024`, `max_duration_seconds=30`), referenced by exactly one Macro Pulse worker's runtime advertisement (`news-pulse@signals-co`) so the demo's varied-outcome flow has a concrete advertisement to violate against. See `work-order.md` D.2 for the violation paths and F.1 for the per-worker assignment mechanism.
- a small population of *historical* events: pre-seeded `audit_events` rows spanning the prior 7 days, generated by a randomized synthesizer that varies session counts and envelope volumes hour-by-hour. These are explicitly synthetic and labeled in the bootstrap output, but they make the dashboard's "last 7 days" charts read populated from the moment the demo starts.

A launch script, `bin/demo-up.sh`, ties everything together. Postgres and Ziti are operational preconditions — the script assumes both are reachable per the connection details in `etc/demo-controller.yaml` and does not attempt to start, configure, or stop them.

1. builds the dashboard UI bundle via `(cd ui && npm ci && npm run build)`, populating `ui/dist/` for the Go embed (the untagged Go build embeds `ui/dist` via `//go:embed dist`, and `ui/dist/` is gitignored, so this step is required before step 2 — see `work-order.md` F.2 step 1 for the full contract and B.1/B.2 for the embed and gitignore directives)
2. builds all required Go binaries via `go install ./cmd/... ./examples/macro-pulse/cmd/...` (installs `agora`, `demo-bootstrap`, and the 10 Macro Pulse binaries — see `work-order.md` F.2 step 2 for the full binary list and the "Binary name discipline" Note for why directory layout determines binary names)
3. runs `agora admin store migrate ./etc/demo-controller.yaml up` against the demo's Postgres database (the CLI requires both a config path and an action per `cmd/agora/storeMigrate.go`'s `cobra.ExactArgs(2)`; bare `migrate up` is invalid syntax). Migrate runs *before* the controller starts so the controller never sees an unmigrated schema; the command is idempotent (sql-migrate skips up-to-date migrations) so re-running `demo-up.sh` is a no-op at this step on a synchronized database.
4. runs the Agora controller with the demo config in the background (PID and log path under `$AGORA_DEMO_ROOT/run/` and `$AGORA_DEMO_ROOT/logs/` — see `work-order.md` F.2 step 4 for the contract)
5. runs `demo-bootstrap`, which provisions the topology, seeds historical events, and enrolls 11 env roots under `$AGORA_DEMO_ROOT/envs/<email>/` (one per Macro Pulse agent account plus two for the gateway services)
6. launches the eight Macro Pulse worker agents (`macro-pulse-equity-feed`, `macro-pulse-fx-feed`, `macro-pulse-commodities-feed`, `macro-pulse-weather-feed`, `macro-pulse-search-trends`, `macro-pulse-news-pulse`, `macro-pulse-correlator`, `macro-pulse-narrator`) in the background, each with `AGORA_ENV_ROOT` pointing at its enrolled env root from step 5. Each worker publishes its advertisement and registers a session handler. Without these workers, the catalog is empty and the orchestrator in the next step has nothing to call.
7. runs `macro-pulse-pulse-agent --loop --profile=etc/demo-profile.yaml` in the background, with `AGORA_ENV_ROOT` pointing at its enrolled env root. Looked up from `$PATH` with `AGORA_PULSE_AGENT_BIN` as override; aborts with a clear error if not found, since the loop is load-bearing for the demo.
8. runs `llm-gateway --network=agora` and `mcp-gateway --network=agora` in the background, looking up their binary locations from `$PATH` (with `AGORA_LLM_GATEWAY_BIN` and `AGORA_MCP_GATEWAY_BIN` env vars as overrides). If either binary is not found, the script logs a warning and continues — the demo still runs, the catalog simply omits that gateway's advertisement.
9. opens `http://localhost:<port>` in the default browser

A companion `bin/demo-down.sh` tears the agora stack down cleanly: stops the eight worker agents, then the orchestrator, then the gateways, then the controller, preserving `$AGORA_DEMO_ROOT/envs/` and `$AGORA_DEMO_ROOT/logs/` so a follow-up `demo-up.sh` re-uses the existing enrollments. Postgres and Ziti continue to run; the script does not touch them. A `--purge` flag deletes the entire `$AGORA_DEMO_ROOT/` tree for clean-slate behavior. A `WALKTHROUGH.md` document under the demo directory captures the click-path the salesperson follows, with talking points keyed to each screen.

## Authentication Posture

The dashboard authenticates users with a login screen and session cookies. The controller's existing account-token authentication is the bedrock; the cookie layer sits on top of it without replacing it.

### Existing foundations

These pieces are already in place in the agora codebase and the dashboard work reuses them as-is:

- the `accounts` table carries `password_salt` and `password_hash` columns
- argon2id-based `hashPassword`, `verifyPassword`, and `rehashPassword` helpers in `internal/controller/passwords.go`
- a working `POST /v1/account/login` endpoint that returns `AccountTokenResponse{accountToken}` on success
- a working `POST /v1/account/change-password` endpoint
- a working `POST /v1/organizations/{organizationId}/accounts` endpoint (operationId `createAccount`) that the demo bootstrap uses, secured by `adminTokenAuth`
- the existing account-token authenticator behind every protected endpoint, driven by the `X-TOKEN` header

The dashboard work does not modify any of these. The login response shape stays at `AccountTokenResponse{accountToken}` because the CLI consumes that same endpoint and the same shape.

### Cookie layer

On a successful login, two cookies are set on the response:

- **`agora-session`** — `httpOnly`, `Secure` (when TLS is configured), `SameSite=Strict`, `Path=/`. Carries the account token. JavaScript cannot read it; an XSS regression cannot exfiltrate it.
- **`agora-csrf`** — readable by JavaScript, `Secure` (when TLS), `SameSite=Strict`, `Path=/`. Carries a randomly-generated CSRF token (using the existing `nanoid`-style token generator).

A **cookie-to-header middleware** wraps the ogen handler. For incoming requests with no `X-TOKEN` header but a present `agora-session` cookie, it copies the cookie value into `X-TOKEN` before routing. The existing account-token authenticator runs unchanged. CLI requests, which arrive with `X-TOKEN` directly and no cookies, bypass the cookie path entirely.

A **CSRF middleware** enforces the double-submit pattern on state-changing methods (`POST`, `PUT`, `PATCH`, `DELETE`) when the request arrives with a session cookie. The `agora-csrf` cookie value must match the `X-CSRF-Token` request header, or the request is rejected with `403`. Login itself bypasses CSRF (no session exists yet). Logout bypasses CSRF (the action is to clear, and it requires a session cookie to be meaningful). Pure CLI requests (no session cookie present) bypass CSRF entirely.

A **login-cookie-emit middleware** runs on the response side. For successful (`200`) responses to `POST /v1/account/login` and `POST /v1/account/regenerate-token`, it parses the response body's `accountToken` field and sets the `agora-session` and `agora-csrf` cookies on the response. The body itself is forwarded unchanged so the CLI continues to receive the token in its expected shape.

### Logout

A new endpoint, `POST /v1/account/logout`, is added to the OpenAPI spec under the existing `account` module. It requires no authentication (it is idempotent and exists to clear cookies). The response is empty. A response-side middleware sets `MaxAge: -1` on both the `agora-session` and `agora-csrf` cookies. The dashboard SPA calls this on user-initiated logout; the CLI never calls it.

### Session lifetime

Session cookies have no explicit `Expires` and persist as session cookies in the browser (cleared when the browser closes or by explicit logout). The underlying account token is permanent — there is no server-side session table. This matches the demo-grade posture without precluding production-grade session expiry being added later: a future `Expires` value on the session cookie at login time fits the same architecture.

### Implementation reference

The architecture closely follows the design captured in zrok's `COOKIE_AUTH.md` reference document (preserved alongside this design as `cookie-auth-reference.md`), with several agora-specific simplifications:

- agora's account model already carries `password_salt` and `password_hash`; no schema work is required
- agora ships cookie-based auth from day one — no pre-existing localStorage to migrate from, no `getXApi(user)` call sites to refactor
- agora uses fixed cookie names (`agora-session`, `agora-csrf`); the configurable-cookie-name and `/configuration` exposure mechanism that zrok needs for dynamic name discovery is unnecessary here
- agora has no UI for account-token regeneration in MVP scope; the regeneration flow is wired through the cookie-emit middleware so a future regen UI gets the cookie refresh automatically, but no UI work for it lands now
- agora uses `ogen` (not `go-swagger`) for OpenAPI codegen; the cookie-setting pattern is HTTP middleware around the ogen handler rather than the `middleware.Responder` wrapper pattern zrok uses
- agora's CLI has not yet been deployed to users at scale, so there is no compatibility concern from the X-TOKEN-versus-cookie split

### Demo posture

The `demo-bootstrap` binary provisions a fixed demo account at `demo@agora.local` with the password `Agora-Demo-1` (chosen to satisfy a future hardened password policy without requiring one in MVP). The launch script displays the credentials prominently in its terminal output:

```
Agora demo is running at http://localhost:18080/
   Email:    demo@agora.local
   Password: Agora-Demo-1
```

The salesperson types these at the login screen. Showing the authentication step in the demo is a feature, not a footnote — it visually demonstrates that the system has real authentication rather than implying a punt. The five seconds spent typing credentials establishes posture for the entire walkthrough.

`demo@agora.local` is also the runtime identity of the Macro Pulse orchestrator (`macro-pulse-pulse-agent`). The two identities — the demo audience's dashboard login and the orchestrator process's account — are the same account on purpose: the dashboard's `GET /v1/sessions` is participant-account-scoped, so the only way for the dashboard to show the live Macro Pulse activity is for its login to be a participant in those sessions. The orchestrator is the consumer in every Macro Pulse session it opens, so logging in as the orchestrator account gives the dashboard natural visibility into all the activity. The full reasoning, alternatives considered, and the resulting topology change (the bootstrap renames `pulse-agent@enterprise-client` to `demo@agora.local`) live in `work-order.md` F.1's "Demo visibility model" Note.

The launch script does not auto-login or inject credentials into the URL; the login screen is the demo's first frame.

### Login screen

The login screen is a full-page component outside the AppShell chrome. It renders a centered card with the indigo brand mark at the top, an email field, a password field, a submit button, and an error region beneath. No nav, no env switcher, no status pill — the chrome only appears once authenticated. The card uses the same design tokens as the rest of the dashboard: zinc neutrals, indigo brand color on the submit button and the brand mark, the same border and radius conventions.

The login screen visual is included in the brochure and walkthrough materials so stakeholders can preview it before the build.

## Repository Layout

The dashboard work introduces these new top-level paths in the Agora repository:

- `ui/` — the React SPA. Vite, React 19, Tailwind v4, lucide-react, react-router. Build artifact at `ui/dist/`. Includes the login screen at `ui/src/screens/Login.tsx`.
- `internal/controller/getDashboardSummary.go`, `getDashboardActivity.go`, `getDashboardEnvironments.go`, `getWorkgroupsActivity.go` — the new dashboard aggregation operation handlers, each as a method on `*Service` matching the existing per-operation-file convention. (No `internal/dashboard/` package: ogen's generated `api.Handler` is a single interface implemented by `Service` end-to-end, so a sibling package can't independently implement only the dashboard operations. Four new files in `internal/controller/` is consistent with how every other operation has been added to date.)
- `internal/controller/dashboard_helpers.go` — response-mapping helpers specific to the dashboard endpoints, matching the existing `_helpers.go` convention (`advertisement_helpers.go`, `contract_helpers.go`, etc.). The aggregation query work itself lives in `internal/persistence/audit_aggregations.go`; this file is just for the controller-side mapping layer.
- `internal/persistence/audit_events.go` — the `AuditEventsRepository` (with its `Record` emission method) and the `audit_events` table integration. Registered on `Store` as `Store.AuditEvents` matching the existing repository convention. Migrations live alongside the existing ones in `internal/persistence/migrations/`.
- `internal/controller/logout.go` — the new `Logout` service handler.
- `internal/controller/auth_middleware.go` — the cookie-to-header middleware, the CSRF middleware, the login-cookie-emit middleware, and the logout-cookie-clear middleware. All four middleware functions live together in this single file because they share configuration (cookie names, TLS-aware `Secure` flag) and run as a coordinated stack.
- `cmd/demo-bootstrap/` — one-shot bootstrap binary.
- `etc/demo-profile.yaml`, `etc/demo-controller.yaml` — demo-specific config files.
- `bin/demo-up.sh`, `bin/demo-down.sh` — launch and teardown scripts.
- `docs/dashboard/design.md`, `docs/dashboard/work-order.md`, `docs/dashboard/cookie-auth-reference.md` — these documents and the cookie-auth implementation reference.

The `ui/` build is embedded into the controller binary using the same pattern as zrok: `ui/embed.go` with `//go:embed dist`, `ui/embed_stub.go` for `no_agora_ui` builds, and `ui/middleware.go` that passes through `/v1` and falls back to `index.html` for SPA routes.

## Acceptance Criteria

The dashboard work is complete when all of the following are demonstrably true on a configured demo host (Postgres and Ziti reachable per `etc/demo-controller.yaml`) running the launch script.

**Demo orchestration**

- `bin/demo-up.sh` brings the agora stack up and the dashboard reachable at `http://localhost:<port>` in under 60 seconds, given an already-running Postgres and Ziti reachable per `etc/demo-controller.yaml`.
- `bin/demo-down.sh` tears the agora stack down without leaving stray processes, and without leaving unmanaged files outside `/tmp`, the agora repo, or `$AGORA_DEMO_ROOT/` (which by design preserves `envs/` and `logs/` for warm-restart and post-run log inspection — see `work-order.md` F.2 step 5 and "Demo state preservation" in F.2 Notes for the contract). Postgres and Ziti are external services the script does not manage. Passing `--purge` additionally removes `$AGORA_DEMO_ROOT/` entirely for full clean-slate behavior.
- The demo populates 6 organizations, 11 accounts, 8 workgroups, the `macro-pulse-provider-default`, `gateway-services-default`, and `demo-contract-tight` contracts, and at least 8 mandatory Macro Pulse advertisements plus 0–2 gateway advertisements conditional on the LLM and MCP gateway binaries being present (see "Gateway integration" below for the warn-and-continue handling).
- The demo provisions a `demo@agora.local` account with the password `Agora-Demo-1`. The launch script displays these credentials in its terminal output.
- Pre-seeded historical events span the prior 7 days and produce visibly varied per-day activity in the Dashboard tab's "Last 7 days" view.

**Authentication**

- Visiting `http://localhost:<port>/` while unauthenticated lands on the login screen.
- Logging in with the demo credentials sets `agora-session` (httpOnly) and `agora-csrf` (readable) cookies and lands on the Dashboard tab.
- `localStorage` does not contain the account token at any point — only the email string for display use.
- Authenticated API calls succeed without an `X-TOKEN` header (the cookie carries the session); the cookie-to-header middleware bridges them transparently.
- Pure CLI flow continues to work: `agora account login`, `agora environment enable`, and other CLI verbs operate via `X-TOKEN` exactly as before.
- A cross-origin POST without `X-CSRF-Token` (e.g., crafted via `curl` while the browser holds a session cookie) is rejected with `403`.
- Logging out clears both cookies and returns the user to the login screen.

**Dashboard tab**

- All four stat cards render with real values from the calling account's organization. Deltas are computed from the time-series substrate and update on tab navigation.
- The activity bar chart renders 24 / 7d / 30d windows. Each window shows real envelope volume from the time-series substrate (mixing pre-seeded historical and live continuous-Macro-Pulse activity).
- The workgroup breakdown sidebar shows the top five workgroups by envelope volume in the selected window, with proportional bar widths.
- The environment status table lists every environment in the organization with live status driven by `environments.last_seen_at` (the column updated atomically on every `heartbeatEnvironment` call). The dashboard's `online`/`stale`/`unknown`/`disabled` derivation matches the existing `agora status` CLI byte-for-byte by reading from the same column. See `work-order.md` A.4 Notes ("Heartbeat timing source") for why this reads from the live column rather than aggregating audit events.

**Sessions tab**

- The Active Sessions panel lists every session in `proposed`, `accepting`, `active`, or `closing` state for the calling account, with live duration counters that tick once per second.
- The Recent Sessions panel lists the last 50 closed sessions with close reason pills.
- Clicking a session row opens a drawer showing the session's full detail and contract snapshot.
- Search and filter narrow both panels' results in real time.

**Workgroups tab**

- The Structural Security panel renders with live counts.
- One workgroup card per workgroup the calling account is a member of, with the activity bar showing real recent envelope volume.
- The Members table lists every member across all the caller's workgroups.

**Catalog tab**

- One advertisement card per visible advertisement. The two gateway advertisements (LLM Gateway and MCP Gateway, running in agora mode) appear when their binaries are present; see "Gateway integration" below for the conditional behavior.
- Tunnel mode and contract breakdowns on the right side reflect real distributions.
- The gateway cards visibly carry their gateway-product gradient on their card accent (cyan for LLM, violet for MCP).

**Continuous activity**

- The dashboard's counters and timestamps visibly update during a 5-minute idle observation, driven by `macro-pulse-pulse-agent --loop` and gateway activity.
- Within a 5-minute window of normal activity, the Recent Sessions panel displays multiple distinct close-reason pills, including at least one `contract_violation` row (guaranteed by D.2's deterministic warm-up via the reaper path) and at least one `consumer_close` or `provider_close` row. The deterministic warm-up phase in D.2 guarantees these outcomes appear within roughly the first 2 minutes regardless of where the random-probability tail lands.

**Visual fidelity**

- Side-by-side with the LLM Gateway and MCP Gateway mockups, the Agora dashboard reads as the third member of the same family: same Inter font, same Zinc neutrals, same StatCard typography, same SectionPanel borders, same StatusPill shape.
- The chrome (header + nav + status pill) is structurally identical to the gateway mockups' chrome, differing only in brand color and product name.

**Gateway integration**

- With `llm-gateway --network=agora` and `mcp-gateway --network=agora` running, both gateways' advertisements appear in agora's Catalog with the correct gradient accents (cyan / violet) on their cards.
- With one or both gateways stopped (or not installed), the demo still runs end-to-end. The launch script's terminal output reports gateway availability clearly, and the catalog simply omits the missing cards.
- Building the gateways' own dashboards is out of scope for this slice. The agora dashboard's representation of the gateways is the catalog card; drilling into a gateway from the catalog is not implemented.

## Out of Scope

These items are explicitly deferred. They are not blockers, and proposing them mid-flight should be redirected to a follow-on slice.

- multi-tenant isolation on the dashboard surface beyond the existing organization-scoped authentication
- registration, password reset, email verification, MFA, SSO, identity-provider integration; the demo bootstrap creates the demo account directly and the existing CLI continues to be the path for any other account-management operations
- per-account or per-IP rate limiting on login attempts; session expiry and refresh; account lockout
- environment-scoped aggregations on dashboard data; the chrome's org indicator is read-only in MVP and the dashboard endpoints do not accept `environmentId` filters
- a password-change UI in the dashboard; the existing CLI path (`agora account change-password`) remains the only entry point
- a regenerate-account-token UI in the dashboard; the cookie-emit middleware will refresh the session cookie correctly when a future regen UI is added, but no UI for it lands now
- mobile, tablet, or responsive layouts
- accessibility audit beyond what falls out of semantic HTML and keyboard navigability
- internationalization
- per-advertisement detail pages, per-contract detail pages, per-tunnel detail pages
- write paths from the dashboard (creating workgroups, publishing advertisements, proposing sessions, etc.)
- envelope-level audit (the dashboard reads aggregate envelope counts only)
- streaming updates over WebSocket or SSE; the dashboard polls
- the deep gateway integration where tool calls are routed as Agora envelopes
- gateway dashboards: the LLM Gateway and MCP Gateway do not currently ship dashboards; building them is a separate slice that follows agora's dashboard work and the extraction of a shared component package
- gateway-side implementation of the agora-mode flag: lives in the LLM Gateway and MCP Gateway repositories, not in this scope
- the post-MVP metrics work tracked in `docs/roadmap/post-mvp.md` (this work is a precursor that accelerates that roadmap item but does not replace it)
- the post-MVP semantic catalog search, programmatic contracts, workgroup hierarchy, memory-oriented services
- the existing Layer 1 runtime library extraction tracked in `docs/layer-1/status.md`

## Open Questions

None at the design-spec level. All decisions identified during the design conversation are resolved:

- Visual stack: Tailwind v4 + lucide-react, no shadcn, no MUI
- Architecture: Vite/React/embed.go pattern lifted from zrok
- Brand identity: indigo (`#4f46e5` → `#1d4ed8`)
- Gateway integration depth: shallow (advertisement only)
- Gateway repo placement: gateways stay in their own repositories; agora-mode is added there. Agora's repository contains no gateway-specific code.
- Gateway agora-mode invocation: `--network=agora` flag (or equivalent in each gateway's existing config style); binaries keep their existing names (`llm-gateway`, `mcp-gateway`)
- Gateway dashboards: out of scope for this slice; deferred until after agora's dashboard ships and the shared component kit is extracted
- Instance switcher semantics: org indicator is read-only in MVP and displays the calling account's organization name. Dashboard data is scoped by the calling account's organization. There is no environment-level chrome label because the cookie-authenticated principal carries no environment identity. Per-environment filtering, plus making the indicator a real switcher (whether organization-switching or environment-switching), is deferred to a follow-on slice that adds `environment_id` to event emissions and `environmentId` query parameters to the dashboard endpoints.
- Day-one screens: Dashboard, Sessions, Workgroups, Catalog
- Stretch screens: Contracts, Audit Log
- Authentication: cookie-based session auth on top of the existing account-token authenticator; fixed cookie names (`agora-session`, `agora-csrf`); demo account `demo@agora.local` provisioned by `demo-bootstrap`; no registration, password reset, MFA, or SSO in MVP scope

Decisions that may surface during implementation are deferred to the implementation conversation rather than pre-resolved here.
