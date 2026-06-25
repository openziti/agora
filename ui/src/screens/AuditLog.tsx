import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from 'react';
import { useLocation, useNavigate } from 'react-router';
import {
  Activity,
  AlertTriangle,
  ArrowLeftRight,
  ChevronDown,
  ChevronRight,
  Download,
  FileCheck2,
  FileSearch,
  Mail,
  RefreshCcw,
  ScrollText,
  ShieldAlert,
  ShieldCheck,
  Users,
  Wifi,
} from 'lucide-react';

import { AppShell, Button, DrawerCard, DrawerCodeChip, DrawerDivider, DrawerTip, EmptyState, InfoDrawer, PageHeader, Select, SectionPanel, StatCard, StatusPill, type StatusPillStatus } from '../components';
import {
  ApiError,
  fetchAllAuditEvents,
  getDashboardSummary,
  getSession,
  listWorkgroups,
  useApiResource,
  type AuditEvent,
  type AuditEventType,
  type Session,
} from '../lib/api';

import { SessionDrawer } from './SessionDrawer';

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
  muted?: boolean;
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
  'account.login',
  'account.login_failed',
  'account.logout',
  'advertisement.published',
  'advertisement.retracted',
  // Contract Violations (virtual filter) sorts here
  'envelope.flowed',
  'environment.heartbeat',
  'session.accepted',
  'session.closed',
  'session.proposed',
  'session.rejected',
  'tunnel.attached',
  'tunnel.detached',
];

const numberFormatter = new Intl.NumberFormat();
const AUDIT_POLL_MS = 5000;

const ROW_HEIGHT = 16;
const ROW_GAP = 4;
const LEGEND_WIDTH = 140;

type LaneDef = { label: string; cssColor: string };

const LANE_DEFS: LaneDef[] = [
  { label: 'Sessions',       cssColor: 'var(--color-info)' },
  { label: 'Violations',     cssColor: 'var(--color-danger)' },
  { label: 'Envelopes',      cssColor: 'var(--color-success)' },
  { label: 'Tunnels',        cssColor: 'var(--color-warning)' },
  { label: 'Advertisements', cssColor: 'var(--color-brand-agora)' },
  { label: 'Account & Environment', cssColor: 'var(--color-text-mute-2)' },
];

type TickMark = { position: number; width: number };
type ViolationTickMark = { position: number; event: AuditEvent; gap: number };

const EVENT_TYPE_LABELS: Partial<Record<string, string>> = {
  'session.proposed': 'Session Proposed',
  'session.accepted': 'Session Accepted',
  'session.rejected': 'Session Rejected',
  'session.closed': 'Session Closed',
  'envelope.flowed': 'Envelope Transferred',
  'environment.heartbeat': 'Environment Heartbeat',
  'tunnel.attached': 'Tunnel Attached',
  'tunnel.detached': 'Tunnel Detached',
  'account.login': 'Account Login',
  'account.login_failed': 'Account Login Failed',
  'account.logout': 'Account Logout',
  'advertisement.published': 'Advertisement Published',
  'advertisement.retracted': 'Advertisement Retracted',
};

function formatEventType(eventType: string): string {
  return EVENT_TYPE_LABELS[eventType] ?? eventType;
}

