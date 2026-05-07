import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import {
  Activity,
  AlertTriangle,
  Clock3,
  FileWarning,
  RefreshCcw,
  Search,
  Timer,
  Users,
} from 'lucide-react';

import {
  AppShell,
  DataTable,
  EmptyState,
  SectionPanel,
  StatCard,
  type DataTableColumn,
  type StatusPillStatus,
} from '../components';
import {
  ApiError,
  getDashboardSummary,
  listSessions,
  useApiResource,
  type Session,
  type SessionState,
} from '../lib/api';

import { SessionDrawer } from './SessionDrawer';

type SessionRoleFilter = 'both' | 'provider' | 'consumer';
type SessionStateFilter = 'all' | SessionState;
type DerivedRole = 'provider' | 'consumer' | 'unknown';

type Counterparty = {
  accountId: string;
  organizationId: string;
  organizationName: string;
  email?: string;
  label: string;
  initials: string;
  avatarClassName: string;
};

type SessionRow = {
  session: Session;
  role: DerivedRole;
  counterparty: Counterparty;
  duration: string;
};

const activeStates: SessionState[] = ['proposed', 'accepting', 'active', 'closing'];
const stateFilters: SessionStateFilter[] = ['all', 'proposed', 'accepting', 'active', 'closing', 'closed'];
const roleFilters: SessionRoleFilter[] = ['both', 'provider', 'consumer'];
const routeByTab: Record<string, string> = {
  dashboard: '/',
  sessions: '/sessions',
  workgroups: '/workgroups',
  catalog: '/catalog',
};

const numberFormatter = new Intl.NumberFormat();
const avatarClassNames = [
  'bg-brand-agora/10 text-brand-agora',
  'bg-brand-llm/10 text-brand-llm',
  'bg-brand-mcp/10 text-brand-mcp',
  'bg-info/10 text-info',
  'bg-success/10 text-success-strong',
];

