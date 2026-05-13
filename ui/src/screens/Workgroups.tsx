import { useCallback, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import {
  AlertTriangle,
  Boxes,
  Clipboard,
  RefreshCcw,
  ShieldCheck,
  Users,
  Wifi,
  X,
} from 'lucide-react';

import {
  AppShell,
  DataTable,
  EmptyState,
  SectionPanel,
  StatusPill,
  type DataTableColumn,
  type StatusPillStatus,
} from '../components';
import {
  ApiError,
  fetchAllVisibleAdvertisements,
  getDashboardSummary,
  getWorkgroupsActivity,
  listWorkgroupMembers,
  listWorkgroups,
  useApiResource,
  type Advertisement,
  type Workgroup,
  type WorkgroupMembership,
  type WorkgroupMembershipRole,
} from '../lib/api';

type MemberWithWorkgroups = {
  accountId: string;
  organizationId: string;
  email?: string;
  label: string;
  initials: string;
  avatarClassName: string;
  role: WorkgroupMembershipRole;
  workgroupIds: string[];
  workgroupNames: string[];
};

type WorkgroupCardModel = {
  workgroup: Workgroup;
  memberCount: number;
  callerRole?: WorkgroupMembershipRole;
  envelopes24h: number;
  advertisements: Advertisement[];
};

const routeByTab: Record<string, string> = {
  dashboard: '/',
  sessions: '/sessions',
  workgroups: '/workgroups',
  catalog: '/catalog',
  contracts: '/contracts',
  audit: '/audit',
};

const avatarClassNames = [
  'bg-brand-agora/10 text-brand-agora',
  'bg-brand-llm/10 text-brand-llm',
  'bg-brand-mcp/10 text-brand-mcp',
  'bg-info/10 text-info',
  'bg-success/10 text-success-strong',
];
const numberFormatter = new Intl.NumberFormat();

export default function Workgroups() {
  const navigate = useNavigate();
  const [selectedWorkgroupId, setSelectedWorkgroupId] = useState<string>();
  const account = useApiResource(getDashboardSummary);
  const workgroups = useApiResource(listWorkgroups);
  const workgroupsActivityLoad = useCallback(
    (signal: AbortSignal) => getWorkgroupsActivity({ window: '24h' }, signal),
    [],
  );
  const workgroupsActivity = useApiResource(workgroupsActivityLoad);
  const membersLoad = useCallback(
    (signal: AbortSignal) =>
      Promise.all((workgroups.data ?? []).map((workgroup) => listWorkgroupMembers(workgroup.id, signal))).then((groups) =>
        groups.flat(),
      ),
    [workgroups.data],
  );
  const members = useApiResource(membersLoad);
  const advertisements = useApiResource(fetchAllVisibleAdvertisements);
  const callerAccount = account.data?.account;
  const hasError = Boolean(account.error || workgroups.error || workgroupsActivity.error || members.error || advertisements.error);
  const isLoading =
    account.loading || workgroups.loading || workgroupsActivity.loading || members.loading || advertisements.loading;

  const workgroupNameById = useMemo(
    () => new Map((workgroups.data ?? []).map((workgroup) => [workgroup.id, workgroup.name])),
    [workgroups.data],
  );
  const membersByWorkgroup = useMemo(() => groupMembersByWorkgroup(members.data ?? []), [members.data]);
  const activityByWorkgroup = useMemo(
    () =>
      new Map(
        (workgroupsActivity.data?.byWorkgroup ?? []).map((row) => [row.workgroupId, row.envelopes]),
      ),
    [workgroupsActivity.data],
  );
  const adsByWorkgroup = useMemo(
    () => groupAdvertisementsByWorkgroup(advertisements.data ?? []),
    [advertisements.data],
  );
  const cardModels = useMemo(
    () =>
      (workgroups.data ?? []).map((workgroup) => {
        const workgroupMembers = membersByWorkgroup.get(workgroup.id) ?? [];
        const callerMembership = workgroupMembers.find(
          (member) => member.accountId === callerAccount?.accountId && member.workgroupId === workgroup.id,
        );

        return {
          workgroup,
          memberCount: workgroupMembers.length,
          callerRole: callerMembership?.role,
          envelopes24h: activityByWorkgroup.get(workgroup.id) ?? 0,
          advertisements: adsByWorkgroup.get(workgroup.id) ?? [],
        };
      }),
    [activityByWorkgroup, adsByWorkgroup, callerAccount?.accountId, membersByWorkgroup, workgroups.data],
  );
  const memberRows = useMemo(
    () => buildMemberRows(members.data ?? [], workgroupNameById),
    [members.data, workgroupNameById],
  );
  const selectedWorkgroup = cardModels.find((card) => card.workgroup.id === selectedWorkgroupId);

  function handleTabChange(tabId: string) {
    const route = routeByTab[tabId];

    if (route) {
      navigate(route);
    }
  }

  return (
    <AppShell
      product="agora"
      organizationName={callerAccount?.organizationName ?? 'Loading organization'}
      activeTab="workgroups"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'Connected'}
      userInitials={callerAccount ? initialsFromEmail(callerAccount.email) : '--'}
      userLabel={callerAccount?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        {account.error ? <ErrorPanel title="Current account unavailable" error={account.error} onRetry={account.refetch} /> : null}

        <StructuralSecurityPanel
          memberCount={memberRows.length}
          workgroupCount={cardModels.length}
          loading={!callerAccount || isLoading}
        />

        {workgroups.error || workgroupsActivity.error || members.error || advertisements.error ? (
          <ErrorPanel
            title="Workgroup data unavailable"
            error={workgroups.error ?? workgroupsActivity.error ?? members.error ?? advertisements.error}
            onRetry={() => {
              workgroups.refetch();
              workgroupsActivity.refetch();
              members.refetch();
              advertisements.refetch();
            }}
          />
        ) : null}

        {!callerAccount || workgroups.loading || members.loading || workgroupsActivity.loading || advertisements.loading ? (
          <LoadingPanel title="Loading workgroups" />
        ) : cardModels.length > 0 ? (
          <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
            {cardModels.map((card) => (
              <WorkgroupCard key={card.workgroup.id} card={card} onSelect={() => setSelectedWorkgroupId(card.workgroup.id)} />
            ))}
          </div>
        ) : (
          <EmptyState
            icon={Boxes}
            title="No workgroups"
            description="No workgroups are visible to the current account."
          />
        )}

        <SectionPanel title="Members" bodyClassName="p-0">
          {!callerAccount || members.loading || workgroups.loading ? (
            <div className="p-5">
              <LoadingPanel title="Loading members" compact />
            </div>
          ) : members.error ? (
            <div className="p-5">
              <ErrorPanel title="Members unavailable" error={members.error} onRetry={members.refetch} compact />
            </div>
          ) : (
            <DataTable
              columns={memberColumns}
              rows={memberRows}
              getRowKey={(row) => `${row.accountId}:${row.organizationId}`}
              className="rounded-none border-0"
              emptyState={
                <div className="p-5">
                  <EmptyState
                    icon={Users}
                    title="No members"
                    description="No memberships are visible for these workgroups."
                  />
                </div>
              }
            />
          )}
        </SectionPanel>
      </div>

      {selectedWorkgroup ? (
        <WorkgroupDrawer card={selectedWorkgroup} onClose={() => setSelectedWorkgroupId(undefined)} />
      ) : null}
    </AppShell>
  );
}

function StructuralSecurityPanel({
  memberCount,
  workgroupCount,
  loading,
}: {
  memberCount: number;
  workgroupCount: number;
  loading: boolean;
}) {
  return (
    <section className="rounded-card border border-border bg-panel p-5">
      <div className="grid gap-5 lg:grid-cols-[1fr_auto]">
        <div className="min-w-0">
          <div className="flex items-center gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-pill bg-brand-agora/10 text-brand-agora">
              <ShieldCheck size={20} aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <p className="text-label font-medium uppercase text-text-mute">Structural Security</p>
              <h1 className="mt-1 text-2xl font-semibold text-text">Workgroup Access Graph</h1>
            </div>
          </div>
          <p className="mt-4 max-w-3xl text-body text-text-mute">
            Workgroups define the governed collaboration boundary across organizations, accounts, and advertised services.
          </p>
        </div>

        <div className="grid min-w-72 gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <SecurityMetric label="members" value={loading ? '-' : formatInteger(memberCount)} />
          <SecurityMetric label="workgroups" value={loading ? '-' : formatInteger(workgroupCount)} />
          <div className="flex flex-col justify-between rounded-card border border-border bg-panel-subtle p-3">
            <p className="text-label font-medium uppercase text-text-mute">identity-bound</p>
            <StatusPill status="success" label="100%" className="mt-2" />
          </div>
          <div className="flex flex-col justify-between rounded-card border border-border bg-panel-subtle p-3">
            <p className="text-label font-medium uppercase text-text-mute">posture</p>
            <StatusPill status="success" label="Zero-Trust Active" className="mt-2" />
          </div>
        </div>
      </div>
    </section>
  );
}

function SecurityMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-card border border-border bg-panel-subtle p-3">
      <p className="text-label font-medium uppercase text-text-mute">{label}</p>
      <p className="mt-1 text-section font-semibold text-text">{value}</p>
    </div>
  );
}