export default function AuditLog() {
  const navigate = useNavigate();
  const location = useLocation();
  const [infoOpen, setInfoOpen] = useState(false);
  const [auditSelectedSession, setAuditSelectedSession] = useState<Session | null>(null);
  const [timeRange, setTimeRange] = useState<TimeRange>(() => {
    const state = location.state as { timeRange?: TimeRange } | null;
    return state?.timeRange ?? '24h';
  });
  const [eventTypeFilter, setEventTypeFilter] = useState<AuditEventType | 'all' | 'contract_violations'>(() => {
    const state = location.state as { eventTypeFilter?: AuditEventType | 'all' | 'contract_violations' } | null;
    return state?.eventTypeFilter ?? 'all';
  });
  const [workgroupFilter, setWorkgroupFilter] = useState('all');
  const [accountFilter, setAccountFilter] = useState('all');
  const [reportOpen, setReportOpen] = useState(false);
  const account = useApiResource(getDashboardSummary);
  const workgroupsResource = useApiResource(listWorkgroups);
  const auditLoad = useCallback((signal: AbortSignal) => fetchAllAuditEvents(rangeParams(timeRange), signal), [timeRange]);
  const auditEvents = useApiResource(auditLoad, { intervalMs: AUDIT_POLL_MS });
  const callerAccount = account.data?.account;
  const hasError = Boolean(account.error || auditEvents.error);
  const isLoading = (account.loading && !account.data) || (auditEvents.loading && !auditEvents.data);
  const events = useMemo(() => auditEvents.data ?? [], [auditEvents.data]);

  const workgroupNameMap = useMemo(() => {
    const map: Record<string, string> = {};
    workgroupsResource.data?.forEach((wg) => { map[wg.id] = wg.name; });
    return map;
  }, [workgroupsResource.data]);

  const advertisementNameMap = useMemo(() => {
    const map: Record<string, string> = {};
    events.forEach((event) => {
      if (event.advertisementId && event.eventType === 'advertisement.published') {
        const name = dataString(event.data ?? {}, 'name');
        if (name) map[event.advertisementId] = name;
      }
    });
    return map;
  }, [events]);

  const workgroupOptions = useMemo(
    () => buildIDOptions(events, (event) => event.workgroupId, workgroupNameMap),
    [events, workgroupNameMap],
  );
  const accountOptions = useMemo(() => buildIDOptions(events, (event) => event.accountId), [events]);
  const filteredEvents = useMemo(
    () => filterAuditEvents(events, eventTypeFilter, workgroupFilter, accountFilter),
    [events, eventTypeFilter, workgroupFilter, accountFilter],
  );
  const summary = useMemo(() => buildComplianceSummary(filteredEvents), [filteredEvents]);

  function handleSessionClick(sessionId: string) {
    getSession(sessionId)
      .then((session) => setAuditSelectedSession(session))
      .catch((error: unknown) => console.error('Failed to load session', error));
  }

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

  const isFiltered =
    eventTypeFilter !== 'all' ||
    workgroupFilter !== 'all' ||
    accountFilter !== 'all';

  function handleResetFilters() {
    setEventTypeFilter('all');
    setWorkgroupFilter('all');
    setAccountFilter('all');
  }

  return (
    <AppShell
      product="agora"
      organizationName={callerAccount?.organizationName ?? 'Loading organization'}
      activeTab="audit"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'Connected'}
      userInitials={callerAccount ? initialsFromEmail(callerAccount.email) : '--'}
      userLabel={callerAccount?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        <PageHeader
          icon={ScrollText}
          label="COMPLIANCE"
          title="Audit"
          description="The complete event trail for all agent activity — every session, envelope, and contract enforcement action, reconstructible from end to end."
          onInfoClick={() => setInfoOpen(true)}
        />
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
          onResetFilters={isFiltered ? handleResetFilters : undefined}
        />

        <ActivityStrip
          events={filteredEvents}
          timeRange={timeRange}
          workgroupNames={workgroupNameMap}
          onSessionClick={handleSessionClick}
        />

        {reportOpen ? (
          <ComplianceReport
            summary={summary}
            timeRange={timeRange}
            accountFilter={accountFilter}
            accountOptions={accountOptions}
          />
        ) : null}

        <SectionPanel
          title={`Timeline (${formatInteger(filteredEvents.length)} events)`}
          bodyClassName="p-0"
        >
          {!callerAccount || isLoading ? (
            <div className="p-5">
              <LoadingPanel title="Loading audit events" compact />
            </div>
          ) : filteredEvents.length > 0 ? (
            <div className="divide-y divide-border">
              {filteredEvents.map((event) => (
                <AuditEventRow
                  key={event.id}
                  event={event}
                  workgroupNames={workgroupNameMap}
                  advertisementNames={advertisementNameMap}
                  onSessionClick={handleSessionClick}
                />
              ))}
            </div>
          ) : (
            <div className="p-5">
              <EmptyState icon={FileSearch} title="No audit events" description="No audit events match the current filters." />
            </div>
          )}
        </SectionPanel>
      </div>

      {auditSelectedSession ? (
        <SessionDrawer
          session={auditSelectedSession}
          role={
            callerAccount?.accountId === auditSelectedSession.providerAccountId
              ? 'provider'
              : callerAccount?.accountId === auditSelectedSession.consumerAccountId
                ? 'consumer'
                : 'unknown'
          }
          counterpartyOrganizationName={
            callerAccount?.accountId === auditSelectedSession.providerAccountId
              ? auditSelectedSession.consumerOrganizationName
              : auditSelectedSession.providerOrganizationName
          }
          auditEvents={events}
          initialTab="trace"
          onClose={() => setAuditSelectedSession(null)}
        />
      ) : null}

      {infoOpen ? (
        <InfoDrawer title="About the Audit Log" onClose={() => setInfoOpen(false)}>
          <div className="flex flex-col gap-5">
            <section>
              <h3 className="mb-2 font-semibold text-text">What is the Audit Log?</h3>
              <p className="leading-relaxed text-text-mute">
                The Audit log is the controller's complete record of all agent activity in the system.
                Every session state transition, every contract enforcement action, and every close reason
                is recorded automatically — not by application-level logging, but by infrastructure logs
                that exist because every interaction passes through the governed session layer.
              </p>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-3 font-semibold text-text">What gets recorded</h3>
              <div className="flex flex-col gap-2">
                <DrawerCard icon={ArrowLeftRight} title="Session Lifecycle Events" description="Every state transition from proposed to closed, with timestamps and recorded close reason." />
                <DrawerCard icon={FileCheck2} title="Contract Violations" description="Which session, which term was violated, and the enforcement action taken." />
                <DrawerCard icon={Mail} title="Envelope Activity" description="Counts per session, tied to the session's contract snapshot." />
                <DrawerCard icon={ShieldCheck} title="Workgroup Changes" description="Membership changes that caused session closures or advertisement visibility changes." />
              </div>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-2 font-semibold text-text">Correlation IDs</h3>
              <p className="leading-relaxed text-text-mute">
                Every envelope carries a <DrawerCodeChip>correlation_id</DrawerCodeChip> that ties requests to responses across the full
                call graph. The complete chain of a multi-agent interaction — which agent requested what,
                which agent responded, in which session, under which contract — is reconstructible from
                the audit log without relying on application-level tracing.
              </p>
            </section>

            <DrawerDivider />

            <DrawerTip>
              Sessions are never deleted — closed sessions are retained indefinitely. The complete interaction chain is reconstructible from controller audit logs without relying on application-level tracing.
            </DrawerTip>
          </div>
        </InfoDrawer>
      ) : null}
    </AppShell>
  );
}

