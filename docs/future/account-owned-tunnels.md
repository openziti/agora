# Account-Owned Tunnels

A tunnel is meant to be a durable thing an account depends on — a named service that clients are wired to and expect to keep finding. Agora almost treats it that way: the name is unique across the organization, the grants outlive any session, and a consumer in any of the account's environments can already reach it. But underneath, a tunnel is *owned* by the single environment that happened to create it. Retire that environment and the tunnel goes with it. Run from a second environment and the controller refuses. The durable resource is quietly pinned to the most ephemeral thing in the system — a particular running environment.

This spec moves ownership to where it belongs: **the account owns the tunnel; an environment only hosts it.** The name and the resource belong to the account, like a reserved name; an environment is simply where the tunnel happens to be running right now, and that can change. This is the model zrok reaches with namespaces and names — a name is created in the instance and owned by the requesting account, and a listening share attaches to the name in order to provide service. Agora's organization-unique tunnel name is the same idea; we just need ownership to sit at the account, not the environment.

## How it works, in practice

Follow one tunnel through its life. Alice runs an LLM gateway and wants it reachable at a stable name her teammates can wire to.

**She creates it.** `my-important-tunnel` comes into being as a thing *her account* owns — a reserved name with a service behind it. She runs the create command from her laptop, but the tunnel isn't her laptop's; the laptop is just where she was standing when she made it.

**She serves it.** She hosts the gateway from her laptop. The controller notes that the laptop is currently hosting `my-important-tunnel`. Teammates connect by name and traffic flows.

**Her laptop dies.** The tunnel doesn't. It's the account's, not the laptop's. Alice starts the gateway on her always-on server instead; the server is now the host. Her teammates — who only ever knew the name — reconnect to the same tunnel and never learn which machine is behind it. Nothing was recreated and nothing was rewired; the host just moved.

**She retires the old laptop for good.** She decommissions that environment entirely. The tunnel stays standing. Retiring an environment tears down what that environment was *doing* — the hosting it was running, the connections it held — not the account's durable tunnels. (This is the part that doesn't work today, and the heart of why ownership has to move: today, retiring the creating environment deletes the tunnel out from under everyone wired to it.)

**A host goes half-dead.** One afternoon the server is wedged — still connected, not actually serving. Alice wants to move hosting back to her laptop, but the server is still claiming the spot. She serves with `--takeover`: it evicts the server's stale claim and her laptop takes over. (If the server were genuinely healthy, takeover refuses unless she insists with `--force` — so she can't knock over a working host by fat-fingering a name.)

**The one thing it won't promise yet.** If Alice accidentally serves `my-important-tunnel` from two healthy environments at once, the system doesn't yet stop her, and traffic could split between them. Guaranteeing *exactly one* active host at all times is a separate, harder piece of work; see [single-active-listen](./single-active-listen.md). What this spec guarantees is ownership and movability — the tunnel survives its host and can be hosted from anywhere the account runs — not concurrency control.

## Why the environment is the wrong owner

The consumer side already shows the right shape. Any of an account's environments can reach the account's tunnels — the right to *reach* is an account-level fact, granted once and usable from anywhere the account runs. Only the right to *host*, and the ownership of the resource itself, were pinned to one environment. That split is the whole bug: a tunnel's reachability is account-wide while its existence is environment-bound, so the durable resource is hostage to its most disposable dependency.

The cost is concrete, not theoretical. Decommissioning an environment today destroys the tunnels that environment created — so the most ordinary lifecycle event, retiring a machine, silently deletes a service teammates depend on. No amount of "let another environment serve it" fixes that while the environment still *owns* the tunnel; the tunnel has to belong to the account.

## The shift: the account owns the tunnel

The change in model is one sentence: a tunnel belongs to its account, and an environment is recorded only as *who is hosting it now*.

- **Ownership** moves from the creating environment to the account. The tunnel is created under the account, identified by its organization-unique name; it is not pinned to, and does not die with, any environment.
- **The right to host** becomes an account-level grant — the same account that already grants the right to reach. Any of the account's environments may host the tunnel; no other account's may.
- **Hosting** is the only place an environment appears: the controller records which environment is currently serving, and that record moves when hosting moves. The environment is a *current fact about the tunnel*, not a property of it.
- **Retiring an environment** tears down that environment's hosting and connections — its participation — and leaves the account's tunnels standing.