export default function Sessions() {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');
  const [stateFilter, setStateFilter] = useState<SessionStateFilter>('all');
  const [roleFilter, setRoleFilter] = useState<SessionRoleFilter>('both');
  const [selectedSessionId, setSelectedSessionId] = useState<string>();
  const now = useNow();
  const account = useApiResource(getDashboardSummary);
  const activeLoad = useCallback(
    (signal: AbortSignal) => listSessions({ states: activeStates, role: 'both' }, signal),
    [],
  );
  const recentLoad = useCallback(
    (signal: AbortSignal) => listSessions({ states: ['closed'], role: 'both', sort: 'closedAtDesc', limit: 50 }, signal),
    [],
  );
  const active = useApiResource(activeLoad);
  const recent = useApiResource(recentLoad);
  const callerAccount = account.data?.account;
  const hasError = Boolean(account.error || active.error || recent.error);
  const isLoading = account.loading || active.loading || recent.loading;

  const activeRows = useMemo(
    () => toRows(active.data ?? [], callerAccount?.accountId, now),
    [active.data, callerAccount?.accountId, now],
  );
  const recentRows = useMemo(
    () => toRows(recent.data ?? [], callerAccount?.accountId, now),
    [recent.data, callerAccount?.accountId, now],
  );
  const visibleActiveRows = useMemo(
    () => filterRows(activeRows, search, stateFilter, roleFilter),
    [activeRows, search, stateFilter, roleFilter],
  );
  const visibleRecentRows = useMemo(
    () => filterRows(recentRows, search, stateFilter, roleFilter),
    [recentRows, search, stateFilter, roleFilter],
  );
  const stats = useMemo(
    () => buildStats(activeRows, recentRows, now),
    [activeRows, recentRows, now],
  );
  const selectedRow = useMemo(
    () => [...activeRows, ...recentRows].find((row) => row.session.id === selectedSessionId),
    [activeRows, recentRows, selectedSessionId],
  );

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
      activeTab="sessions"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'All systems operational'}
      userInitials={callerAccount ? initialsFor(callerAccount.email) : '--'}
      userLabel={callerAccount?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        {account.error ? (
          <ErrorPanel title="Current account unavailable" error={account.error} onRetry={account.refetch} />
        ) : null}

        <StatsRow stats={stats} loading={!callerAccount || isLoading} />

        <Filters
          search={search}
          stateFilter={stateFilter}
          roleFilter={roleFilter}
          onSearchChange={setSearch}
          onStateFilterChange={setStateFilter}
          onRoleFilterChange={setRoleFilter}
        />

        <SectionPanel title="Active Sessions" bodyClassName="p-0">
          {!callerAccount || (active.loading && !active.data) ? (
            <div className="p-5">
              <LoadingPanel title="Loading active sessions" compact />
            </div>
          ) : active.error ? (
            <div className="p-5">
              <ErrorPanel title="Active sessions unavailable" error={active.error} onRetry={active.refetch} compact />
            </div>
          ) : (
            <DataTable
              columns={sessionColumns(false)}
              rows={visibleActiveRows}
              getRowKey={(row) => row.session.id}
              onRowClick={(row) => setSelectedSessionId(row.session.id)}
              className="rounded-none border-0"
              emptyState={
                <div className="p-5">
                  <EmptyState
                    icon={Activity}
                    title="No active sessions"
                    description="No in-flight sessions match the current filters."
                  />
                </div>
              }
            />
          )}
        </SectionPanel>

        <SectionPanel title="Recent Sessions" bodyClassName="p-0">
          {!callerAccount || (recent.loading && !recent.data) ? (
            <div className="p-5">
              <LoadingPanel title="Loading recent sessions" compact />
            </div>
          ) : recent.error ? (
            <div className="p-5">
              <ErrorPanel title="Recent sessions unavailable" error={recent.error} onRetry={recent.refetch} compact />
            </div>
          ) : (
            <DataTable
              columns={sessionColumns(true, false)}
              rows={visibleRecentRows}
              getRowKey={(row) => row.session.id}
              onRowClick={(row) => setSelectedSessionId(row.session.id)}
              className="rounded-none border-0"
              emptyState={
                <div className="p-5">
                  <EmptyState
                    icon={Clock3}
                    title="No recent sessions"
                    description="No closed sessions match the current filters."
                  />
                </div>
              }
            />
          )}
        </SectionPanel>
      </div>

      {selectedRow ? (
        <SessionDrawer
          session={selectedRow.session}
          role={selectedRow.role}
          counterpartyLabel={selectedRow.counterparty.label}
          counterpartyOrganizationName={selectedRow.counterparty.organizationName}
          onClose={() => setSelectedSessionId(undefined)}
        />
      ) : null}
    </AppShell>
  );
}

function StatsRow({ stats, loading }: { stats: SessionStats; loading: boolean }) {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <StatCard
        label="Active Sessions"
        value={loading ? '-' : formatInteger(stats.activeSessions)}
        icon={Activity}
        accent="agora"
      />
      <StatCard
        label="Sessions Today"
        value={loading ? '-' : formatInteger(stats.sessionsToday)}
        icon={Users}
        accent="info"
      />
      <StatCard
        label="Avg Session Duration"
        value={loading ? '-' : stats.averageRecentDuration}
        icon={Timer}
        accent="success"
      />
      <StatCard
        label="Contract Violations"
        value={loading ? '-' : formatInteger(stats.contractViolations)}
        icon={FileWarning}
        accent="warning"
      />
    </div>
  );
}

