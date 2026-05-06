import { Activity, Boxes, FileText, GitBranch, Server, ShieldCheck, Users, Wifi } from 'lucide-react';

import {
  AppShell,
  BarChart,
  DataTable,
  EmptyState,
  KeyValueGrid,
  SectionPanel,
  SidebarBreakdown,
  StatCard,
  StatusPill,
  type DataTableColumn,
  type Product,
  type StatusPillStatus,
} from '../components';

type ColorToken = {
  name: string;
  className: string;
  hex: string;
  usage: string;
};

const colors: ColorToken[] = [
  { name: 'page', className: 'bg-page', hex: '#fafafa', usage: 'page background' },
  { name: 'panel', className: 'bg-panel', hex: '#ffffff', usage: 'card and panel surfaces' },
  { name: 'panel-subtle', className: 'bg-panel-subtle', hex: '#f4f4f5', usage: 'nested panels' },
  { name: 'border', className: 'bg-border', hex: '#e4e4e7', usage: 'card borders' },
  { name: 'border-strong', className: 'bg-border-strong', hex: '#d4d4d8', usage: 'separators' },
  { name: 'text-mute-2', className: 'bg-text-mute-2', hex: '#a1a1aa', usage: 'tertiary text' },
  { name: 'text-mute', className: 'bg-text-mute', hex: '#71717a', usage: 'secondary text' },
  { name: 'text-mute-strong', className: 'bg-text-mute-strong', hex: '#52525b', usage: 'headers' },
  { name: 'text', className: 'bg-text', hex: '#18181b', usage: 'primary text' },
  { name: 'brand-llm', className: 'bg-brand-llm', hex: '#0891b2', usage: 'LLM Gateway' },
  { name: 'brand-llm-end', className: 'bg-brand-llm-end', hex: '#0284c7', usage: 'LLM gradient end' },
  { name: 'brand-mcp', className: 'bg-brand-mcp', hex: '#7c3aed', usage: 'MCP Gateway' },
  { name: 'brand-mcp-end', className: 'bg-brand-mcp-end', hex: '#a855f7', usage: 'MCP gradient end' },
  { name: 'brand-agora', className: 'bg-brand-agora', hex: '#4f46e5', usage: 'Agora' },
  { name: 'brand-agora-end', className: 'bg-brand-agora-end', hex: '#1d4ed8', usage: 'Agora gradient end' },
  { name: 'brand-family-start', className: 'bg-brand-family-start', hex: '#0891b2', usage: 'family gradient start' },
  { name: 'brand-family-end', className: 'bg-brand-family-end', hex: '#7c3aed', usage: 'family gradient end' },
  { name: 'success', className: 'bg-success', hex: '#22c55e', usage: 'healthy' },
  { name: 'success-strong', className: 'bg-success-strong', hex: '#16a34a', usage: 'active' },
  { name: 'warning', className: 'bg-warning', hex: '#d97706', usage: 'warning' },
  { name: 'warning-strong', className: 'bg-warning-strong', hex: '#f97316', usage: 'degraded' },
  { name: 'danger', className: 'bg-danger', hex: '#dc2626', usage: 'failed' },
  { name: 'info', className: 'bg-info', hex: '#2563eb', usage: 'info' },
  { name: 'highlight', className: 'bg-highlight', hex: '#eab308', usage: 'highlight' },
];

const typeRows = [
  {
    label: 'Stat',
    className: 'text-stat font-bold text-text',
    sample: '1,284',
    detail: '28-32px / weight 700',
  },
  {
    label: 'Section',
    className: 'text-section font-semibold text-text',
    sample: 'Active Sessions',
    detail: '18-20px / weight 600',
  },
  {
    label: 'Body',
    className: 'text-body text-text',
    sample: 'Governed collaboration activity across organizations.',
    detail: '14px / weight 400',
  },
  {
    label: 'Label',
    className: 'text-label font-medium uppercase text-text-mute',
    sample: 'close reason',
    detail: '11-12px / weight 500',
  },
  {
    label: 'Table',
    className: 'text-table text-text',
    sample: 'con_k9x2e8p31wqz',
    detail: '13px / weight 400',
  },
];

const shellProducts: Product[] = ['agora', 'llm', 'mcp'];