function WorkgroupCard({ card, onSelect }: { card: WorkgroupCardModel; onSelect: () => void }) {
  return (
    <button
      type="button"
      className="flex min-h-64 flex-col justify-between rounded-card border border-border bg-panel p-5 text-left hover:border-border-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
      onClick={onSelect}
    >
      <div>
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <p className="text-label font-medium uppercase text-text-mute">Workgroup</p>
            <h2 className="mt-1 truncate text-section font-semibold text-text">{card.workgroup.name}</h2>
          </div>
          <StatusPill status={scopeStatus(card.workgroup.scope)} label={card.workgroup.scope} />
        </div>

        <div className="mt-5 grid gap-3 sm:grid-cols-3">
          <CardMetric label="members" value={formatInteger(card.memberCount)} />
          <CardMetric label="my role" value={card.callerRole ?? '-'} />
          <CardMetric label="24h envelopes" value={formatInteger(card.envelopes24h)} />
        </div>
      </div>

      {card.advertisements.length > 0 ? (
        <div className="mt-5 flex flex-wrap gap-2">
          {card.advertisements.map((advertisement) => (
            <AdvertisementPill key={advertisement.id} advertisement={advertisement} />
          ))}
        </div>
      ) : null}
    </button>
  );
}

function CardMetric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-label font-medium uppercase text-text-mute">{label}</p>
      <p className="mt-1 truncate text-body font-semibold text-text">{value}</p>
    </div>
  );
}

