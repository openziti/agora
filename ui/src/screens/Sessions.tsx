import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import {
  Activity,
  AlertTriangle,
  ArrowLeftRight,
  ChevronDown,
  ChevronRight,
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
  DrawerCodeChip,
  DrawerDivider,
  DrawerStepList,
  DrawerTip,
  EmptyState,
  InfoDrawer,
  Input,
  PageHeader,
  Pagination,
  Select,
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
const routeByTab: Record<string, string> = {
  dashboard: '/',
  sessions: '/sessions',
  workgroups: '/workgroups',
  catalog: '/catalog',
  contracts: '/contracts',
  audit: '/audit',
};

const numberFormatter = new Intl.NumberFormat();
const SESSIONS_POLL_MS = 5000;
const closeReasonLabels: Record<string, string> = {
  consumer_close: 'Closed by consumer',
  provider_close: 'Closed by provider',
  tunnel_failed: 'Tunnel connection failed',
  rejected: 'Rejected',
  contract_violation: 'Contract violation',
  admin_close: 'Closed by administrator',
  workgroup_deleted: 'Workgroup deleted',
  environment_disabled: 'Environment disabled',
};

const avatarClassNames = [
  'bg-brand-agora/10 text-brand-agora',
  'bg-brand-llm/10 text-brand-llm',
  'bg-brand-mcp/10 text-brand-mcp',
  'bg-info/10 text-info',
  'bg-success/10 text-success-strong',
];

export default function Sessions() {
  const navigate = useNavigate();
  const [infoOpen, setInfoOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [stateFilter, setStateFilter] = useState<SessionStateFilter>('all');
  const [roleFilter, setRoleFilter] = useState<SessionRoleFilter>('both');
  const [serviceFilter, setServiceFilter] = useState('all');
  const [orgFilter, setOrgFilter] = useState('all');
  const [channelFilter, setChannelFilter] = useState('all');
  const [closeReasonFilter, setCloseReasonFilter] = useState('all');
  const [selectedSessionId, setSelectedSessionId] = useState<string>();
  const [activeCurrentPage, setActiveCurrentPage] = useState(1);
  const [activeItemsPerPage, setActiveItemsPerPage] = useState(10);
  const [recentCurrentPage, setRecentCurrentPage] = useState(1);
  const [recentItemsPerPage, setRecentItemsPerPage] = useState(10);
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
  const active = useApiResource(activeLoad, { intervalMs: SESSIONS_POLL_MS });
  const recent = useApiResource(recentLoad, { intervalMs: SESSIONS_POLL_MS });
  const callerAccount = account.data?.account;
  const hasError = Boolean(account.error || active.error || recent.error);
  const isLoading =
    (account.loading && !account.data) ||
    (active.loading && !active.data) ||
    (recent.loading && !recent.data);

  const activeRows = useMemo(
    () => toRows(active.data ?? [], callerAccount?.accountId, now),
    [active.data, callerAccount?.accountId, now],
  );
  const recentRows = useMemo(
    () => toRows(recent.data ?? [], callerAccount?.accountId, now),
    [recent.data, callerAccount?.accountId, now],
  );
  const allRows = useMemo(() => [...activeRows, ...recentRows], [activeRows, recentRows]);
  const serviceOptions = useMemo(
    () => uniqueSortedValues(allRows, (r) => r.session.advertisementName ?? '').filter(Boolean),
    [allRows],
  );
  const orgOptions = useMemo(
    () => uniqueSortedValues(allRows, (r) => r.counterparty.organizationName ?? '').filter(Boolean),
    [allRows],
  );
  const channelOptions = useMemo(
    () => uniqueSortedValues(allRows, (r) => r.session.workgroupName ?? '').filter(Boolean),
    [allRows],
  );
  const closeReasonOptions = useMemo(
    () => uniqueSortedValues(recentRows, (r) => r.session.closeReason ?? '').filter(Boolean),
    [recentRows],
  );
  const visibleActiveRows = useMemo(
    () => filterRows(activeRows, search, stateFilter, roleFilter, serviceFilter, orgFilter, channelFilter, closeReasonFilter),
    [activeRows, search, stateFilter, roleFilter, serviceFilter, orgFilter, channelFilter, closeReasonFilter],
  );
  const visibleRecentRows = useMemo(
    () => filterRows(recentRows, search, stateFilter, roleFilter, serviceFilter, orgFilter, channelFilter, closeReasonFilter),
    [recentRows, search, stateFilter, roleFilter, serviceFilter, orgFilter, channelFilter, closeReasonFilter],
  );
  // clamp the page during render so a shrinking list never strands us past the last page
  const activeTotalPages = Math.max(1, Math.ceil(visibleActiveRows.length / activeItemsPerPage));
  const safeActivePage = Math.min(activeCurrentPage, activeTotalPages);
  const recentTotalPages = Math.max(1, Math.ceil(visibleRecentRows.length / recentItemsPerPage));
  const safeRecentPage = Math.min(recentCurrentPage, recentTotalPages);
  const paginatedActiveRows = useMemo(() => {
    const start = (safeActivePage - 1) * activeItemsPerPage;
    return visibleActiveRows.slice(start, start + activeItemsPerPage);
  }, [visibleActiveRows, safeActivePage, activeItemsPerPage]);
  const paginatedRecentRows = useMemo(() => {
    const start = (safeRecentPage - 1) * recentItemsPerPage;
    return visibleRecentRows.slice(start, start + recentItemsPerPage);
  }, [visibleRecentRows, safeRecentPage, recentItemsPerPage]);
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

  const isFiltered =
    search !== '' ||
    stateFilter !== 'all' ||
    roleFilter !== 'both' ||
    serviceFilter !== 'all' ||
    orgFilter !== 'all' ||
    channelFilter !== 'all' ||
    closeReasonFilter !== 'all';

  function handleResetFilters() {
    setSearch('');
    setStateFilter('all');
    setRoleFilter('both');
    setServiceFilter('all');
    setOrgFilter('all');
    setChannelFilter('all');
    setCloseReasonFilter('all');
  }

  return (
    <AppShell
      product="agora"
      organizationName={callerAccount?.organizationName ?? 'Loading organization'}
      activeTab="sessions"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'Connected'}
      userInitials={callerAccount ? initialsFor(callerAccount.email) : '--'}
      userLabel={callerAccount?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        <PageHeader
          icon={ArrowLeftRight}
          label="COMMUNICATION"
          title="Sessions"
          description="Governed communication channels between agents, each backed by an explicit contract and retaining a full audit record — even after close."
          onInfoClick={() => setInfoOpen(true)}
        />
        {account.error ? (
          <ErrorPanel title="Current account unavailable" error={account.error} onRetry={account.refetch} />
        ) : null}

        <StatsRow
          stats={stats}
          loading={!callerAccount || isLoading}
          onContractViolationsClick={() =>
            navigate('/audit', { state: { eventTypeFilter: 'contract_violations', timeRange: '24h' } })
          }
        />

        <Filters
          search={search}
          stateFilter={stateFilter}
          roleFilter={roleFilter}
          serviceFilter={serviceFilter}
          orgFilter={orgFilter}
          channelFilter={channelFilter}
          closeReasonFilter={closeReasonFilter}
          serviceOptions={serviceOptions}
          orgOptions={orgOptions}
          channelOptions={channelOptions}
          closeReasonOptions={closeReasonOptions}
          onSearchChange={setSearch}
          onStateFilterChange={setStateFilter}
          onRoleFilterChange={setRoleFilter}
          onServiceFilterChange={setServiceFilter}
          onOrgFilterChange={setOrgFilter}
          onChannelFilterChange={setChannelFilter}
          onCloseReasonFilterChange={setCloseReasonFilter}
          onResetFilters={isFiltered ? handleResetFilters : undefined}
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
            <>
              <DataTable
                columns={sessionColumns()}
                rows={paginatedActiveRows}
                getRowKey={(row) => row.session.id}
                onRowClick={(row) => setSelectedSessionId(row.session.id)}
                className="rounded-none border-0"
                emptyState={
                  <div className="p-5">
                    <EmptyState
                      icon={Activity}
                      title="No active sessions"
                      description={
                        isFiltered
                          ? 'No in-flight sessions match the current filters.'
                          : 'No active sessions right now.'
                      }
                    />
                  </div>
                }
              />
              <div className="border-t border-border">
                <Pagination
                  totalItems={visibleActiveRows.length}
                  itemsPerPage={activeItemsPerPage}
                  currentPage={safeActivePage}
                  onPageChange={setActiveCurrentPage}
                  onItemsPerPageChange={(count) => {
                    setActiveItemsPerPage(count);
                    setActiveCurrentPage(1);
                  }}
                />
              </div>
            </>
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
            <>
              <DataTable
                columns={sessionColumns(false, true)}
                rows={paginatedRecentRows}
                getRowKey={(row) => row.session.id}
                onRowClick={(row) => setSelectedSessionId(row.session.id)}
                className="rounded-none border-0"
                emptyState={
                  <div className="p-5">
                    <EmptyState
                      icon={Clock3}
                      title="No recent sessions"
                      description={
                        isFiltered
                          ? 'No recent sessions match the current filters.'
                          : 'No recent sessions yet.'
                      }
                    />
                  </div>
                }
              />
              <div className="border-t border-border">
                <Pagination
                  totalItems={visibleRecentRows.length}
                  itemsPerPage={recentItemsPerPage}
                  currentPage={safeRecentPage}
                  onPageChange={setRecentCurrentPage}
                  onItemsPerPageChange={(count) => {
                    setRecentItemsPerPage(count);
                    setRecentCurrentPage(1);
                  }}
                />
              </div>
            </>
          )}
        </SectionPanel>
      </div>

      {selectedRow ? (
        <SessionDrawer
          session={selectedRow.session}
          role={selectedRow.role}
          counterpartyOrganizationName={selectedRow.counterparty.organizationName}
          onClose={() => setSelectedSessionId(undefined)}
        />
      ) : null}

      {infoOpen ? (
        <InfoDrawer title="About Sessions" onClose={() => setInfoOpen(false)}>
          <div className="flex flex-col gap-5">
            <section>
              <h3 className="mb-2 font-semibold text-text">What is a Session?</h3>
              <p className="leading-relaxed text-text-mute">
                A Session is the governed communication channel between two agents. Sessions have an
                explicit lifecycle and are backed one-to-one by an encrypted Layer 1 tunnel. Either side
                can close a session; policy or workgroup changes can close it automatically. Closed sessions
                are never deleted — they are retained with a recorded close reason for audit.
              </p>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-3 font-semibold text-text">Session Lifecycle</h3>
              <DrawerStepList steps={[
                { name: 'Proposed', description: 'One agent proposes a session to another, referencing the target\'s advertisement and a contract.' },
                { name: 'Accepting', description: 'The receiving agent (or the controller on its behalf) evaluates the proposal against the advertised contract terms.' },
                { name: 'Active', description: 'The session is live. Agents exchange envelopes over the backing tunnel.' },
                { name: 'Closing', description: 'Either side, a contract violation, or a workgroup change initiates closure. The close reason is captured.' },
                { name: 'Closed', description: 'The session is terminated and retained permanently for audit. The close reason is recorded.' },
              ]} />
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-2 font-semibold text-text">Contract Enforcement</h3>
              <p className="mb-3 leading-relaxed text-text-mute">
                Every session carries a contract snapshot frozen at establishment time. The controller
                enforces the terms throughout the session — if an agent sends a message type outside the
                allowed set, or the session exceeds its maximum duration, the controller closes it.
                The agents do not need to implement this logic themselves.
              </p>
              <DrawerTip>
                The contract is frozen as a snapshot at session establishment. Later changes to the contract definition do not affect in-flight sessions.
              </DrawerTip>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-2 font-semibold text-text">Audit Trail</h3>
              <p className="leading-relaxed text-text-mute">
                Every envelope exchanged in a session carries a <DrawerCodeChip>correlation_id</DrawerCodeChip>. The full interaction
                chain — which requests were made, which responses were returned — is reconstructible
                from controller audit logs after the session closes.
              </p>
            </section>
          </div>
        </InfoDrawer>
      ) : null}
    </AppShell>
  );
}