const activityData = [
  { label: '06', value: 42 },
  { label: '08', value: 68 },
  { label: '10', value: 124 },
  { label: '12', value: 96 },
  { label: '14', value: 148 },
  { label: '16', value: 132 },
  { label: '18', value: 174 },
  { label: '20', value: 118 },
];

const workgroupBreakdown = [
  { label: 'Enterprise Client', value: 1248, accent: 'agora' as const },
  { label: 'Macro Pulse Equities', value: 982, accent: 'llm' as const },
  { label: 'Gateway Services', value: 618, accent: 'mcp' as const },
  { label: 'Risk Review', value: 312, accent: 'info' as const },
];

type SessionRow = {
  id: string;
  workgroup: string;
  provider: string;
  state: string;
  status: StatusPillStatus;
  envelopes: number;
  updated: string;
};

const sessionRows: SessionRow[] = [
  {
    id: 'ses_9x2e8p31wqz',
    workgroup: 'Enterprise Client',
    provider: 'equity-feed',
    state: 'active',
    status: 'active',
    envelopes: 1482,
    updated: '18s ago',
  },
  {
    id: 'ses_j7p4k2m0cva',
    workgroup: 'Gateway Services',
    provider: 'llm-gateway',
    state: 'online',
    status: 'online',
    envelopes: 836,
    updated: '41s ago',
  },
  {
    id: 'ses_2m6rvp8d4na',
    workgroup: 'Risk Review',
    provider: 'fx-feed',
    state: 'stale',
    status: 'stale',
    envelopes: 224,
    updated: '3m ago',
  },
];

const sessionColumns: DataTableColumn<SessionRow>[] = [
  { id: 'id', header: 'Session', accessor: (row) => row.id, kind: 'mono', sortable: true },
  { id: 'workgroup', header: 'Workgroup', accessor: (row) => row.workgroup, sortable: true },
  { id: 'provider', header: 'Provider', accessor: (row) => row.provider },
  { id: 'state', header: 'State', accessor: (row) => ({ status: row.status, label: row.state }), kind: 'pill', sortable: true },
  { id: 'envelopes', header: 'Envelopes', accessor: (row) => row.envelopes, sortable: true, align: 'right' },
  { id: 'updated', header: 'Updated', accessor: (row) => row.updated, align: 'right' },
];