function Filters({
  search,
  stateFilter,
  roleFilter,
  onSearchChange,
  onStateFilterChange,
  onRoleFilterChange,
}: {
  search: string;
  stateFilter: SessionStateFilter;
  roleFilter: SessionRoleFilter;
  onSearchChange: (value: string) => void;
  onStateFilterChange: (value: SessionStateFilter) => void;
  onRoleFilterChange: (value: SessionRoleFilter) => void;
}) {
  return (
    <section className="rounded-card border border-border bg-panel p-4">
      <div className="grid gap-4 xl:grid-cols-[minmax(18rem,1fr)_auto]">
        <label className="relative block min-w-0">
          <span className="sr-only">Search sessions</span>
          <Search size={17} aria-hidden="true" className="absolute left-3 top-1/2 -translate-y-1/2 text-text-mute" />
          <input
            type="search"
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder="Search sessions, accounts, organizations, advertisements, or workgroups"
            className="h-10 w-full rounded-pill border border-border bg-panel-subtle pl-10 pr-3 text-body text-text outline-none placeholder:text-text-mute focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
          />
        </label>

        <div className="flex flex-wrap gap-3">
          <FilterGroup label="State">
            {stateFilters.map((state) => (
              <FilterButton
                key={state}
                active={stateFilter === state}
                label={state === 'all' ? 'all states' : state}
                onClick={() => onStateFilterChange(state)}
              />
            ))}
          </FilterGroup>
          <FilterGroup label="Role">
            {roleFilters.map((role) => (
              <FilterButton
                key={role}
                active={roleFilter === role}
                label={role === 'both' ? 'all roles' : role}
                onClick={() => onRoleFilterChange(role)}
              />
            ))}
          </FilterGroup>
        </div>
      </div>
    </section>
  );
}

function FilterGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-label font-medium uppercase text-text-mute">{label}</span>
      {children}
    </div>
  );
}

function FilterButton({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className={[
        'h-8 rounded-status border px-3 text-table font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
        active
          ? 'border-brand-agora bg-brand-agora/10 text-brand-agora'
          : 'border-border bg-panel-subtle text-text-mute-strong hover:bg-panel',
      ].join(' ')}
      aria-pressed={active}
      onClick={onClick}
    >
      {label}
    </button>
  );
}

function sessionColumns(includeClosedFields: boolean, sortable = true): DataTableColumn<SessionRow>[] {
  const columns: DataTableColumn<SessionRow>[] = [
    {
      id: 'id',
      header: 'Session',
      accessor: (row) => row.session.id,
      kind: 'mono',
      sortable,
    },
    {
      id: 'role',
      header: 'Role',
      accessor: (row) => ({ status: roleStatus(row.role), label: row.role }),
      kind: 'pill',
      sortable,
    },
    {
      id: 'counterparty',
      header: 'Counterparty',
      accessor: (row) => <CounterpartyCell counterparty={row.counterparty} />,
      sortable,
      sortValue: (row) => row.counterparty.label,
    },
    {
      id: 'organization',
      header: 'Organization',
      accessor: (row) => row.counterparty.organizationName,
      sortable,
    },
    {
      id: 'advertisement',
      header: 'Advertisement',
      accessor: (row) => row.session.advertisementName,
      sortable,
    },
    {
      id: 'workgroup',
      header: 'Workgroup',
      accessor: (row) => row.session.workgroupName,
      sortable,
    },
    {
      id: 'tunnelMode',
      header: 'Mode',
      accessor: (row) => ({ status: 'info', label: row.session.tunnelMode }),
      kind: 'pill',
      sortable,
    },
    {
      id: 'state',
      header: 'State',
      accessor: (row) => ({ status: stateStatus(row.session.state), label: row.session.state }),
      kind: 'pill',
      sortable,
    },
    {
      id: 'duration',
      header: 'Duration',
      accessor: (row) => row.duration,
      sortable,
      sortValue: (row) => durationSortValue(row.session),
      align: 'right',
    },
    {
      id: 'envelopes',
      header: 'Envelopes',
      accessor: (row) => row.session.envelopeCount ?? '',
      sortable,
      sortValue: (row) => row.session.envelopeCount ?? 0,
      align: 'right',
    },
  ];

  if (includeClosedFields) {
    columns.push(
      {
        id: 'closeReason',
        header: 'Close reason',
        accessor: (row) =>
          row.session.closeReason
            ? { status: closeReasonStatus(row.session.closeReason), label: row.session.closeReason }
            : '',
        kind: 'pill',
        sortable,
        sortValue: (row) => row.session.closeReason ?? '',
      },
      {
        id: 'closeDetail',
        header: 'Close detail',
        accessor: (row) => row.session.closeDetail ?? '',
      },
    );
  }

  return columns;
}