function AuditOverview({ summary, loading }: { summary: ComplianceSummary; loading: boolean }) {
  return (
    <div className="grid grid-cols-2 gap-4 xl:grid-cols-4">
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
        tooltip="Envelope count reflects successfully committed envelopes. Sessions closed by a contract violation may show zero if envelopes were never committed."
      />
      <StatCard
        label="Violations"
        value={loading ? '-' : formatInteger(summary.contractViolations)}
        icon={ShieldAlert}
        accent={summary.contractViolations > 0 ? 'danger' : 'neutral'}
        tooltip="Sessions that exceeded contract limits during this period"
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
  onResetFilters,
}: {
  timeRange: TimeRange;
  eventTypeFilter: AuditEventType | 'all' | 'contract_violations';
  workgroupFilter: string;
  accountFilter: string;
  workgroupOptions: FilterOption[];
  accountOptions: FilterOption[];
  reportOpen: boolean;
  filteredEvents: AuditEvent[];
  onTimeRangeChange: (range: TimeRange) => void;
  onEventTypeFilterChange: (eventType: AuditEventType | 'all' | 'contract_violations') => void;
  onWorkgroupFilterChange: (workgroupId: string) => void;
  onAccountFilterChange: (accountId: string) => void;
  onExportCSV: () => void;
  onToggleReport: () => void;
  onResetFilters?: () => void;
}) {
  return (
    <section className="rounded-card border border-border bg-panel p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center sm:gap-2">
        <div className="flex gap-2">
          {timeRangeOptions.map((option) => (
            <FilterButton
              key={option.id}
              active={timeRange === option.id}
              label={option.label}
              className="flex-1 sm:flex-none"
              onClick={() => onTimeRangeChange(option.id)}
            />
          ))}
        </div>

        <div className="relative sm:shrink-0">
          <Select
            value={eventTypeFilter}
            onChange={(event) => onEventTypeFilterChange(event.target.value as AuditEventType | 'all' | 'contract_violations')}
            className="w-full pr-7 sm:w-auto"
          >
            <option value="all">all events</option>
            {auditEventTypes.slice(0, 5).map((eventType) => (
              <option key={eventType} value={eventType}>
                {formatEventType(eventType)}
              </option>
            ))}
            <option value="contract_violations">Contract Violations</option>
            {auditEventTypes.slice(5).map((eventType) => (
              <option key={eventType} value={eventType}>
                {formatEventType(eventType)}
              </option>
            ))}
          </Select>
          <ChevronDown size={13} aria-hidden="true" className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-text-mute" />
        </div>

        <div className="relative sm:shrink-0">
          <Select
            value={workgroupFilter}
            onChange={(event) => onWorkgroupFilterChange(event.target.value)}
            className="w-full pr-7 sm:w-auto"
          >
            <option value="all">all workgroups</option>
            {workgroupOptions.map((option) => (
              <option key={option.id} value={option.id}>
                {option.label}
              </option>
            ))}
          </Select>
          <ChevronDown size={13} aria-hidden="true" className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-text-mute" />
        </div>

        <div className="relative sm:shrink-0">
          <Select
            value={accountFilter}
            onChange={(event) => onAccountFilterChange(event.target.value)}
            className="w-full pr-7 sm:w-auto"
          >
            <option value="all">all accounts</option>
            {accountOptions.map((option) => (
              <option key={option.id} value={option.id}>
                {option.label}
              </option>
            ))}
          </Select>
          <ChevronDown size={13} aria-hidden="true" className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-text-mute" />
        </div>

        {onResetFilters ? (
          <button
            type="button"
            onClick={onResetFilters}
            className="h-9 w-full rounded-pill border border-border bg-panel px-3 text-table font-medium text-text-mute-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora sm:w-auto sm:shrink-0"
          >
            Reset filters
          </button>
        ) : null}
        <div className="flex flex-col gap-2 sm:ml-auto sm:flex-row sm:items-center sm:gap-2">
          <Button
            variant="secondary"
            disabled={filteredEvents.length === 0}
            onClick={onExportCSV}
            className="w-full sm:w-auto"
          >
            <Download size={15} aria-hidden="true" />
            CSV
          </Button>
          <Button
            variant="primary"
            aria-pressed={reportOpen}
            onClick={onToggleReport}
            className="w-full sm:w-auto"
          >
            <FileCheck2 size={15} aria-hidden="true" />
            Compliance Report
          </Button>
        </div>
      </div>
    </section>
  );
}