function AdvertisementPill({ advertisement }: { advertisement: Advertisement }) {
  const accent = advertisementAccent(advertisement);

  return (
    <span
      className={[
        'inline-flex max-w-full items-center rounded-status border px-3 py-1 text-table font-medium',
        accent,
      ].join(' ')}
    >
      <span className="truncate">{advertisement.name}</span>
    </span>
  );
}

const memberColumns: DataTableColumn<MemberWithWorkgroups>[] = [
  {
    id: 'member',
    header: 'Member',
    accessor: (row) => <MemberIdentity member={row} />,
    sortable: true,
    sortValue: (row) => row.label,
  },
  {
    id: 'role',
    header: 'Role',
    accessor: (row) => ({ status: roleStatus(row.role), label: row.role }),
    kind: 'pill',
    sortable: true,
  },
  {
    id: 'workgroups',
    header: 'Workgroups',
    accessor: (row) => row.workgroupNames.join(', '),
    sortable: true,
  },
];

function MemberIdentity({ member }: { member: MemberWithWorkgroups }) {
  return (
    <div className="flex min-w-64 items-center gap-3">
      <div
        className={[
          'flex size-8 shrink-0 items-center justify-center rounded-status text-table font-semibold',
          member.avatarClassName,
        ].join(' ')}
      >
        {member.initials}
      </div>
      <div className="min-w-0">
        <p className="truncate text-table font-medium text-text">{member.label}</p>
        <p className="truncate text-table text-text-mute">{member.organizationId}</p>
      </div>
    </div>
  );
}