function CounterpartyCell({ counterparty }: { counterparty: Counterparty }) {
  return (
    <div className="flex min-w-56 items-center gap-3">
      <div
        className={[
          'flex size-8 shrink-0 items-center justify-center rounded-status text-table font-semibold',
          counterparty.avatarClassName,
        ].join(' ')}
      >
        {counterparty.initials}
      </div>
      <div className="min-w-0">
        <p className="truncate text-table font-medium text-text">{counterparty.label}</p>
        <p className="truncate text-table text-text-mute">{counterparty.organizationName}</p>
      </div>
    </div>
  );
}

type SessionStats = {
  activeSessions: number;
  sessionsToday: number;
  averageRecentDuration: string;
  contractViolations: number;
};

function buildStats(activeRows: SessionRow[], recentRows: SessionRow[], now: number): SessionStats {
  const since = now - 24 * 60 * 60 * 1000;
  const sessionsToday = [...activeRows, ...recentRows].filter((row) => {
    const proposedAt = Date.parse(row.session.proposedAt);

    return !Number.isNaN(proposedAt) && proposedAt >= since && proposedAt < now;
  }).length;
  const closedDurations = recentRows
    .map((row) => durationMilliseconds(row.session.proposedAt, row.session.closedAt))
    .filter((duration): duration is number => duration !== undefined);
  const averageDuration =
    closedDurations.length > 0
      ? closedDurations.reduce((total, duration) => total + duration, 0) / closedDurations.length
      : undefined;

  return {
    activeSessions: activeRows.length,
    sessionsToday,
    averageRecentDuration: averageDuration === undefined ? '-' : formatDuration(averageDuration),
    contractViolations: recentRows.filter((row) => row.session.closeReason === 'contract_violation').length,
  };
}

function toRows(sessions: Session[], callerAccountId: string | undefined, now: number): SessionRow[] {
  return sessions.map((session) => {
    const role = roleForSession(session, callerAccountId);
    const counterparty = counterpartyForSession(session, role);

    return {
      session,
      role,
      counterparty,
      duration: durationForSession(session, now),
    };
  });
}

function filterRows(
  rows: SessionRow[],
  search: string,
  stateFilter: SessionStateFilter,
  roleFilter: SessionRoleFilter,
): SessionRow[] {
  const normalizedSearch = search.trim().toLowerCase();

  return rows.filter((row) => {
    if (stateFilter !== 'all' && row.session.state !== stateFilter) {
      return false;
    }

    if (roleFilter !== 'both' && row.role !== roleFilter) {
      return false;
    }

    if (!normalizedSearch) {
      return true;
    }

    return searchableText(row).includes(normalizedSearch);
  });
}

function searchableText(row: SessionRow): string {
  const session = row.session;
  const providerAccount = session.providerAccountEmail ?? session.providerAccountId;
  const consumerAccount = session.consumerAccountEmail ?? session.consumerAccountId;

  return [
    session.id,
    providerAccount,
    consumerAccount,
    session.providerOrganizationName,
    session.consumerOrganizationName,
    session.advertisementName,
    session.workgroupName,
  ]
    .join(' ')
    .toLowerCase();
}