export default function Kit() {
  return (
    <main className="min-h-screen bg-page px-8 py-10 text-text">
      <section className="mx-auto flex max-w-6xl flex-col gap-8">
        <header>
          <p className="text-label font-medium uppercase text-text-mute">Agora UI kit</p>
          <h1 className="mt-2 text-2xl font-semibold text-text">Design Tokens</h1>
        </header>

        <section className="rounded-card border border-border bg-panel p-6">
          <h2 className="mb-4 text-section font-semibold text-text">Type</h2>
          <div className="divide-y divide-border">
            {typeRows.map((row) => (
              <div key={row.label} className="grid gap-3 py-4 md:grid-cols-[160px_1fr_220px]">
                <p className="text-label font-medium uppercase text-text-mute">{row.label}</p>
                <p className={row.className}>{row.sample}</p>
                <p className="text-table text-text-mute">{row.detail}</p>
              </div>
            ))}
          </div>
        </section>

        <section className="flex flex-col gap-4">
          <div>
            <h2 className="text-section font-semibold text-text">Chrome</h2>
            <p className="mt-1 text-body text-text-mute">AppShell product variants with shared structure.</p>
          </div>
          {shellProducts.map((product) => (
            <div key={product} className="overflow-hidden rounded-card border border-border bg-page">
              <AppShell
                product={product}
                organizationName="Agora Demo Corp"
                activeTab="dashboard"
                userInitials="AD"
                userLabel="demo@agora.local"
                fullHeight={false}
              >
                <div className="grid gap-4 md:grid-cols-[1fr_18rem]">
                  <section className="rounded-card border border-border bg-panel p-5">
                    <div className="mb-4 flex items-center gap-3">
                      <Activity size={20} aria-hidden="true" className="text-info" />
                      <h3 className="text-section font-semibold text-text">Governed Activity</h3>
                    </div>
                    <div className="grid gap-3 sm:grid-cols-3">
                      <StatCard label="events" value="248" accent={product} />
                      <StatCard label="sessions" value="18" accent={product} />
                      <StatCard label="workgroups" value="6" accent={product} />
                    </div>
                  </section>
                  <section className="rounded-card border border-border bg-panel p-5">
                    <div className="mb-4 flex items-center gap-3">
                      <Boxes size={20} aria-hidden="true" className="text-text-mute" />
                      <h3 className="text-section font-semibold text-text">Catalog</h3>
                    </div>
                    <div className="flex items-center gap-3 rounded-card border border-border bg-panel-subtle p-3">
                      <Server size={18} aria-hidden="true" className="shrink-0 text-text-mute" />
                      <p className="text-body text-text-mute">10 advertisements visible to this organization.</p>
                    </div>
                  </section>
                </div>
              </AppShell>
            </div>
          ))}
        </section>

        <section className="flex flex-col gap-4">
          <div>
            <h2 className="text-section font-semibold text-text">Surface Primitives</h2>
            <p className="mt-1 text-body text-text-mute">Reusable panels, charts, tables, and resource surfaces.</p>
          </div>

          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <StatCard
              label="Active Sessions"
              value="18"
              icon={Activity}
              accent="agora"
              delta={{ direction: 'up', value: '+12%', label: 'from 7d avg' }}
            />
            <StatCard
              label="Envelopes Today"
              value="4,288"
              icon={Wifi}
              accent="success"
              delta={{ direction: 'up', value: '+824', label: 'since yesterday' }}
            />
            <StatCard label="Active Workgroups" value="6" icon={Users} accent="info" />
            <StatCard
              label="Contract Flags"
              value="2"
              icon={ShieldCheck}
              accent="warning"
              delta={{ direction: 'flat', value: 'steady', label: 'last hour' }}
            />
          </div>

          <div className="grid gap-4 lg:grid-cols-[1fr_20rem]">
            <SectionPanel
              title="Envelope Flow"
              actions={
                <div className="flex rounded-pill border border-border bg-panel-subtle p-1">
                  <button type="button" className="rounded-pill bg-panel px-3 py-1 text-table font-medium text-text">
                    24h
                  </button>
                  <button type="button" className="rounded-pill px-3 py-1 text-table font-medium text-text-mute">
                    7d
                  </button>
                </div>
              }
            >
              <BarChart data={activityData} accent="agora" />
            </SectionPanel>
            <SectionPanel title="By Workgroup">
              <SidebarBreakdown items={workgroupBreakdown} />
            </SectionPanel>
          </div>

          <div className="grid gap-4 xl:grid-cols-[1fr_22rem]">
            <SectionPanel title="Live Sessions" bodyClassName="p-0">
              <DataTable
                columns={sessionColumns}
                rows={sessionRows}
                getRowKey={(row) => row.id}
                className="rounded-none border-0"
                actions={() => undefined}
              />
            </SectionPanel>
            <div className="flex flex-col gap-4">
              <SectionPanel title="Resource Detail">
                <KeyValueGrid
                  entries={[
                    { key: 'workgroup', value: 'Enterprise Client' },
                    { key: 'contract', value: <span className="font-mono">con_gateway_default</span> },
                    { key: 'mode', value: <StatusPill status="online" label="tunnel active" /> },
                    { key: 'owner', value: 'demo@agora.local' },
                  ]}
                />
              </SectionPanel>
              <SectionPanel>
                <EmptyState
                  icon={FileText}
                  title="No audit events match"
                  description="The selected filters do not include any governed collaboration activity."
                />
              </SectionPanel>
            </div>
          </div>

          <SectionPanel title="Headerless Panel">
            <div className="flex items-center gap-3">
              <GitBranch size={20} aria-hidden="true" className="text-brand-agora" />
              <p className="text-body text-text-mute">
                Headerless sections keep compact details framed without adding another title row.
              </p>
            </div>
          </SectionPanel>
        </section>

        <section className="rounded-card border border-border bg-panel p-6">
          <h2 className="mb-4 text-section font-semibold text-text">Colors</h2>
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {colors.map((color) => (
              <div key={color.name} className="flex items-center gap-3 rounded-card border border-border bg-panel p-3">
                <div className={`size-10 rounded-pill border border-border-strong ${color.className}`} />
                <div>
                  <p className="font-mono text-table text-text">{color.name}</p>
                  <p className="text-table text-text-mute">
                    {color.hex} - {color.usage}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </section>
      </section>
    </main>
  );
}