function ComplianceReport({
  summary,
  timeRange,
  accountFilter,
  accountOptions,
}: {
  summary: ComplianceSummary;
  timeRange: TimeRange;
  accountFilter: string;
  accountOptions: FilterOption[];
}) {
  const generatedAt = useMemo(() => new Date(), []);
  const accountLabel =
    accountFilter === 'all'
      ? 'All Accounts'
      : (accountOptions.find((o) => o.id === accountFilter)?.label ?? accountFilter);

  return (
    <SectionPanel title={`Compliance Report — ${complianceRangeLabel(timeRange)}`}>
      <div className="mb-5 flex flex-col gap-1">
        <p className="text-table text-text-mute">{formatGeneratedAt(generatedAt)}</p>
        <p className="text-table text-text-mute">Account: {accountLabel}</p>
      </div>
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

function ActivityStrip({
  events,
  timeRange,
  workgroupNames,
  onSessionClick,
}: {
  events: AuditEvent[];
  timeRange: TimeRange;
  workgroupNames?: Record<string, string>;
  onSessionClick?: (sessionId: string) => void;
}) {
  const { lanePositions, violationTicks, minTime, maxTime } = useMemo(() => {
    if (events.length === 0) {
      return {
        lanePositions: LANE_DEFS.map(() => [] as TickMark[]),
        violationTicks: [] as ViolationTickMark[],
        minTime: 0,
        maxTime: 0,
      };
    }
    let minT = Infinity;
    let maxT = -Infinity;
    const rawByLane: number[][] = LANE_DEFS.map(() => []);
    const rawViolations: { t: number; event: AuditEvent }[] = [];
    for (const event of events) {
      const t = new Date(event.occurredAt).getTime();
      if (t < minT) minT = t;
      if (t > maxT) maxT = t;
      const lane = classifyEventToLane(event);
      if (lane === 1) {
        rawViolations.push({ t, event });
      } else if (lane >= 0) {
        rawByLane[lane]!.push(t);
      }
    }
    const span = maxT - minT || 1;
    const lanePositions = rawByLane.map((times) =>
      mergeTickPositions(times.map((t) => ((t - minT) / span) * 100)),
    );
    const violationPositions = rawViolations.map(({ t }) => ((t - minT) / span) * 100);
    const sortedViolationPositions = [...violationPositions].sort((a, b) => a - b);
    const violationTicks: ViolationTickMark[] = rawViolations.map(({ event }, idx) => ({
      position: violationPositions[idx]!,
      event,
      gap: nearestTickGap(sortedViolationPositions, violationPositions[idx]!),
    }));
    return { lanePositions, violationTicks, minTime: minT, maxTime: maxT };
  }, [events]);

  const timeLabels = useMemo(
    () => buildTimeAxisLabels(minTime, maxTime, timeRange),
    [minTime, maxTime, timeRange],
  );

  const [popover, setPopover] = useState<{ event: AuditEvent; tickIndex: number; rect: DOMRect } | null>(null);
  const popoverRef = useRef<HTMLDivElement | null>(null);

  // close the popover when the event set refreshes — the open popover references an
  // event that may no longer exist. Adjust during render instead of in an effect.
  const [popoverEvents, setPopoverEvents] = useState(events);
  if (popoverEvents !== events) {
    setPopoverEvents(events);
    setPopover(null);
  }

  useEffect(() => {
    if (!popover) return;
    function handleOutside(e: MouseEvent) {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setPopover(null);
      }
    }
    document.addEventListener('click', handleOutside);
    return () => document.removeEventListener('click', handleOutside);
  }, [popover]);

  useEffect(() => {
    if (!popover) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setPopover(null);
    }
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [popover]);

  if (events.length === 0) {
    return (
      <SectionPanel title="Activity">
        <div className="p-5">
          <EmptyState icon={Activity} title="No activity in this period" description="No events were recorded in the selected time range." />
        </div>
      </SectionPanel>
    );
  }

  return (
    <>
      <SectionPanel title="Activity">
        <div className="flex gap-4">
          {/* Legend */}
          <div style={{ width: LEGEND_WIDTH, flexShrink: 0 }}>
            {LANE_DEFS.map((lane, i) => (
              <div
                key={lane.label}
                className="flex items-center gap-1.5"
                style={{ height: ROW_HEIGHT, marginBottom: i < LANE_DEFS.length - 1 ? ROW_GAP : 0 }}
              >
                <div
                  style={{
                    width: 10,
                    height: 10,
                    borderRadius: 2,
                    backgroundColor: lane.cssColor,
                    flexShrink: 0,
                  }}
                />
                <span className="truncate text-table text-text-mute" title={lane.label}>{lane.label}</span>
              </div>
            ))}
          </div>

          {/* Swimlanes + time axis */}
          <div className="min-w-0 flex-1">
            {LANE_DEFS.map((lane, i) => {
              const ticks = lanePositions[i] ?? [];
              return (
                <div
                  key={lane.label}
                  className="relative overflow-hidden rounded-sm"
                  style={{
                    height: ROW_HEIGHT,
                    marginBottom: i < LANE_DEFS.length - 1 ? ROW_GAP : 0,
                    backgroundColor: `color-mix(in srgb, ${lane.cssColor} 15%, transparent)`,
                  }}
                >
                  {i === 1 ? (
                    violationTicks.map((vt, j) => (
                      <div
                        key={j}
                        className="group absolute top-0 h-full -translate-x-1/2 cursor-pointer"
                        style={{
                          left: `${vt.position}%`,
                          width: `min(10px, max(2px, ${vt.gap}%))`,
                        }}
                        onClick={(e) => {
                          e.stopPropagation();
                          const rect = e.currentTarget.getBoundingClientRect();
                          setPopover((prev) =>
                            prev?.tickIndex === j
                              ? null
                              : { event: vt.event, tickIndex: j, rect },
                          );
                        }}
                      >
                        <div
                          className="pointer-events-none absolute top-0 left-1/2 h-full w-0.5 -translate-x-1/2 rounded-full transition-[width,filter] duration-150 group-hover:w-1.5 group-hover:brightness-150 group-hover:saturate-150"
                          style={{ backgroundColor: lane.cssColor }}
                        />
                      </div>
                    ))
                  ) : (
                    ticks.map((tick, j) => (
                      <div
                        key={j}
                        className="absolute top-0 h-full"
                        style={{
                          left: `${tick.position}%`,
                          width: tick.width,
                          backgroundColor: lane.cssColor,
                          transform: 'translateX(-50%)',
                        }}
                      />
                    ))
                  )}
                </div>
              );
            })}

            {/* Time axis */}
            <div className="relative mt-2" style={{ height: 16 }}>
              {timeLabels.map((item, i) => (
                <span
                  key={i}
                  className="absolute text-label text-text-mute"
                  style={{
                    left: `${item.position}%`,
                    transform:
                      i === 0
                        ? 'none'
                        : i === timeLabels.length - 1
                          ? 'translateX(-100%)'
                          : 'translateX(-50%)',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {item.label}
                </span>
              ))}
            </div>
          </div>
        </div>
      </SectionPanel>
      {popover ? (
        <ViolationTooltip
          event={popover.event}
          rect={popover.rect}
          workgroupNames={workgroupNames ?? {}}
          onSessionClick={onSessionClick}
          containerRef={popoverRef}
        />
      ) : null}
    </>
  );
}

function ViolationTooltip({
  event,
  rect,
  workgroupNames,
  onSessionClick,
  containerRef,
}: {
  event: AuditEvent;
  rect: DOMRect;
  workgroupNames: Record<string, string>;
  onSessionClick?: (sessionId: string) => void;
  containerRef?: RefObject<HTMLDivElement | null>;
}) {
  const TOOLTIP_W = 260;
  const GAP = 6;
  const MARGIN = 8;
  const data = event.data ?? {};
  const closeDetail = dataString(data, 'close_detail');
  const workgroupName = event.workgroupId
    ? (workgroupNames[event.workgroupId] ?? event.workgroupId)
    : undefined;

  const renderBelow = rect.top - GAP < 130;
  const top = renderBelow ? rect.bottom + GAP : undefined;
  const bottom = renderBelow ? undefined : window.innerHeight - rect.top + GAP;
  const tickCenterX = rect.left + rect.width / 2;
  let left = tickCenterX - TOOLTIP_W / 2;
  left = Math.max(MARGIN, Math.min(left, window.innerWidth - TOOLTIP_W - MARGIN));
  const caretOffset = Math.max(8, Math.min(tickCenterX - left, TOOLTIP_W - 8));

  return (
    <div
      ref={containerRef}
      className="fixed z-[500] rounded-card border border-border bg-panel shadow-xl p-3"
      style={{ top, bottom, left, width: TOOLTIP_W }}
    >
      {!renderBelow ? (
        <>
          <div style={{ position: 'absolute', bottom: -8, left: caretOffset - 8, width: 0, height: 0, borderStyle: 'solid', borderWidth: '8px 8px 0 8px', borderColor: 'var(--color-border) transparent transparent transparent' }} />
          <div style={{ position: 'absolute', bottom: -7, left: caretOffset - 7, width: 0, height: 0, borderStyle: 'solid', borderWidth: '7px 7px 0 7px', borderColor: 'var(--color-panel) transparent transparent transparent' }} />
        </>
      ) : (
        <>
          <div style={{ position: 'absolute', top: -8, left: caretOffset - 8, width: 0, height: 0, borderStyle: 'solid', borderWidth: '0 8px 8px 8px', borderColor: 'transparent transparent var(--color-border) transparent' }} />
          <div style={{ position: 'absolute', top: -7, left: caretOffset - 7, width: 0, height: 0, borderStyle: 'solid', borderWidth: '0 7px 7px 7px', borderColor: 'transparent transparent var(--color-panel) transparent' }} />
        </>
      )}
      <p className="font-mono text-table text-text-mute-strong">{formatDateTime(event.occurredAt)}</p>
      {workgroupName ? (
        <p className="mt-1 text-table text-text-mute">
          Workgroup: <span className="font-medium text-text">{workgroupName}</span>
        </p>
      ) : null}
      {closeDetail ? (
        <p className="mt-1 text-table text-text-mute">
          Reason: <span className="text-text">{closeDetail}</span>
        </p>
      ) : null}
      {event.sessionId ? (
        <div className="mt-2">
          <ResourcePill
            label="session"
            value={event.sessionId}
            onClick={onSessionClick ? () => onSessionClick(event.sessionId!) : undefined}
          />
        </div>
      ) : null}
    </div>
  );
}

function AuditEventRow({
  event,
  workgroupNames = {},
  advertisementNames = {},
  onSessionClick,
}: {
  event: AuditEvent;
  workgroupNames?: Record<string, string>;
  advertisementNames?: Record<string, string>;
  onSessionClick?: (sessionId: string) => void;
}) {
  const [showJson, setShowJson] = useState(false);
  const data = event.data ?? {};
  const resourcePills = eventResources(event, workgroupNames, advertisementNames);
  const hasData = Object.keys(data).length > 0;
  const isContractViolation = event.eventType === 'session.closed' && dataString(data, 'close_reason') === 'contract_violation';

  const closeDetail = isContractViolation ? dataString(data, 'close_detail') : '';
  const hasDuration = isContractViolation && 'duration_seconds' in data;
  const durationSeconds = hasDuration ? dataNumber(data, 'duration_seconds') : 0;
  const consumerOrgId = isContractViolation ? dataString(data, 'consumer_organization_id') : '';
  const providerOrgId = isContractViolation ? dataString(data, 'provider_organization_id') : '';
  const hasViolationPills = hasDuration || Boolean(consumerOrgId) || Boolean(providerOrgId);

  return (
    <article className="grid gap-4 p-5 xl:grid-cols-[12rem_minmax(0,1fr)]">
      <div>
        <p className="font-mono text-table text-text-mute-strong">{formatDateTime(event.occurredAt)}</p>
        <p className="mt-1 text-table text-text-mute">{relativeTime(event.occurredAt)}</p>
        {event.accountId ? (
          <p className="mt-1 text-table text-text-mute">
            account: <span className="font-mono text-text-mute-strong">{event.accountId}</span>
          </p>
        ) : null}
        <div className="mt-2">
          <StatusPill status={eventStatus(event)} label={isContractViolation ? 'Contract Violation' : formatEventType(event.eventType)} />
        </div>
      </div>
      <div className="min-w-0">
        <h2 className="text-body font-semibold text-text">{eventTitle(event)}</h2>
        {isContractViolation && closeDetail ? (
          <p className="mt-1 text-table text-text-mute">Reason: {closeDetail}</p>
        ) : null}
        {(resourcePills.length > 0 || hasViolationPills) ? (
          <div className="mt-3 flex flex-wrap gap-2">
            {resourcePills.map((resource) => (
              <ResourcePill
                key={`${resource.label}:${resource.value}`}
                label={resource.label}
                value={resource.value}
                onClick={
                  resource.label === 'session' && onSessionClick && resource.value
                    ? () => onSessionClick(resource.value)
                    : undefined
                }
              />
            ))}
            {hasDuration ? <ResourcePill label="Duration" value={`${durationSeconds}s`} /> : null}
            {consumerOrgId ? <ResourcePill label="consumer" value={consumerOrgId} /> : null}
            {providerOrgId ? <ResourcePill label="provider" value={providerOrgId} /> : null}
          </div>
        ) : null}
        {hasData ? (
          <div className="mt-3">
            <button
              type="button"
              className="inline-flex items-center gap-1 rounded-pill text-label font-medium text-text-mute hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
              onClick={() => setShowJson((v) => !v)}
            >
              {showJson ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
              {showJson ? 'Hide raw JSON' : 'Show raw JSON'}
            </button>
            {showJson ? (
              <pre className="mt-2 max-h-64 overflow-auto rounded-card border border-border bg-panel-subtle p-4 font-mono text-table text-text-mute-strong">
                {JSON.stringify(data, null, 2)}
              </pre>
            ) : null}
          </div>
        ) : null}
      </div>
    </article>
  );
}

function ResourcePill({
  label,
  value,
  muted = false,
  onClick,
}: {
  label: string;
  value?: string;
  muted?: boolean;
  onClick?: () => void;
}) {
  const pillClass = 'inline-flex max-w-full items-center gap-1 rounded-status border border-border bg-panel-subtle px-2 py-1 text-table text-text-mute-strong';
  const inner = (
    <>
      {value ? <span className="text-text-mute">{label}</span> : null}
      <span className={['truncate font-mono', muted ? 'text-text-mute' : ''].filter(Boolean).join(' ')}>
        {value ?? label}
      </span>
      {onClick ? <ChevronRight size={11} className="shrink-0 text-text-mute" aria-hidden="true" /> : null}
    </>
  );

  if (onClick) {
    return (
      <button
        type="button"
        className={[pillClass, 'cursor-pointer hover:border-brand-agora/50 hover:bg-panel focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora'].join(' ')}
        onClick={onClick}
      >
        {inner}
      </button>
    );
  }

  return <span className={pillClass}>{inner}</span>;
}


function FilterButton({ active, label, onClick, className }: { active: boolean; label: string; onClick: () => void; className?: string }) {
  return (
    <button
      type="button"
      className={[
        'h-9 rounded-pill border px-3 text-table font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
        active
          ? 'border-brand-agora bg-brand-agora/10 text-brand-agora'
          : 'border-border bg-panel-subtle text-text-mute-strong hover:bg-panel',
        className,
      ].filter(Boolean).join(' ')}
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
  eventTypeFilter: AuditEventType | 'all' | 'contract_violations',
  workgroupFilter: string,
  accountFilter: string,
): AuditEvent[] {
  return events.filter((event) => {
    if (eventTypeFilter === 'contract_violations') {
      if (event.eventType !== 'session.closed' || dataString(event.data ?? {}, 'close_reason') !== 'contract_violation') {
        return false;
      }
    } else if (eventTypeFilter !== 'all' && event.eventType !== eventTypeFilter) {
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

function buildIDOptions(
  events: AuditEvent[],
  select: (event: AuditEvent) => string | undefined,
  nameMap: Record<string, string> = {},
): FilterOption[] {
  return Array.from(new Set(events.map(select).filter((value): value is string => Boolean(value))))
    .sort((a, b) => a.localeCompare(b))
    .map((id) => ({ id, label: nameMap[id] ?? id }));
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
      .map(([eventType, count]) => ({ id: eventType, label: formatEventType(eventType), count }))
      .sort((left, right) => right.count - left.count || left.label.localeCompare(right.label)),
  };
}

function eventResources(
  event: AuditEvent,
  workgroupNames: Record<string, string> = {},
  advertisementNames: Record<string, string> = {},
): ResourceOption[] {
  return [
    event.workgroupId
      ? { label: 'workgroup', value: workgroupNames[event.workgroupId] ?? event.workgroupId }
      : undefined,
    event.sessionId ? { label: 'session', value: event.sessionId, muted: true } : undefined,
    event.advertisementId
      ? { label: 'advertisement', value: advertisementNames[event.advertisementId] ?? event.advertisementId }
      : undefined,
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

function complianceRangeLabel(timeRange: TimeRange): string {
  switch (timeRange) {
    case '24h': return 'Last 24 hours';
    case '7d': return 'Last 7 days';
    case '30d': return 'Last 30 days';
  }
}

function formatGeneratedAt(date: Date): string {
  return `Generated ${new Intl.DateTimeFormat(undefined, {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(date)}`;
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

function classifyEventToLane(event: AuditEvent): number {
  switch (event.eventType) {
    case 'session.proposed':
    case 'session.accepted':
    case 'session.rejected':
      return 0;
    case 'session.closed':
      return dataString(event.data ?? {}, 'close_reason') === 'contract_violation' ? 1 : 0;
    case 'envelope.flowed':
      return 2;
    case 'tunnel.attached':
    case 'tunnel.detached':
      return 3;
    case 'advertisement.published':
    case 'advertisement.retracted':
      return 4;
    case 'account.login':
    case 'account.login_failed':
    case 'account.logout':
    case 'environment.heartbeat':
      return 5;
    default:
      return -1;
  }
}

// nearestTickGap returns the distance (in the same percent units as tick
// positions) to the closest other violation tick, used to cap a tick's hit
// area so it never extends far enough to cover a neighbor. Falls back to 100
// (effectively uncapped) when a tick has no distinct neighbor.
function nearestTickGap(sortedPositions: number[], position: number): number {
  let nearest = Infinity;
  for (const p of sortedPositions) {
    const distance = Math.abs(p - position);
    if (distance > 0 && distance < nearest) nearest = distance;
  }
  return Number.isFinite(nearest) ? nearest : 100;
}

function mergeTickPositions(positions: number[]): TickMark[] {
  if (positions.length === 0) return [];
  const sorted = [...positions].sort((a, b) => a - b);
  const groups: number[][] = [[sorted[0]!]];
  for (let i = 1; i < sorted.length; i++) {
    const group = groups[groups.length - 1]!;
    if (sorted[i]! - group[0]! <= 1) {
      group.push(sorted[i]!);
    } else {
      groups.push([sorted[i]!]);
    }
  }
  return groups.map((group) => ({
    position: group.reduce((sum, p) => sum + p, 0) / group.length,
    width: group.length > 1 ? 4 : 2,
  }));
}

function buildTimeAxisLabels(
  minTime: number,
  maxTime: number,
  timeRange: TimeRange,
): { position: number; label: string }[] {
  if (minTime === 0 && maxTime === 0) return [];
  if (minTime === maxTime) {
    return [{ position: 50, label: formatAxisTime(new Date(minTime), timeRange) }];
  }
  const count = 4;
  return Array.from({ length: count }, (_, i) => ({
    position: (i / (count - 1)) * 100,
    label: formatAxisTime(new Date(minTime + (maxTime - minTime) * (i / (count - 1))), timeRange),
  }));
}

function formatAxisTime(date: Date, timeRange: TimeRange): string {
  switch (timeRange) {
    case '24h':
      return new Intl.DateTimeFormat(undefined, { hour: 'numeric' }).format(date);
    case '7d':
      return new Intl.DateTimeFormat(undefined, { weekday: 'short' }).format(date);
    case '30d':
      return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(date);
  }
}

function formatInteger(value: number): string {
  return numberFormatter.format(value);
}