function roleForSession(session: Session, callerAccountId: string | undefined): DerivedRole {
  if (callerAccountId && session.providerAccountId === callerAccountId) {
    return 'provider';
  }

  if (callerAccountId && session.consumerAccountId === callerAccountId) {
    return 'consumer';
  }

  return 'unknown';
}

function counterpartyForSession(session: Session, role: DerivedRole): Counterparty {
  if (role === 'provider') {
    return buildCounterparty({
      accountId: session.consumerAccountId,
      organizationId: session.consumerOrganizationId,
      organizationName: session.consumerOrganizationName,
      email: session.consumerAccountEmail,
    });
  }

  return buildCounterparty({
    accountId: session.providerAccountId,
    organizationId: session.providerOrganizationId,
    organizationName: session.providerOrganizationName,
    email: session.providerAccountEmail,
  });
}

function buildCounterparty({
  accountId,
  organizationId,
  organizationName,
  email,
}: {
  accountId: string;
  organizationId: string;
  organizationName: string;
  email?: string;
}): Counterparty {
  const seed = email ?? accountId;

  return {
    accountId,
    organizationId,
    organizationName,
    email,
    label: email ?? `${accountId} · ${organizationId}`,
    initials: initialsFor(seed),
    avatarClassName: avatarClassNames[hashString(seed) % avatarClassNames.length],
  };
}

function durationForSession(session: Session, now: number): string {
  const end = session.closedAt ? Date.parse(session.closedAt) : now;
  const start = Date.parse(session.proposedAt);

  if (Number.isNaN(start) || Number.isNaN(end) || end < start) {
    return '-';
  }

  return formatDuration(end - start);
}

function durationSortValue(session: Session): number {
  return durationMilliseconds(session.proposedAt, session.closedAt) ?? 0;
}

function durationMilliseconds(startValue: string, endValue: string | undefined): number | undefined {
  const start = Date.parse(startValue);
  const end = endValue ? Date.parse(endValue) : Date.now();

  if (Number.isNaN(start) || Number.isNaN(end) || end < start) {
    return undefined;
  }

  return end - start;
}

function formatDuration(milliseconds: number): string {
  const totalSeconds = Math.max(Math.floor(milliseconds / 1000), 0);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }

  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }

  return `${seconds}s`;
}

function useNow(): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);

    return () => window.clearInterval(timer);
  }, []);

  return now;
}

function stateStatus(state: SessionState): StatusPillStatus {
  if (state === 'closed') {
    return 'neutral';
  }

  if (state === 'active') {
    return 'active';
  }

  return 'info';
}

function roleStatus(role: DerivedRole): StatusPillStatus {
  if (role === 'provider') {
    return 'info';
  }

  if (role === 'consumer') {
    return 'success';
  }

  return 'neutral';
}

function closeReasonStatus(reason: NonNullable<Session['closeReason']>): StatusPillStatus {
  if (reason === 'contract_violation' || reason === 'tunnel_failed') {
    return 'warning';
  }

  if (reason === 'admin_close' || reason === 'workgroup_deleted' || reason === 'environment_disabled') {
    return 'danger';
  }

  return 'neutral';
}

function LoadingPanel({ title, compact = false }: { title: string; compact?: boolean }) {
  return (
    <div
      className={[
        'flex items-center gap-3 rounded-card border border-border bg-panel-subtle px-4 text-body text-text-mute-strong',
        compact ? 'min-h-28 py-4' : 'min-h-32 py-5',
      ].join(' ')}
    >
      <Clock3 size={18} aria-hidden="true" className="text-text-mute" />
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

function initialsFor(value: string): string {
  const parts = value.split(/[^a-zA-Z0-9]+/).filter(Boolean);
  const initials = (parts.length > 1 ? parts : [value])
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();

  return initials || 'AG';
}

function hashString(value: string): number {
  return Array.from(value).reduce((hash, char) => (hash * 31 + char.charCodeAt(0)) >>> 0, 0);
}

function formatInteger(value: number): string {
  return numberFormatter.format(value);
}
