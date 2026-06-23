# Single-Active Listener

A tunnel in Agora names one logical service, but nothing today makes the overlay treat one *host* of that service as authoritative. More than one live listener can host the same tunnel at once — and when that happens, the fabric does not pick a winner and hold to it; under load it spreads sessions across them. For a service that is supposed to have a single backend, two hosts quietly serving the same name is a correctness problem, not a feature. This document captures the problem and what we learned about the fabric while scoping it. The resolution is a separate design cycle; this is the problem statement it starts from.

## The issue is orthogonal to environment scope

It is tempting to file this under "account-owned tunnels let two environments host the same tunnel" (see [account-owned tunnels](../current/layer-1/spec.md)), but that framing is wrong, and getting it wrong would scope the fix incorrectly. The concern is not about *which* environments may host — it is about *how many live hosts* a tunnel may have at once, and the answer today is "any number, with no authoritative one."

A single environment already produces the situation. The provider primitive establishes more than one terminator per host by default (one per available edge router, for path redundancy), and an environment can run its gateway process twice, or call the listen primitive twice, and stand up two genuinely distinct hosts of the same tunnel. The fabric sees terminators and routes by cost; it has no notion of "environment" in that decision. So two live hosts of one tunnel behave the same way whether they live in one environment or in two environments of the same account.

Account-owned tunnels widen the set of environments *able* to host. That change does not create the multi-host condition, and it does not change its character. The two pieces of work compose cleanly: one governs *ownership and authorization* (whose tunnel it is, and who may host), this one governs *concurrency* (how many hosts are authoritative at once). Keeping them separate is what keeps each honest.

A clarification the fix will have to hold onto: the multiple terminators a *single* host opens across edge routers are not the problem — they are one backend reachable by several network paths, which is the redundancy the fabric is built to provide. The unit of concern is the distinct *host* (a process serving the tunnel), not the per-router terminator. The design must enforce a single authoritative host while leaving each host's per-router terminator fan-out alone.

## What the fabric does, and doesn't

The enforcement options are shaped — and bounded — by what OpenZiti actually offers.

It does **not** offer a way to cap a service at one terminator, nor to reject a second host's bind. A service is inherently multi-terminator; there is no max-terminators knob and no bind-rejection path. So "enforce one host by refusing the second one" is not a mechanism the fabric has. Any design that assumes it will not survive contact with the platform.

What it offers instead is *routing* control. The default strategy, smartrouting, finds all active terminators and routes to the lowest **cost** one — it concentrates rather than fans out. But cost is dynamic and rises with a terminator's circuit count, so two hosts at *equal* precedence will begin to spread traffic as the busy one's cost climbs. Concentration at equal precedence is a tendency, not a guarantee; it is not single-active.

Deterministic single-active comes from **precedence**. Terminators carry a precedence — `required`, `default`, or `failed` — and smartrouting always ranks a higher precedence above a lower one regardless of cost. The platform's own active/passive HA pattern is built on this: the active host's terminators are `required`, standbys are `default`; all traffic goes to the active; when it fails, its terminators are marked `failed` and traffic moves to a standby; on recovery it is bumped back to `required`. Precedence and static cost are settable through the management API, so an authority that holds the management plane — the controller — can drive which host wins.

Two more facts matter for the design. Terminators **self-clean on host disconnect**: when a hosting SDK drops its edge-router connection, its terminators are removed automatically, so a dead host's claim on the service expires on its own (bounded by how quickly the router notices the dead connection). And the same precedence mechanism that enforces single-active **also delivers automatic failover** — which is why this concern and the failover motivation are relatives, but separable: failover is the payoff, single-active is the invariant.

## What "enforced at the fabric" can actually mean

Given the above, "enforced at the fabric" cannot mean bind-rejection. The achievable shape is: **the controller designates one host's terminators authoritative (`required`) and demotes the rest, and smartrouting then enforces single-active and fails over.** Fabric-routing-enforced, controller-directed. The controller, not the fabric alone, is the authority that decides which host is the one.

That has a consequence the design cycle has to confront head-on. To designate the active host, the controller needs a notion of *who the active host is*, per tunnel, for both tunnel shapes. The managed proxy shape already has one — its serve record. The direct shape has none: the listen primitive was deliberately built thin — no controller record, no heartbeat — precisely so a provider's authorization could persist without one (see [sdk-l1-primitives](./sdk-l1-primitives.md)). Enforcing single-active for direct tunnels therefore reaches back into that decision: it wants a control-plane presence for a primitive whose whole point was not to have one, plus a controller responsibility to watch terminators and set precedence. Whether that record is reintroduced, whether the proxy serve record becomes the precedence driver, and how the controller learns of an interloping terminator at all, are the load -bearing questions for the cycle — not things to settle here.

## Open questions for the design cycle

- **Direct tunnels and thin Listen.** Does enforcing single-active require giving direct `Listen` a control-plane host record (reopening the recordless/heartbeatless design), or is there a lighter path — e.g., the controller reconciling terminator precedence from what it can observe on the fabric without the host checking in?
- **Who drives precedence, and from what signal.** For proxy, the serve record is the natural driver. For direct, what is? And what reconciles reality when a host appears that the controller didn't bless?
- **Failover timing.** Single-active's recovery time is bounded by how fast a dead host's terminators are reclaimed (router-to-SDK keepalive detection). What is that bound, and is it good enough, or does the design need an explicit demote path?
- **Service configuration.** Should Agora set an explicit terminator strategy (e.g. the HA strategy) and/or constrain `MaxTerminators` per host, rather than relying on smartrouting defaults?
- **In-flight circuits on failover.** Precedence governs *new* session assignment; what should happen to circuits already established on a host that is being demoted or has died?
- **Relationship to the proxy one-active-serve check.** Today the controller's serve record gates proxy hosting cooperatively (the runtime checks it before binding). Does single-active enforcement subsume that record into a precedence-driving role, or sit beside it?

## Relationship to account-owned tunnels

[Account-owned tunnels](../current/layer-1/spec.md) moved a tunnel's ownership from its creating environment to the account, so the tunnel survives its host and can be hosted from any of the account's environments. It assumes a single active host at a time and defers *enforcing* that to this document — with one exception it carries itself: its `--takeover` is the manual, operator-triggered slice of the eviction this document generalizes, displacing a stale host's terminators on demand. What takeover does not do is *hold* the invariant — it means "take it now," not "keep exactly one host live continuously." This document owns that continuous, automatic enforcement, and the automatic failover that falls out of it. The two are sequenced: account-owned tunnels landed first as a self-contained ownership change that carries manual takeover; single-active enforcement is the continuous concurrency invariant layered on top, still ahead in its own cycle. Enforcement beyond that manual slice should not be smuggled into the ownership change — they answer different questions.
