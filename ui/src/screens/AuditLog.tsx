import { useCallback, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import {
  Activity,
  AlertTriangle,
  Download,
  FileCheck2,
  FileSearch,
  RefreshCcw,
  ShieldAlert,
  Users,
  Wifi,
} from 'lucide-react';

import { AppShell, EmptyState, SectionPanel, StatCard, StatusPill, type StatusPillStatus } from '../components';
import {
  ApiError,
  fetchAllAuditEvents,
  getDashboardSummary,
  useApiResource,
  type AuditEvent,
  type AuditEventType,
} from '../lib/api';

type TimeRange = '24h' | '7d' | '30d';
type FilterOption = {
  id: string;
  label: string;
};
type ComplianceSummary = {
  totalEvents: number;
  uniqueAccounts: number;
  uniqueWorkgroups: number;
  envelopeCount: number;
  contractViolations: number;
  byType: FilterOptionWithCount[];
};
type FilterOptionWithCount = FilterOption & {
  count: number;
};
type ResourceOption = {
  label: string;
  value: string;
};

const routeByTab: Record<string, string> = {
  dashboard: '/',
  sessions: '/sessions',
  workgroups: '/workgroups',
  catalog: '/catalog',
  contracts: '/contracts',
  audit: '/audit',
};

const timeRangeOptions: { id: TimeRange; label: string; durationMs: number }[] = [
  { id: '24h', label: '24 hours', durationMs: 24 * 60 * 60 * 1000 },
  { id: '7d', label: '7 days', durationMs: 7 * 24 * 60 * 60 * 1000 },
  { id: '30d', label: '30 days', durationMs: 30 * 24 * 60 * 60 * 1000 },
];

const auditEventTypes: AuditEventType[] = [
  'session.proposed',
  'session.accepted',
  'session.rejected',
  'session.closed',
  'envelope.flowed',
  'tunnel.attached',
  'tunnel.detached',
  'advertisement.published',
  'advertisement.retracted',
  'environment.heartbeat',
  'account.login',
  'account.login_failed',
  'account.logout',
];

const numberFormatter = new Intl.NumberFormat();

export default function AuditLog() {
  const navigate = useNavigate();
  const [timeRange, setTimeRange] = useState<TimeRange>('24h');
  const [eventTypeFilter, setEventTypeFilter] = useState<AuditEventType | 'all'>('all');
  const [workgroupFilter, setWorkgroupFilter] = useState('all');
  const [accountFilter, setAccountFilter] = useState('all');
  const [reportOpen, setReportOpen] = useState(false);
  const account = useApiResource(getDashboardSummary);
  const auditLoad = useCallback((signal: AbortSignal) => fetchAllAuditEvents(rangeParams(timeRange), signal), [timeRange]);
  const auditEvents = useApiResource(auditLoad);
  const callerAccount = account.data?.account;
  const hasError = Boolean(account.error || auditEvents.error);
  const isLoading = account.loading || auditEvents.loading;
  const events = useMemo(() => auditEvents.data ?? [], [auditEvents.data]);

  const workgroupOptions = useMemo(() => buildIDOptions(events, (event) => event.workgroupId), [events]);
  const accountOptions = useMemo(() => buildIDOptions(events, (event) => event.accountId), [events]);
  const filteredEvents = useMemo(
    () => filterAuditEvents(events, eventTypeFilter, workgroupFilter, accountFilter),
    [events, eventTypeFilter, workgroupFilter, accountFilter],
  );
  const summary = useMemo(() => buildComplianceSummary(filteredEvents), [filteredEvents]);

  function handleTabChange(tabId: string) {
    const route = routeByTab[tabId];

    if (route) {
      navigate(route);
    }
  }

  function refetchAll() {
    account.refetch();
    auditEvents.refetch();
  }

  return (
    <AppShell
      product="agora"
      organizationName={callerAccount?.organizationName ?? 'Loading organization'}
      activeTab="audit"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'All systems operational'}
      userInitials={callerAccount ? initialsFromEmail(callerAccount.email) : '--'}
      userLabel={callerAccount?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        {account.error ? <ErrorPanel title="Current account unavailable" error={account.error} onRetry={account.refetch} /> : null}

        <AuditOverview summary={summary} loading={!callerAccount || isLoading} />

        {auditEvents.error ? <ErrorPanel title="Audit events unavailable" error={auditEvents.error} onRetry={refetchAll} /> : null}

        <AuditControls
          timeRange={timeRange}
          eventTypeFilter={eventTypeFilter}
          workgroupFilter={workgroupFilter}
          accountFilter={accountFilter}
          workgroupOptions={workgroupOptions}
          accountOptions={accountOptions}
          reportOpen={reportOpen}
          filteredEvents={filteredEvents}
          onTimeRangeChange={setTimeRange}
          onEventTypeFilterChange={setEventTypeFilter}
          onWorkgroupFilterChange={setWorkgroupFilter}
          onAccountFilterChange={setAccountFilter}
          onExportCSV={() => exportAuditCSV(filteredEvents, timeRange)}
          onToggleReport={() => setReportOpen((open) => !open)}
        />

        {reportOpen ? <ComplianceReport summary={summary} timeRange={timeRange} /> : null}

        <SectionPanel
          title="Timeline"
          actions={<StatusPill status="info" label={`${formatInteger(filteredEvents.length)} events`} />}
          bodyClassName="p-0"
        >
          {!callerAccount || auditEvents.loading ? (
            <div className="p-5">
              <LoadingPanel title="Loading audit events" compact />
            </div>
          ) : filteredEvents.length > 0 ? (
            <div className="divide-y divide-border">
              {filteredEvents.map((event) => (
                <AuditEventRow key={event.id} event={event} />
              ))}
            </div>
          ) : (
            <div className="p-5">
              <EmptyState icon={FileSearch} title="No audit events" description="No audit events match the current filters." />
            </div>
          )}
        </SectionPanel>
      </div>
    </AppShell>
  );
}

function AuditOverview({ summary, loading }: { summary: ComplianceSummary; loading: boolean }) {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <StatCard
        label="Audit Events"
        value={loading ? '-' : formatInteger(summary.totalEvents)}
        icon={FileSearch}
        accent="agora"
      />
      <StatCard
        label="Accounts"
        value={loading ? '-' : formatInteger(summary.uniqueAccounts)}
        icon={Users}
        accent="info"
      />
      <StatCard
        label="Envelope Count"
        value={loading ? '-' : formatInteger(summary.envelopeCount)}
        icon={Activity}
        accent="success"
      />
      <StatCard
        label="Violations"
        value={loading ? '-' : formatInteger(summary.contractViolations)}
        icon={ShieldAlert}
        accent={summary.contractViolations > 0 ? 'danger' : 'neutral'}
      />
    </div>
  );
}