function StatsRow({
  stats,
  loading,
  onContractViolationsClick,
}: {
  stats: SessionStats;
  loading: boolean;
  onContractViolationsClick?: () => void;
}) {
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
        onClick={onContractViolationsClick}
        caret={Boolean(onContractViolationsClick)}
      />
    </div>
  );
}

function Filters({
  search,
  stateFilter,
  roleFilter,
  serviceFilter,
  orgFilter,
  channelFilter,
  closeReasonFilter,
  serviceOptions,
  orgOptions,
  channelOptions,
  closeReasonOptions,
  onSearchChange,
  onStateFilterChange,
  onRoleFilterChange,
  onServiceFilterChange,
  onOrgFilterChange,
  onChannelFilterChange,
  onCloseReasonFilterChange,
  onResetFilters,
}: {
  search: string;
  stateFilter: SessionStateFilter;
  roleFilter: SessionRoleFilter;
  serviceFilter: string;
  orgFilter: string;
  channelFilter: string;
  closeReasonFilter: string;
  serviceOptions: string[];
  orgOptions: string[];
  channelOptions: string[];
  closeReasonOptions: string[];
  onSearchChange: (value: string) => void;
  onStateFilterChange: (value: SessionStateFilter) => void;
  onRoleFilterChange: (value: SessionRoleFilter) => void;
  onServiceFilterChange: (value: string) => void;
  onOrgFilterChange: (value: string) => void;
  onChannelFilterChange: (value: string) => void;
  onCloseReasonFilterChange: (value: string) => void;
  onResetFilters?: () => void;
}) {
  return (
    <section className="rounded-card border border-border bg-panel p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center sm:gap-3">
        <label className="relative block w-full sm:min-w-48 sm:flex-1">
          <span className="sr-only">Search sessions</span>
          <Search size={17} aria-hidden="true" className="absolute left-3 top-1/2 -translate-y-1/2 text-text-mute" />
          <Input
            type="search"
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder="Search sessions, accounts, organizations, advertisements, or workgroups"
            className="pl-10 pr-3"
          />
        </label>

        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:shrink-0 sm:items-center sm:gap-2">
          <FilterSelect
            value={stateFilter}
            onChange={(value) => onStateFilterChange(value as SessionStateFilter)}
          >
            <option value="all">All Statuses</option>
            <option value="proposed">Proposed</option>
            <option value="accepting">Accepting</option>
            <option value="active">Active</option>
            <option value="closing">Closing</option>
            <option value="closed">Closed</option>
          </FilterSelect>
          <FilterSelect
            value={roleFilter}
            onChange={(value) => onRoleFilterChange(value as SessionRoleFilter)}
          >
            <option value="both">All Roles</option>
            <option value="provider">Provider</option>
            <option value="consumer">Consumer</option>
          </FilterSelect>
          <FilterSelect value={serviceFilter} onChange={onServiceFilterChange}>
            <option value="all">All Services</option>
            {serviceOptions.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </FilterSelect>
          <FilterSelect value={orgFilter} onChange={onOrgFilterChange}>
            <option value="all">Sessions With: All</option>
            {orgOptions.map((o) => (
              <option key={o} value={o}>{o}</option>
            ))}
          </FilterSelect>
          <FilterSelect value={channelFilter} onChange={onChannelFilterChange}>
            <option value="all">All Workgroups</option>
            {channelOptions.map((c) => (
              <option key={c} value={c}>{c}</option>
            ))}
          </FilterSelect>
          {closeReasonOptions.length > 0 && (
            <FilterSelect value={closeReasonFilter} onChange={onCloseReasonFilterChange}>
              <option value="all">All Close Reasons</option>
              {closeReasonOptions.map((r) => (
                <option key={r} value={r}>{closeReasonLabels[r] ?? r}</option>
              ))}
            </FilterSelect>
          )}
          {onResetFilters ? (
            <button
              type="button"
              onClick={onResetFilters}
              className="h-9 w-full rounded-pill border border-border bg-panel px-3 text-table font-medium text-text-mute-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora sm:w-auto"
            >
              Reset filters
            </button>
          ) : null}
        </div>
      </div>
    </section>
  );
}

function FilterSelect({
  value,
  onChange,
  children,
}: {
  value: string;
  onChange: (value: string) => void;
  children: ReactNode;
}) {
  const isFiltered = value !== 'all' && value !== 'both';

  return (
    <div className="relative">
      <Select
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className={[
          'w-full pl-3 pr-7 font-medium sm:w-auto',
          isFiltered
            ? 'border-brand-agora bg-brand-agora/10 text-brand-agora'
            : 'text-text-mute-strong',
        ].join(' ')}
      >
        {children}
      </Select>
      <ChevronDown
        size={13}
        aria-hidden="true"
        className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-current"
      />
    </div>
  );
}

function sessionColumns(sortable = true, showChevron = false): DataTableColumn<SessionRow>[] {
  const columns: DataTableColumn<SessionRow>[] = [
    {
      id: 'id',
      header: 'Session',
      accessor: (row) => row.session.id,
      kind: 'mono',
      sortable,
      sortValue: (row) => row.session.id,
    },
    {
      id: 'role',
      header: 'Role',
      accessor: (row) => row.role.charAt(0).toUpperCase() + row.role.slice(1),
      sortable,
    },
    {
      id: 'service',
      header: 'Service',
      accessor: (row) => row.session.advertisementName,
      sortable,
    },
    {
      id: 'organization',
      header: 'Session with',
      accessor: (row) => row.counterparty.organizationName,
      sortable,
      sortValue: (row) => row.counterparty.organizationName,
    },
    {
      id: 'channel',
      header: 'Workgroup',
      accessor: (row) => row.session.workgroupName ?? '',
      sortable,
    },
    {
      id: 'status',
      header: 'Status',
      accessor: (row) => ({
        status: row.session.closeReason ? closeReasonStatus(row.session.closeReason) : stateStatus(row.session.state),
        label: row.session.state,
      }),
      kind: 'pill',
      sortable,
    },
    {
      id: 'closeReason',
      header: 'Close Reason',
      accessor: (row) => {
        const reason = row.session.closeReason;
        if (!reason) return '';
        const label = closeReasonLabels[reason] ?? reason;
        const status = closeReasonStatus(reason);
        const colorClass =
          status === 'success' ? 'text-success-strong' :
          status === 'danger' ? 'text-danger' :
          status === 'warning' ? 'text-warning-strong' : '';
        return colorClass ? <span className={colorClass}>{label}</span> : <span>{label}</span>;
      },
      sortable,
      sortValue: (row) => row.session.closeReason ?? '',
    },
  ];

  if (showChevron) {
    columns.push({
      id: 'action',
      header: '',
      accessor: () => <ChevronRight size={15} className="text-text-mute" aria-hidden="true" />,
      align: 'right',
    });
  }

  return columns;
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
  serviceFilter: string,
  orgFilter: string,
  channelFilter: string,
  closeReasonFilter: string,
): SessionRow[] {
  const normalizedSearch = search.trim().toLowerCase();

  return rows.filter((row) => {
    if (stateFilter !== 'all' && row.session.state !== stateFilter) {
      return false;
    }

    if (roleFilter !== 'both' && row.role !== roleFilter) {
      return false;
    }

    if (serviceFilter !== 'all' && row.session.advertisementName !== serviceFilter) {
      return false;
    }

    if (orgFilter !== 'all' && row.counterparty.organizationName !== orgFilter) {
      return false;
    }

    if (channelFilter !== 'all' && (row.session.workgroupName ?? '') !== channelFilter) {
      return false;
    }

    if (closeReasonFilter !== 'all' && (row.session.closeReason ?? '') !== closeReasonFilter) {
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

export function durationSortValue(session: Session): number {
  return durationMilliseconds(session.proposedAt, session.closedAt) ?? 0;
}

function durationForSession(session: Session, now: number): string {
  const end = session.closedAt ? Date.parse(session.closedAt) : now;
  const start = Date.parse(session.proposedAt);

  if (Number.isNaN(start) || Number.isNaN(end) || end < start) {
    return '-';
  }

  return formatDuration(end - start);
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

export function roleStatus(role: DerivedRole): StatusPillStatus {
  if (role === 'provider') {
    return 'info';
  }

  if (role === 'consumer') {
    return 'success';
  }

  return 'neutral';
}

export function closeReasonStatus(reason: NonNullable<Session['closeReason']>): StatusPillStatus {
  if (reason === 'consumer_close' || reason === 'provider_close') return 'success';
  if (reason === 'contract_violation') return 'danger';
  if (reason === 'tunnel_failed') return 'warning';
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

function uniqueSortedValues<T>(rows: T[], getValue: (row: T) => string): string[] {
  return [...new Set(rows.map(getValue))].sort();
}

function formatInteger(value: number): string {
  return numberFormatter.format(value);
}