function WorkgroupDrawer({ card, onClose }: { card: WorkgroupCardModel; onClose: () => void }) {
  const command = `agora workgroup describe ${card.workgroup.id}`;

  function copyCommand() {
    void navigator.clipboard?.writeText(command);
  }

  return (
    <div className="fixed inset-0 z-40 flex justify-end bg-text/20" role="dialog" aria-modal="true" aria-labelledby="workgroup-drawer-title">
      <button type="button" className="absolute inset-0 cursor-default" aria-label="Close workgroup detail" onClick={onClose} />
      <aside className="relative flex h-full w-full max-w-xl flex-col border-l border-border bg-page shadow-xl">
        <header className="flex min-h-16 items-center justify-between gap-4 border-b border-border bg-panel px-6">
          <div className="min-w-0">
            <p className="text-label font-medium uppercase text-text-mute">Workgroup detail</p>
            <h2 id="workgroup-drawer-title" className="truncate text-section font-semibold text-text">
              {card.workgroup.name}
            </h2>
          </div>
          <button
            type="button"
            className="inline-flex size-9 shrink-0 items-center justify-center rounded-pill border border-border bg-panel text-text-mute-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            aria-label="Close workgroup detail"
            onClick={onClose}
          >
            <X size={18} aria-hidden="true" />
          </button>
        </header>

        <div className="flex flex-1 flex-col gap-4 overflow-y-auto p-6">
          <SectionPanel title="Resource">
            <dl className="grid gap-3 sm:grid-cols-2">
              <DrawerField label="id" value={<span className="font-mono">{card.workgroup.id}</span>} />
              <DrawerField label="scope" value={<StatusPill status={scopeStatus(card.workgroup.scope)} label={card.workgroup.scope} />} />
              <DrawerField label="state" value={card.workgroup.state} />
              <DrawerField label="members" value={formatInteger(card.memberCount)} />
              <DrawerField label="my role" value={card.callerRole ?? '-'} />
              <DrawerField label="24h envelopes" value={formatInteger(card.envelopes24h)} />
            </dl>
          </SectionPanel>

          <SectionPanel title="CLI Handoff">
            <div className="flex flex-wrap items-center gap-3 rounded-card border border-border bg-panel-subtle p-3">
              <code className="min-w-0 flex-1 break-all font-mono text-table text-text">{command}</code>
              <button
                type="button"
                className="inline-flex h-9 shrink-0 items-center gap-2 rounded-pill border border-border bg-panel px-3 text-table font-medium text-text-mute-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
                onClick={copyCommand}
              >
                <Clipboard size={14} aria-hidden="true" />
                Copy
              </button>
            </div>
          </SectionPanel>
        </div>
      </aside>
    </div>
  );
}

function DrawerField({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="rounded-card border border-border bg-panel-subtle p-3">
      <dt className="text-label font-medium uppercase text-text-mute">{label}</dt>
      <dd className="mt-1 break-words text-body font-medium text-text">{value}</dd>
    </div>
  );
}

function groupMembersByWorkgroup(members: WorkgroupMembership[]): Map<string, WorkgroupMembership[]> {
  const grouped = new Map<string, WorkgroupMembership[]>();

  members.forEach((member) => {
    const group = grouped.get(member.workgroupId) ?? [];

    group.push(member);
    grouped.set(member.workgroupId, group);
  });

  return grouped;
}

function groupAdvertisementsByWorkgroup(advertisements: Advertisement[]): Map<string, Advertisement[]> {
  const grouped = new Map<string, Advertisement[]>();

  advertisements.forEach((advertisement) => {
    advertisement.workgroupScopes.forEach((workgroupId) => {
      const group = grouped.get(workgroupId) ?? [];

      group.push(advertisement);
      grouped.set(workgroupId, group);
    });
  });

  return grouped;
}