function AuditControls({
  timeRange,
  eventTypeFilter,
  workgroupFilter,
  accountFilter,
  workgroupOptions,
  accountOptions,
  reportOpen,
  filteredEvents,
  onTimeRangeChange,
  onEventTypeFilterChange,
  onWorkgroupFilterChange,
  onAccountFilterChange,
  onExportCSV,
  onToggleReport,
}: {
  timeRange: TimeRange;
  eventTypeFilter: AuditEventType | 'all';
  workgroupFilter: string;
  accountFilter: string;
  workgroupOptions: FilterOption[];
  accountOptions: FilterOption[];
  reportOpen: boolean;
  filteredEvents: AuditEvent[];
  onTimeRangeChange: (range: TimeRange) => void;
  onEventTypeFilterChange: (eventType: AuditEventType | 'all') => void;
  onWorkgroupFilterChange: (workgroupId: string) => void;
  onAccountFilterChange: (accountId: string) => void;
  onExportCSV: () => void;
  onToggleReport: () => void;
}) {
  return (
    <section className="rounded-card border border-border bg-panel p-4">
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_auto]">
        <div className="flex flex-wrap gap-3">
          <FilterGroup label="Range">
            {timeRangeOptions.map((option) => (
              <FilterButton
                key={option.id}
                active={timeRange === option.id}
                label={option.label}
                onClick={() => onTimeRangeChange(option.id)}
              />
            ))}
          </FilterGroup>

          <label className="flex min-w-48 flex-col gap-1">
            <span className="text-label font-medium uppercase text-text-mute">Event Type</span>
            <select
              value={eventTypeFilter}
              onChange={(event) => onEventTypeFilterChange(event.target.value as AuditEventType | 'all')}
              className="h-10 rounded-pill border border-border bg-panel-subtle px-3 text-body text-text outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            >
              <option value="all">all events</option>
              {auditEventTypes.map((eventType) => (
                <option key={eventType} value={eventType}>
                  {eventType}
                </option>
              ))}
            </select>
          </label>

          <label className="flex min-w-48 flex-col gap-1">
            <span className="text-label font-medium uppercase text-text-mute">Workgroup</span>
            <select
              value={workgroupFilter}
              onChange={(event) => onWorkgroupFilterChange(event.target.value)}
              className="h-10 rounded-pill border border-border bg-panel-subtle px-3 text-body text-text outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            >
              <option value="all">all workgroups</option>
              {workgroupOptions.map((option) => (
                <option key={option.id} value={option.id}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>

          <label className="flex min-w-48 flex-col gap-1">
            <span className="text-label font-medium uppercase text-text-mute">Account</span>
            <select
              value={accountFilter}
              onChange={(event) => onAccountFilterChange(event.target.value)}
              className="h-10 rounded-pill border border-border bg-panel-subtle px-3 text-body text-text outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            >
              <option value="all">all accounts</option>
              {accountOptions.map((option) => (
                <option key={option.id} value={option.id}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
        </div>

        <div className="flex flex-wrap items-end gap-3">
          <button
            type="button"
            className="inline-flex h-10 items-center gap-2 rounded-pill border border-border bg-panel-subtle px-3 text-table font-medium text-text-mute-strong hover:bg-panel focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora disabled:cursor-not-allowed disabled:opacity-50"
            disabled={filteredEvents.length === 0}
            onClick={onExportCSV}
          >
            <Download size={15} aria-hidden="true" />
            CSV
          </button>
          <button
            type="button"
            className={[
              'inline-flex h-10 items-center gap-2 rounded-pill border px-3 text-table font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
              reportOpen
                ? 'border-brand-agora bg-brand-agora/10 text-brand-agora'
                : 'border-border bg-panel-subtle text-text-mute-strong hover:bg-panel',
            ].join(' ')}
            aria-pressed={reportOpen}
            onClick={onToggleReport}
          >
            <FileCheck2 size={15} aria-hidden="true" />
            Compliance Report
          </button>
        </div>
      </div>
    </section>
  );
}

function ComplianceReport({ summary, timeRange }: { summary: ComplianceSummary; timeRange: TimeRange }) {
  return (
    <SectionPanel title="Compliance Report" actions={<StatusPill status="neutral" label={timeRangeLabel(timeRange)} />}>
      <div className="grid gap-4 lg:grid-cols-[1fr_1.2fr]">
        <div className="grid gap-3 sm:grid-cols-2">
          <ReportMetric label="events reviewed" value={formatInteger(summary.totalEvents)} />
          <ReportMetric label="accounts observed" value={formatInteger(summary.uniqueAccounts)} />
          <ReportMetric label="workgroups observed" value={formatInteger(summary.uniqueWorkgroups)} />
          <ReportMetric label="contract violations" value={formatInteger(summary.contractViolations)} />
          <ReportMetric label="envelopes reported" value={formatInteger(summary.envelopeCount)} />
        </div>
        <div className="rounded-card border border-border bg-panel-subtle p-4">
          <p className="text-label font-medium uppercase text-text-mute">Event Mix</p>
          <div className="mt-3 grid gap-2">
            {summary.byType.length > 0 ? (
              summary.byType.map((row) => (
                <div key={row.id} className="flex items-center justify-between gap-3 text-table">
                  <span className="min-w-0 truncate font-medium text-text-mute-strong">{row.label}</span>
                  <span className="font-mono text-text">{formatInteger(row.count)}</span>
                </div>
              ))
            ) : (
              <p className="text-body text-text-mute">No events in this report.</p>
            )}
          </div>
        </div>
      </div>
    </SectionPanel>
  );
}

function ReportMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-card border border-border bg-panel-subtle p-3">
      <p className="text-label font-medium uppercase text-text-mute">{label}</p>
      <p className="mt-1 text-section font-semibold text-text">{value}</p>
    </div>
  );
}

function AuditEventRow({ event }: { event: AuditEvent }) {
  const data = event.data ?? {};
  const resourcePills = eventResources(event);

  return (
    <article className="grid gap-4 p-5 xl:grid-cols-[12rem_minmax(0,1fr)]">
      <div>
        <p className="font-mono text-table text-text-mute-strong">{formatDateTime(event.occurredAt)}</p>
        <p className="mt-1 text-table text-text-mute">{relativeTime(event.occurredAt)}</p>
      </div>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <StatusPill status={eventStatus(event)} label={event.eventType} />
          {event.accountId ? <ResourcePill label={event.accountId} /> : null}
        </div>
        <h2 className="mt-3 text-body font-semibold text-text">{eventTitle(event)}</h2>
        {resourcePills.length > 0 ? (
          <div className="mt-3 flex flex-wrap gap-2">
            {resourcePills.map((resource) => (
              <ResourcePill key={`${resource.label}:${resource.value}`} label={resource.label} value={resource.value} />
            ))}
          </div>
        ) : null}
        {Object.keys(data).length > 0 ? (
          <pre className="mt-3 max-h-40 overflow-auto rounded-card border border-border bg-panel-subtle p-3 font-mono text-table text-text-mute-strong">
            {JSON.stringify(data, null, 2)}
          </pre>
        ) : null}
      </div>
    </article>
  );
}

function ResourcePill({ label, value }: { label: string; value?: string }) {
  return (
    <span className="inline-flex max-w-full items-center gap-1 rounded-status border border-border bg-panel-subtle px-2 py-1 text-table text-text-mute-strong">
      {value ? <span className="text-text-mute">{label}</span> : null}
      <span className="truncate font-mono">{value ?? label}</span>
    </span>
  );
}

function FilterGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-end gap-2">
      <span className="mb-2 text-label font-medium uppercase text-text-mute">{label}</span>
      {children}
    </div>
  );
}

function FilterButton({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className={[
        'h-10 rounded-pill border px-3 text-table font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
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

function rangeParams(timeRange: TimeRange): { from: string; to: string } {
  const option = timeRangeOptions.find((candidate) => candidate.id === timeRange) ?? timeRangeOptions[0]!;
  const to = new Date();
  const from = new Date(to.getTime() - option.durationMs);

  return {
    from: from.toISOString(),
    to: to.toISOString(),
  };
}

function filterAuditEvents(
  events: AuditEvent[],
  eventTypeFilter: AuditEventType | 'all',
  workgroupFilter: string,
  accountFilter: string,
): AuditEvent[] {
  return events.filter((event) => {
    if (eventTypeFilter !== 'all' && event.eventType !== eventTypeFilter) {
      return false;
    }
    if (workgroupFilter !== 'all' && event.workgroupId !== workgroupFilter) {
      return false;
    }
    if (accountFilter !== 'all' && event.accountId !== accountFilter) {
      return false;
    }
    return true;
  });
}

function buildIDOptions(events: AuditEvent[], select: (event: AuditEvent) => string | undefined): FilterOption[] {
  return Array.from(new Set(events.map(select).filter((value): value is string => Boolean(value))))
    .sort((a, b) => a.localeCompare(b))
    .map((id) => ({ id, label: id }));
}

function buildComplianceSummary(events: AuditEvent[]): ComplianceSummary {
  const accounts = new Set<string>();
  const workgroups = new Set<string>();
  const byType = new Map<AuditEventType, number>();
  let envelopeCount = 0;
  let contractViolations = 0;

  events.forEach((event) => {
    if (event.accountId) {
      accounts.add(event.accountId);
    }
    if (event.workgroupId) {
      workgroups.add(event.workgroupId);
    }
    byType.set(event.eventType, (byType.get(event.eventType) ?? 0) + 1);
    if (event.eventType === 'envelope.flowed') {
      envelopeCount += dataNumber(event.data, 'count_delta');
    }
    if (event.eventType === 'session.closed' && dataString(event.data, 'close_reason') === 'contract_violation') {
      contractViolations += 1;
    }
  });

  return {
    totalEvents: events.length,
    uniqueAccounts: accounts.size,
    uniqueWorkgroups: workgroups.size,
    envelopeCount,
    contractViolations,
    byType: Array.from(byType.entries())
      .map(([eventType, count]) => ({ id: eventType, label: eventType, count }))
      .sort((left, right) => right.count - left.count || left.label.localeCompare(right.label)),
  };
}

function eventResources(event: AuditEvent): ResourceOption[] {
  return [
    event.workgroupId ? { label: 'workgroup', value: event.workgroupId } : undefined,
    event.sessionId ? { label: 'session', value: event.sessionId } : undefined,
    event.advertisementId ? { label: 'advertisement', value: event.advertisementId } : undefined,
    event.contractId ? { label: 'contract', value: event.contractId } : undefined,
    event.envelopeId ? { label: 'envelope', value: event.envelopeId } : undefined,
  ].filter((resource): resource is ResourceOption => Boolean(resource));
}

function eventTitle(event: AuditEvent): string {
  const data = event.data;

  switch (event.eventType) {
    case 'session.proposed':
      return `session proposed for ${event.advertisementId ?? 'advertisement'}`;
    case 'session.accepted':
      return `session accepted${dataString(data, 'tunnel_id') ? ` on ${dataString(data, 'tunnel_id')}` : ''}`;
    case 'session.rejected':
      return `session rejected${dataString(data, 'reason') ? `: ${dataString(data, 'reason')}` : ''}`;
    case 'session.closed':
      return `session closed${dataString(data, 'close_reason') ? `: ${dataString(data, 'close_reason')}` : ''}`;
    case 'envelope.flowed':
      return `${formatInteger(dataNumber(data, 'count_delta'))} envelopes flowed`;
    case 'tunnel.attached':
      return `tunnel attached${dataString(data, 'tunnel_id') ? `: ${dataString(data, 'tunnel_id')}` : ''}`;
    case 'tunnel.detached':
      return `tunnel detached${dataString(data, 'final_state') ? `: ${dataString(data, 'final_state')}` : ''}`;
    case 'advertisement.published':
      return `advertisement published${dataString(data, 'name') ? `: ${dataString(data, 'name')}` : ''}`;
    case 'advertisement.retracted':
      return `advertisement retracted${dataString(data, 'reason') ? `: ${dataString(data, 'reason')}` : ''}`;
    case 'environment.heartbeat':
      return `environment heartbeat${dataString(data, 'environment_id') ? `: ${dataString(data, 'environment_id')}` : ''}`;
    case 'account.login':
      return `account login${dataString(data, 'email') ? `: ${dataString(data, 'email')}` : ''}`;
    case 'account.login_failed':
      return `account login failed${dataString(data, 'email_attempted') ? `: ${dataString(data, 'email_attempted')}` : ''}`;
    case 'account.logout':
      return `account logout${dataString(data, 'email') ? `: ${dataString(data, 'email')}` : ''}`;
    default:
      return event.eventType;
  }
}

function eventStatus(event: AuditEvent): StatusPillStatus {
  if (event.eventType === 'account.login_failed') {
    return 'danger';
  }
  if (event.eventType === 'session.closed' && dataString(event.data, 'close_reason') === 'contract_violation') {
    return 'danger';
  }
  if (event.eventType.startsWith('session.')) {
    return event.eventType === 'session.closed' ? 'neutral' : 'success';
  }
  if (event.eventType.startsWith('envelope.')) {
    return 'info';
  }
  if (event.eventType.startsWith('tunnel.')) {
    return 'warning';
  }
  if (event.eventType.startsWith('advertisement.')) {
    return 'active';
  }
  if (event.eventType.startsWith('account.')) {
    return 'success';
  }
  return 'neutral';
}

function exportAuditCSV(events: AuditEvent[], timeRange: TimeRange) {
  const rows = [
    ['id', 'occurred_at', 'event_type', 'organization_id', 'account_id', 'workgroup_id', 'session_id', 'advertisement_id', 'contract_id', 'envelope_id', 'data'],
    ...events.map((event) => [
      String(event.id),
      event.occurredAt,
      event.eventType,
      event.organizationId,
      event.accountId ?? '',
      event.workgroupId ?? '',
      event.sessionId ?? '',
      event.advertisementId ?? '',
      event.contractId ?? '',
      event.envelopeId ?? '',
      JSON.stringify(event.data ?? {}),
    ]),
  ];
  const csv = rows.map((row) => row.map(csvCell).join(',')).join('\n');
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');

  link.href = url;
  link.download = `agora-audit-${timeRange}-${new Date().toISOString().slice(0, 10)}.csv`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function csvCell(value: string): string {
  return `"${value.replace(/"/g, '""')}"`;
}

function dataString(data: Record<string, unknown>, key: string): string {
  const value = data[key];

  return typeof value === 'string' ? value : '';
}

function dataNumber(data: Record<string, unknown>, key: string): number {
  const value = data[key];

  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }

  if (typeof value === 'string') {
    const parsed = Number(value);

    return Number.isFinite(parsed) ? parsed : 0;
  }

  return 0;
}

function formatDateTime(value: string): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  }).format(date);
}

function relativeTime(value: string): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return '';
  }

  const seconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000));

  if (seconds < 60) {
    return `${seconds}s ago`;
  }

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m ago`;
  }

  const hours = Math.floor(minutes / 60);
  if (hours < 48) {
    return `${hours}h ago`;
  }

  return `${Math.floor(hours / 24)}d ago`;
}

function timeRangeLabel(timeRange: TimeRange): string {
  return timeRangeOptions.find((option) => option.id === timeRange)?.label ?? timeRange;
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

function formatInteger(value: number): string {
  return numberFormatter.format(value);
}
