# Agora Dashboard Walkthrough

This is the presentation path for a 7-9 minute Agora dashboard demo. The goal is to show that Agora is the governance layer above agent and gateway collaboration: authenticated, workgroup-scoped, contract-governed, and visibly alive.

## Before You Start

1. Start the demo:

   ```sh
   ./bin/demo-up.sh
   ```

2. Wait for the terminal to end with:

   ```text
   Agora demo is running at http://localhost:18080/
      Email:    demo@agora.local
      Password: Agora-Demo-1
   ```

3. Keep the terminal visible enough to read the credentials, but do not dwell on it. If the script warns that `llm-gateway` or `mcp-gateway` is missing, continue; the core Agora and Macro Pulse demo still works.

4. Use the browser window opened by the script. It should land on the login screen.

## Click Path

### 1. Login

Time: 30-45 seconds

Action:
- Enter `demo@agora.local`.
- Enter `Agora-Demo-1`.
- Submit the form.

Talk track:
- "The dashboard starts with a real login. This is not a static mockup and it is not a URL with a token stuffed into it."
- "After login, the browser is using session cookies. The dashboard is showing the view for this account and this organization."

What to point at:
- The clean login frame before the dashboard chrome appears.
- The organization pill and user badge after login.

### 2. Dashboard

Time: 2 minutes

Action:
- Start on the Dashboard tab.
- Point to the Current account panel.
- Point to the four stat cards.
- Switch the chart from "Last 24 hours" to "Last 7 days".
- Point to Top Workgroups and the environment status table.

Talk track:
- "This is the executive view: who am I, which organization am I operating in, and what is active right now."
- "The counters are scoped to the organization, not to a single process. Agora is aggregating activity across the governed collaboration surface."
- "The chart mixes live activity with pre-seeded history so the demo opens with a realistic baseline, then the Macro Pulse loop keeps adding real activity as we talk."
- "The workgroup breakdown shows where the activity is concentrated. That is the governance boundary becoming visible."
- "The environment table shows the runtime identities behind the activity. These are independent enrolled environments, not threads inside one monolithic demo process."

What to point at:
- `demo@agora.local` in the account panel.
- "Zero-Trust Active" badge.
- Active Sessions and Envelopes Today.
- Last 7 days bar rhythm.
- Markets and signals workgroups reading busier than the quieter workgroups.

### 3. Sessions

Time: 2 minutes

Action:
- Click Sessions.
- Show the Active Sessions panel.
- Show Recent Sessions.
- Point to session state and close-reason pills.
- If available, use the search box for a visible provider name such as `equity`, `news`, or `narrator`.

Talk track:
- "These are real Agora sessions opened by the Macro Pulse orchestrator against worker advertisements."
- "The demo login is also the orchestrator identity, so this account naturally sees the sessions it participates in."
- "Close reasons are part of the governance story. Normal closes and provider closes are expected; contract violations are also visible when the system enforces a bound."
- "This gives an operator a live answer to: what is running, who is involved, and how did it finish?"

What to point at:
- Session IDs and role.
- Counterparty organization.
- Advertisement and workgroup columns.
- `contract_violation`, `consumer_close`, or `provider_close` pills if present.

### 4. Workgroups

Time: 1.5 minutes

Action:
- Click Workgroups.
- Point to the Structural Security panel.
- Scan the workgroup cards.
- Point to inter-org badges and per-card 24h envelope totals.
- Scroll to the members table if needed.

Talk track:
- "Workgroups are the collaboration boundary. They decide which accounts can discover and use which services."
- "The demo account participates in several cross-organization channels: markets, weather, signals, and analytics."
- "The cards show both structure and activity: membership, role, visible advertisements, and recent envelope volume."
- "Cross-org identity is handled deliberately. Agora shows enough to explain the collaboration without leaking account details across boundaries."

What to point at:
- Inter-org badges.
- Admin/member role.
- Workgroup-scoped advertisements.
- Member rows and role pills.

### 5. Catalog

Time: 1.5 minutes

Action:
- Click Catalog.
- Point to the visibility overview panel.
- Use one filter if it helps the screen read cleanly.
- Point to Macro Pulse advertisement cards.
- If LLM Gateway or MCP Gateway cards are present, point to their gateway accents.

Talk track:
- "The catalog is what this account is allowed to see. It is not a global list of everything running."
- "Each card is a governed service advertisement: provider organization, tunnel mode, workgroup scope, and contract reference."
- "The gateway cards, when those binaries are installed, show the family-of-products story: LLM Gateway and MCP Gateway can publish into the same Agora-governed catalog."
- "Deep gateway routing is not what this demo claims. The honest MVP claim is visibility and governance through Agora."

What to point at:
- Advertisement count.
- Workgroup scope pills.
- Contract pills.
- LLM/MCP gateway cards if present.

### 6. Close

Time: 30-45 seconds

Talk track:
- "Agora makes agent collaboration visible and governable."
- "Authentication, workgroup membership, contracts, sessions, and live activity all show up in one place."
- "The important point is not that this is a dashboard. The point is that the primitives behind the dashboard are real Agora primitives."

Action:
- Leave the dashboard on whichever screen best matches the audience: Dashboard for executives, Sessions for operators, Catalog for the gateway story.

## After The Demo

Stop the managed demo processes:

```sh
./bin/demo-down.sh
```

This preserves demo enrollments and logs for the next run. Use a clean slate only when needed:

```sh
./bin/demo-down.sh --purge
```

## Recording Notes

Record from the login screen through the close, using the path above and a normal speaking cadence. Keep the video under 10 minutes. Save the approved recording as `docs/dashboard/walkthrough.mp4` if the team decides to commit the binary; otherwise store it externally and link it from this section.