function buildMemberRows(
  memberships: WorkgroupMembership[],
  workgroupNameById: Map<string, string>,
): MemberWithWorkgroups[] {
  const rows = new Map<string, MemberWithWorkgroups>();

  memberships.forEach((membership) => {
    const key = `${membership.accountId}:${membership.organizationId}`;
    const existing = rows.get(key);
    const workgroupName = workgroupNameById.get(membership.workgroupId) ?? membership.workgroupId;

    if (existing) {
      existing.workgroupIds.push(membership.workgroupId);
      existing.workgroupNames.push(workgroupName);
      if (membership.role === 'admin') {
        existing.role = 'admin';
      }
      return;
    }

    rows.set(key, {
      accountId: membership.accountId,
      organizationId: membership.organizationId,
      email: membership.email,
      label: membership.email ?? `${membership.accountId} · ${membership.organizationId}`,
      initials: membership.email ? initialsFromEmail(membership.email) : initialsFromAccountId(membership.accountId),
      avatarClassName: avatarClassNames[hashString(membership.email ?? membership.accountId) % avatarClassNames.length],
      role: membership.role,
      workgroupIds: [membership.workgroupId],
      workgroupNames: [workgroupName],
    });
  });

  return Array.from(rows.values());
}

function scopeStatus(scope: Workgroup['scope']): StatusPillStatus {
  return scope === 'inter-org' ? 'info' : 'success';
}

function roleStatus(role: WorkgroupMembershipRole): StatusPillStatus {
  return role === 'admin' ? 'warning' : 'neutral';
}

function advertisementAccent(advertisement: Advertisement): string {
  const capabilityNames = advertisement.capabilities.map((capability) => capability.name);

  if (capabilityNames.includes('llm-routing') || advertisement.name.includes('llm-gateway')) {
    return 'border-brand-llm/30 bg-brand-llm/10 text-brand-llm';
  }

  if (capabilityNames.includes('mcp-tools') || advertisement.name.includes('mcp-gateway')) {
    return 'border-brand-mcp/30 bg-brand-mcp/10 text-brand-mcp';
  }

  return 'border-brand-agora/30 bg-brand-agora/10 text-brand-agora';
}

function LoadingPanel({ title, compact = false }: { title: string; compact?: boolean }) {
  return (
    <div
      className={[
        'flex items-center gap-3 rounded-card border border-border bg-panel-subtle px-4 text-body text-text-mute-strong',
        compact ? 'min-h-28 py-4' : 'min-h-32 py-5',
      ].join(' ')}
    >
      <Wifi size={18} aria-hidden="true" className="text-text-mute" />
      <span>{title}</span>
    </div>
  );
}

function ErrorPanel({
  title,
  error,
  onRetry,
  compact = false,
}: {
  title: string;
  error: unknown;
  onRetry: () => void;
  compact?: boolean;
}) {
  const detail = errorDetail(error);

  return (
    <div
      className={[
        'rounded-card border border-danger/30 bg-panel-subtle p-4',
        compact ? 'min-h-28' : 'min-h-32',
      ].join(' ')}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-pill bg-danger/10 text-danger">
            <AlertTriangle size={18} aria-hidden="true" />
          </div>
          <div className="min-w-0">
            <p className="text-body font-semibold text-text">{title}</p>
            <p className="mt-1 break-words text-table text-text-mute">{detail}</p>
          </div>
        </div>
        <button
          type="button"
          className="inline-flex h-9 shrink-0 items-center gap-2 rounded-pill border border-border bg-panel px-3 text-table font-medium text-text-mute-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
          onClick={onRetry}
        >
          <RefreshCcw size={14} aria-hidden="true" />
          Retry
        </button>
      </div>
    </div>
  );
}

function errorDetail(error: unknown): string {
  if (error instanceof ApiError) {
    const status = error.status ? `${error.status} ` : '';
    const code = error.code ? `${error.code}: ` : '';

    return `${status}${code}${error.message}`;
  }

  if (error instanceof Error) {
    return error.message;
  }

  return 'request failed';
}

function initialsFromEmail(email: string): string {
  const localPart = email.split('@')[0] ?? email;
  const initials = localPart
    .split('.')
    .filter(Boolean)
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();

  return initials || localPart.slice(0, 1).toUpperCase() || 'AG';
}

function initialsFromAccountId(accountId: string): string {
  return accountId.replace(/^ac_/, '').slice(0, 2).toUpperCase() || 'AC';
}

function hashString(value: string): number {
  return Array.from(value).reduce((hash, char) => (hash * 31 + char.charCodeAt(0)) >>> 0, 0);
}

function formatInteger(value: number): string {
  return numberFormatter.format(value);
}