Provisioning falls out cleanly: because the right to host is now account-level, creating a tunnel no longer needs a particular environment's identity, so the tunnel needs no environment at all to exist. The model gets smaller, not larger — this is a subtraction (a tunnel stops being owned by an environment), not new machinery.

## Moving and reclaiming the host

For a managed proxy tunnel, the controller still recognizes a single current host at a time — the host record is what makes "the host" a meaningful singular thing. Moving the host to another of the account's environments is ordinary: the new environment serves, the record moves. When the previous host died cleanly there's nothing in the way. When it died *un*cleanly and still appears to be hosting, `--takeover` evicts the stale claim so the new host can take the spot — including evicting what the dead host left behind on the fabric, so the takeover actually displaces it rather than just editing a record. Takeover refuses to displace a host that still looks healthy unless forced, so it's a recovery tool, not a routine way to steal a working service.

Two honesties ride along with this. First, the host record is something the controller *tracks*, not something it can prove — it knows a host is recorded, not that the process behind it is alive; making "exactly one live host" a real enforced invariant is the separate single-active concern. Second, a host evicted by takeover that is somehow still alive could try to re-establish itself; takeover means "take it now," not "hold it against a live rival forever" — permanent exclusion is, again, the single-active cycle.

## What stays the same

- **Layer 2 session tunnels** keep environment ownership. A session tunnel exists only for one environment's participation in one session; "move it to another environment" is meaningless, and it should be torn down with the session, as it is today. Only standalone tunnels — the durable, depended-upon kind — become account-owned.
- **Connecting** is unchanged; it was already account-scoped.
- **The trust boundary** is unchanged: a tunnel belongs to one account, and only that account's environments may host or (absent a grant) reach it. Widening ownership to the account does not widen it past the account.

## Scope and migration

Agora runs only in the lab today, so this is the cheap moment to correct the ownership model. New tunnels are account-owned from the start. Tunnels that already exist under the old environment-owned model are recreated rather than migrated in place; a one-time conversion isn't worth building for a system with no production tenants. (If that changes, an in-place conversion is straightforward to add later.)

## In practice, normatively

- A standalone tunnel is owned by its **account**, identified by its organization-unique name, and does not require or belong to any environment.
- Any environment of the owning account may host the tunnel; no other account's environments may. Hosting is recorded as a current fact (which environment is serving now) that moves with the host.
- **Retiring or disabling an environment** does not delete the account's standalone tunnels; it tears down that environment's hosting and connections only.
- For a managed proxy tunnel, the controller recognizes one current host at a time. `--takeover` lets an eligible environment reclaim a stale host's spot, evicting what the dead host left on the fabric; it refuses a host that still looks healthy unless forced.
- A single active host is *assumed*, not *enforced*; enforcing it is deferred to [single-active-listen](./single-active-listen.md).
- **Session (Layer 2) tunnels** remain environment-owned and session-scoped.
- The account trust boundary and revocation behavior are unchanged.

## Deferred (and Why)

- **Single-active-host enforcement.** Guaranteeing exactly one live host at a time — so two healthy environments can't both serve and split traffic — is its own concern with its own fabric findings; see [single-active-listen](./single-active-listen.md). The fabric can't refuse a second host, so real enforcement is controller-directed and reaches into the thin-listener design. `--takeover` is the manual, on-demand slice of it; continuous automatic enforcement (and the automatic failover that comes with it) is the separate cycle.
- **In-place migration of existing tunnels.** Converting old environment-owned tunnels to account-owned without recreation is deferred; the lab has nothing that warrants it yet.
- **Org-level ownership or serving.** Widening past the account to the whole organization is a different trust model and is out of scope.

## Non-Goals

- Do not keep standalone tunnels owned by an environment, and do not let retiring an environment delete them.
- Do not widen ownership or hosting past the owning account.
- Do not enforce single-active-host here, and do not introduce multi-host / load-balanced serving; both belong to [single-active-listen](./single-active-listen.md).
- Do not add automatic failover or control-plane liveness adjudication in this work.
- Do not change Layer 2 session-tunnel ownership.
- Do not block this work on building an in-place migration.
