import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from 'react';

import { useResizableDrawer } from '../hooks/useResizableDrawer';
import { useNavigate } from 'react-router';
import {
  AlertTriangle,
  ArrowLeftRight,
  BookOpen,
  Boxes,
  Building2,
  ChevronLeft,
  ChevronRight,
  Clipboard,
  GripVertical,
  Info,
  RefreshCcw,
  ShieldCheck,
  Users,
  Wifi,
  X,
  Zap,
} from 'lucide-react';

import {
  AppShell,
  DataTable,
  DrawerCard,
  DrawerDivider,
  DrawerTip,
  EmptyState,
  InfoDrawer,
  InfoTooltip,
  Pagination,
  SectionPanel,
  SessionTrace,
  StatusPill,
  type DataTableColumn,
  type StatusPillStatus,
} from '../components';
import {
  ApiError,
  fetchAllAuditEvents,
  fetchAllVisibleAdvertisements,
  getDashboardSummary,
  getSession,
  getWorkgroupsActivity,
  listAuditEvents,
  listWorkgroupMembers,
  listWorkgroups,
  useApiResource,
  type Advertisement,
  type AuditEvent,
  type Session,
  type Workgroup,
  type WorkgroupMembership,
  type WorkgroupMembershipRole,
} from '../lib/api';

type MemberWithWorkgroups = {
  accountId: string;
  organizationId: string;
  email?: string;
  label: string;
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
  sessionCount?: number;
  violationRate?: number;
  avgSessionDuration?: number;
  metricsLoading?: boolean;
  activityEvents?: AuditEvent[];
  activityLoading?: boolean;
};

type WorkgroupMetrics = {
  sessionCount: number;
  violationRate: number;
  avgSessionDuration?: number;
  loading: boolean;
};

type WorkgroupActivityData = {
  events: AuditEvent[];
  loading: boolean;
};

type TickMark = { position: number; width: number };
type ViolationTickMark = { position: number; event: AuditEvent; gap: number };

type TimeWindow = '24h' | '7d' | '30d';

const timeWindowOptions: { id: TimeWindow; label: string; durationMs: number }[] = [
  { id: '24h', label: '24 hours', durationMs: 24 * 60 * 60 * 1000 },
  { id: '7d', label: '7 days', durationMs: 7 * 24 * 60 * 60 * 1000 },
  { id: '30d', label: '30 days', durationMs: 30 * 24 * 60 * 60 * 1000 },
];

const routeByTab: Record<string, string> = {
  dashboard: '/',
  sessions: '/sessions',
  workgroups: '/workgroups',
  catalog: '/catalog',
  contracts: '/contracts',
  audit: '/audit',
};

const numberFormatter = new Intl.NumberFormat();

const WG_STRIP_LANE_HEIGHT = 6;
const WG_STRIP_LANE_GAP = 2;

const WG_LANE_DEFS = [
  { label: 'Sessions',   cssColor: 'var(--color-info)' },
  { label: 'Violations', cssColor: 'var(--color-danger)' },
  { label: 'Envelopes',  cssColor: 'var(--color-success)' },
  { label: 'Tunnels',    cssColor: 'var(--color-warning)' },
];

export default function Workgroups() {
  const navigate = useNavigate();
  const [infoOpen, setInfoOpen] = useState(false);
  const [selectedWorkgroupId, setSelectedWorkgroupId] = useState<string>();
  const [timeWindow, setTimeWindow] = useState<TimeWindow>('24h');
  const [currentPage, setCurrentPage] = useState(1);
  const [itemsPerPage, setItemsPerPage] = useState(10);
  const [membersCurrentPage, setMembersCurrentPage] = useState(1);
  const [membersItemsPerPage, setMembersItemsPerPage] = useState(10);
  const account = useApiResource(getDashboardSummary);
  const workgroups = useApiResource(listWorkgroups);
  const workgroupsActivityLoad = useCallback(
    (signal: AbortSignal) => getWorkgroupsActivity({ window: timeWindow }, signal),
    [timeWindow],
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
  const [workgroupMetrics, setWorkgroupMetrics] = useState<Map<string, WorkgroupMetrics>>(new Map());
  const [workgroupActivity, setWorkgroupActivity] = useState<Map<string, WorkgroupActivityData>>(new Map());
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
        const metrics = workgroupMetrics.get(workgroup.id);
        const activity = workgroupActivity.get(workgroup.id);

        return {
          workgroup,
          memberCount: workgroupMembers.length,
          callerRole: callerMembership?.role,
          envelopes24h: activityByWorkgroup.get(workgroup.id) ?? 0,
          advertisements: adsByWorkgroup.get(workgroup.id) ?? [],
          sessionCount: metrics?.sessionCount,
          violationRate: metrics?.violationRate,
          avgSessionDuration: metrics?.avgSessionDuration,
          metricsLoading: metrics?.loading,
          activityEvents: activity?.events,
          activityLoading: activity?.loading,
        };
      }),
    [activityByWorkgroup, adsByWorkgroup, callerAccount?.accountId, membersByWorkgroup, workgroups.data, workgroupMetrics, workgroupActivity],
  );
  const memberRows = useMemo(
    () => buildMemberRows(members.data ?? [], workgroupNameById),
    [members.data, workgroupNameById],
  );
  // clamp the page during render so a shrinking list never strands us past the last page
  const cardTotalPages = Math.max(1, Math.ceil(cardModels.length / itemsPerPage));
  const safeCurrentPage = Math.min(currentPage, cardTotalPages);
  const membersTotalPages = Math.max(1, Math.ceil(memberRows.length / membersItemsPerPage));
  const safeMembersCurrentPage = Math.min(membersCurrentPage, membersTotalPages);
  const paginatedCardModels = useMemo(() => {
    const start = (safeCurrentPage - 1) * itemsPerPage;
    return cardModels.slice(start, start + itemsPerPage);
  }, [cardModels, safeCurrentPage, itemsPerPage]);

  const paginatedMemberRows = useMemo(() => {
    const start = (safeMembersCurrentPage - 1) * membersItemsPerPage;
    return memberRows.slice(start, start + membersItemsPerPage);
  }, [memberRows, safeMembersCurrentPage, membersItemsPerPage]);

  const selectedWorkgroup = cardModels.find((card) => card.workgroup.id === selectedWorkgroupId);

  // session.closed metrics — one listAuditEvents call per workgroup, filtered to session.closed only
  useEffect(() => {
    const wgs = workgroups.data;
    if (!wgs || wgs.length === 0) return;

    let cancelled = false;

    // eslint-disable-next-line react-hooks/set-state-in-effect -- seeding loading-state before async fetch; correct effect usage
    setWorkgroupMetrics(
      new Map(wgs.map((wg) => [wg.id, { sessionCount: 0, violationRate: 0, loading: true }])),
    );

    const durationMs = timeWindowOptions.find((o) => o.id === timeWindow)?.durationMs ?? 24 * 60 * 60 * 1000;
    const toDate = new Date();
    const fromDate = new Date(toDate.getTime() - durationMs);
    const fromStr = fromDate.toISOString();
    const toStr = toDate.toISOString();

    wgs.forEach((workgroup) => {
      listAuditEvents({
        workgroupId: workgroup.id,
        eventTypes: ['session.closed'],
        from: fromStr,
        to: toStr,
      })
        .then((response) => {
          if (cancelled) return;
          const events = response.items;
          const sessionCount = events.length;
          const violations = events.filter(
            (e) => typeof e.data['close_reason'] === 'string' && e.data['close_reason'] === 'contract_violation',
          ).length;
          const violationRate =
            sessionCount > 0 ? Math.round((violations / sessionCount) * 1000) / 10 : 0;
          const durations = events
            .map((e) => (typeof e.data['duration_seconds'] === 'number' ? (e.data['duration_seconds'] as number) : undefined))
            .filter((d): d is number => d !== undefined);
          const avgSessionDuration =
            durations.length > 0
              ? durations.reduce((sum, d) => sum + d, 0) / durations.length
              : undefined;

          setWorkgroupMetrics((prev) => {
            const next = new Map(prev);
            next.set(workgroup.id, { sessionCount, violationRate, avgSessionDuration, loading: false });
            return next;
          });
        })
        .catch(() => {
          if (cancelled) return;
          setWorkgroupMetrics((prev) => {
            const next = new Map(prev);
            next.set(workgroup.id, { sessionCount: 0, violationRate: 0, loading: false });
            return next;
          });
        });
    });

    return () => {
      cancelled = true;
    };
  }, [workgroups.data, timeWindow]);

  // activity strip events — single org-wide fetch, filtered client-side per workgroup
  useEffect(() => {
    const wgs = workgroups.data;
    if (!wgs || wgs.length === 0) return;

    const controller = new AbortController();

    // eslint-disable-next-line react-hooks/set-state-in-effect -- seeding loading-state before async fetch; correct effect usage
    setWorkgroupActivity(
      new Map(wgs.map((wg) => [wg.id, { events: [], loading: true }])),
    );

    const durationMs = timeWindowOptions.find((o) => o.id === timeWindow)?.durationMs ?? 24 * 60 * 60 * 1000;
    const toDate = new Date();
    const fromDate = new Date(toDate.getTime() - durationMs);

    fetchAllAuditEvents(
      {
        from: fromDate.toISOString(),
        to: toDate.toISOString(),
        eventTypes: [
          'session.proposed',
          'session.accepted',
          'session.rejected',
          'session.closed',
          'envelope.flowed',
          'tunnel.attached',
          'tunnel.detached',
        ],
      },
      controller.signal,
    )
      .then((events) => {
        // Build sessionId → workgroupId map from session events as a fallback
        // for event types (envelope.flowed, tunnel.*) that may only carry sessionId
        const sessionToWorkgroup = new Map<string, string>();
        for (const event of events) {
          if (event.sessionId && event.workgroupId && event.eventType.startsWith('session.')) {
            sessionToWorkgroup.set(event.sessionId, event.workgroupId);
          }
        }
        setWorkgroupActivity(
          new Map(
            wgs.map((wg) => [
              wg.id,
              {
                events: events.filter((e) => {
                  const wgId = e.workgroupId ?? (e.sessionId ? sessionToWorkgroup.get(e.sessionId) : undefined);
                  return wgId === wg.id;
                }),
                loading: false,
              },
            ]),
          ),
        );
      })
      .catch((err) => {
        if (controller.signal.aborted) return;
        console.error('Failed to load workgroup activity', err);
        setWorkgroupActivity(
          new Map(wgs.map((wg) => [wg.id, { events: [], loading: false }])),
        );
      });

    return () => {
      controller.abort();
    };
  }, [workgroups.data, timeWindow]);

  const workgroupTableColumns = useMemo(
    (): DataTableColumn<WorkgroupCardModel>[] => [
      {
        id: 'name',
        header: 'Name',
        accessor: (row) => <span className="font-semibold text-text">{row.workgroup.name}</span>,
        sortable: true,
        sortValue: (row) => row.workgroup.name,
      },
      {
        id: 'scope',
        header: 'Scope',
        accessor: (row) => <ScopePill scope={row.workgroup.scope} />,
      },
      {
        id: 'members',
        header: 'Members',
        accessor: (row) => formatInteger(row.memberCount),
        sortable: true,
        sortValue: (row) => row.memberCount,
      },
      {
        id: 'role',
        header: 'My Role',
        accessor: (row) => row.callerRole ?? '-',
        sortable: true,
      },
      {
        id: 'envelopes24h',
        header: 'Envelopes',
        accessor: (row) => formatInteger(row.envelopes24h),
        sortable: true,
        sortValue: (row) => row.envelopes24h,
      },
      {
        id: 'sessions',
        header: 'Sessions',
        accessor: (row) => row.metricsLoading ? <MetricSkeleton /> : formatInteger(row.sessionCount ?? 0),
        sortable: true,
        sortValue: (row) => row.sessionCount ?? 0,
      },
      {
        id: 'violationRate',
        header: 'Violation Rate',
        accessor: (row) =>
          row.metricsLoading ? <MetricSkeleton /> : `${(row.violationRate ?? 0).toFixed(1)}%`,
        sortable: true,
        sortValue: (row) => row.violationRate ?? 0,
      },
      {
        id: 'avgDuration',
        header: 'Avg Duration',
        accessor: (row) =>
          row.metricsLoading
            ? <MetricSkeleton />
            : row.avgSessionDuration !== undefined
              ? formatSessionSeconds(row.avgSessionDuration)
              : '—',
        sortable: true,
        sortValue: (row) => row.avgSessionDuration ?? 0,
      },
      {
        id: 'advertisements',
        header: 'Advertisements',
        accessor: (row) => <AdvertisementTags advertisements={row.advertisements} />,
      },
      {
        id: 'activity',
        header: 'Activity',
        accessor: (row) => (
          <WorkgroupActivityStrip
            events={row.activityEvents ?? []}
            loading={row.activityLoading ?? false}
            timeWindow={timeWindow}
            workgroupNameById={workgroupNameById}
          />
        ),
      },
      {
        id: 'action',
        header: '',
        accessor: () => <ChevronRight size={15} className="text-text-mute" aria-hidden="true" />,
        align: 'right',
      },
    ],
    [timeWindow, workgroupNameById],
  );

  function handleTabChange(tabId: string) {
    const route = routeByTab[tabId];

    if (route) {
      navigate(route);
    }
  }

  const workgroupsPanel = (
    <SectionPanel title="Workgroups" bodyClassName="p-0">
      {!callerAccount || workgroups.loading || members.loading || workgroupsActivity.loading || advertisements.loading ? (
        <div className="p-5">
          <LoadingPanel title="Loading workgroups" compact />
        </div>
      ) : workgroups.error || workgroupsActivity.error || members.error || advertisements.error ? (
        <div className="p-5">
          <ErrorPanel
            title="Workgroup data unavailable"
            error={workgroups.error ?? workgroupsActivity.error ?? members.error ?? advertisements.error}
            onRetry={() => {
              workgroups.refetch();
              workgroupsActivity.refetch();
              members.refetch();
              advertisements.refetch();
            }}
            compact
          />
        </div>
      ) : (
        <>
          <DataTable
            columns={workgroupTableColumns}
            rows={paginatedCardModels}
            getRowKey={(row) => row.workgroup.id}
            onRowClick={(row) => setSelectedWorkgroupId(row.workgroup.id)}
            className="rounded-none border-0"
            emptyState={
              <div className="p-5">
                <EmptyState
                  icon={Boxes}
                  title="No workgroups"
                  description="No workgroups are visible to the current account."
                />
              </div>
            }
          />
          <div className="border-t border-border">
            <Pagination
              totalItems={cardModels.length}
              itemsPerPage={itemsPerPage}
              currentPage={safeCurrentPage}
              onPageChange={setCurrentPage}
              onItemsPerPageChange={(count) => {
                setItemsPerPage(count);
                setCurrentPage(1);
              }}
            />
          </div>
        </>
      )}
    </SectionPanel>
  );

  const membersPanel = (
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
        <>
          <DataTable
            columns={memberColumns}
            rows={paginatedMemberRows}
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
          <div className="border-t border-border">
            <Pagination
              totalItems={memberRows.length}
              itemsPerPage={membersItemsPerPage}
              currentPage={safeMembersCurrentPage}
              onPageChange={setMembersCurrentPage}
              onItemsPerPageChange={(count) => {
                setMembersItemsPerPage(count);
                setMembersCurrentPage(1);
              }}
            />
          </div>
        </>
      )}
    </SectionPanel>
  );

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
          onInfoClick={() => setInfoOpen(true)}
        />

        <div className="flex gap-2">
          {timeWindowOptions.map((option) => (
            <FilterButton
              key={option.id}
              active={timeWindow === option.id}
              label={option.label}
              onClick={() => setTimeWindow(option.id)}
            />
          ))}
        </div>

        {workgroupsPanel}
        {membersPanel}
      </div>

      {selectedWorkgroup ? (
        <WorkgroupDrawer
          card={selectedWorkgroup}
          timeWindow={timeWindow}
          workgroupNameById={workgroupNameById}
          onClose={() => setSelectedWorkgroupId(undefined)}
        />
      ) : null}

      {infoOpen ? (
        <InfoDrawer title="About Workgroups" onClose={() => setInfoOpen(false)}>
          <div className="flex flex-col gap-5">
            <section>
              <h3 className="mb-2 font-semibold text-text">What is a Workgroup?</h3>
              <p className="leading-relaxed text-text-mute">
                Workgroups are the primary policy boundary in Agora. They control visibility and
                interaction scope at the discovery layer. An agent outside a workgroup cannot discover
                agents inside it, cannot query their capabilities, and has zero knowledge of their
                existence — this is not a filtered view, it is structural invisibility enforced by the controller.
              </p>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-3 font-semibold text-text">Intra-org vs. Inter-org</h3>
              <div className="flex flex-col gap-2">
                <DrawerCard icon={Building2} title="Intra-Organization" description="Define internal team boundaries within a single organization. Members share resources and visibility within their org." />
                <DrawerCard icon={ArrowLeftRight} title="Inter-Organization" description="Enable cross-company collaboration channels, established via explicit invitation handshakes. Each organization independently manages its own members. No cross-org admin authority." />
              </div>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-3 font-semibold text-text">What workgroup membership controls</h3>
              <div className="flex flex-col gap-2">
                <DrawerCard icon={BookOpen} title="Catalog Visibility" description="Controls which advertisements appear in discovery queries. Agents outside the workgroup cannot see advertisements scoped to it." />
                <DrawerCard icon={ArrowLeftRight} title="Session Proposals" description="Controls which agents can propose sessions to which other agents. Only members with shared workgroup access can initiate a session." />
                <DrawerCard icon={Zap} title="Active Session Continuity" description="If an agent loses workgroup membership, active sessions with members of that workgroup are closed immediately." />
              </div>
            </section>

            <DrawerDivider />

            <DrawerTip>
              When an agent loses workgroup access, advertisements vanish from its catalog view instantly. Active sessions with members of that workgroup are closed immediately with a recorded close reason.
            </DrawerTip>
          </div>
        </InfoDrawer>
      ) : null}
    </AppShell>
  );
}

function StructuralSecurityPanel({
  memberCount,
  workgroupCount,
  loading,
  onInfoClick,
}: {
  memberCount: number;
  workgroupCount: number;
  loading: boolean;
  onInfoClick: () => void;
}) {
  return (
    <section className="rounded-card border border-border bg-panel p-4">
      <div className="grid gap-4 lg:grid-cols-[1fr_auto]">
        <div className="flex items-start gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-[7px] bg-brand-agora/10 text-brand-agora">
            <ShieldCheck size={18} aria-hidden="true" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-[0.6875rem] font-semibold uppercase tracking-[0.04em] text-text-mute-strong">Structural Security</p>
            <h1 className="mt-0.5 text-xl font-bold text-text">Workgroups</h1>
            <p className="mt-1 text-body leading-relaxed text-text-mute">
              Policy boundaries that control who can see whom — agents outside a workgroup cannot discover, interact with, or even detect the agents inside it.
            </p>
          </div>
          <button
            type="button"
            className="mt-0.5 flex shrink-0 items-center gap-1 text-[0.76rem] text-text-mute-2 transition-colors hover:text-brand-agora focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            onClick={onInfoClick}
            aria-label="Learn more about Workgroups"
          >
            <Info size={13} aria-hidden="true" />
            <span>Learn more</span>
          </button>
        </div>

        <div className="grid min-w-72 gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <SecurityMetric label="members" value={loading ? '-' : formatInteger(memberCount)} />
          <SecurityMetric label="workgroups" value={loading ? '-' : formatInteger(workgroupCount)} />
          <div className="flex flex-col justify-between rounded-card border border-border bg-panel-subtle p-3">
            <p className="text-label font-medium uppercase text-text-mute">identity-bound</p>
            <StatusPill status="success" label="100%" className="mt-2 self-start" />
          </div>
          <div className="flex flex-col justify-between rounded-card border border-border bg-panel-subtle p-3">
            <p className="text-label font-medium uppercase text-text-mute">posture</p>
            <div className="mt-2 flex items-center gap-1">
              <StatusPill status="success" label="Zero-Trust Active" />
              <InfoTooltip content="Zero Trust Active: Workgroups enforce explicit identity binding and contract terms on every session. Agents outside a workgroup cannot discover or reach agents inside it." ariaLabel="About Zero Trust posture" />
            </div>
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

function FilterButton({
  active,
  label,
  onClick,
  className,
}: {
  active: boolean;
  label: string;
  onClick: () => void;
  className?: string;
}) {
  return (
    <button
      type="button"
      className={[
        'h-9 rounded-pill border px-3 text-table font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
        active
          ? 'border-brand-agora bg-brand-agora/10 text-brand-agora'
          : 'border-border bg-panel-subtle text-text-mute-strong hover:bg-panel',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
      aria-pressed={active}
      onClick={onClick}
    >
      {label}
    </button>
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
    accessor: (row) => row.role,
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
    <div className="min-w-64">
      <p className="truncate text-table font-medium text-text">{member.label}</p>
      <p className="truncate text-table text-text-mute">{member.accountId}</p>
    </div>
  );
}

function AdvertisementTags({ advertisements }: { advertisements: Advertisement[] }) {
  if (advertisements.length === 0) {
    return <span className="text-text-mute">—</span>;
  }

  if (advertisements.length > 3) {
    return <span className="text-table text-text-mute-strong">{advertisements.length} ads</span>;
  }

  return (
    <span className="flex flex-wrap gap-1">
      {advertisements.map((ad) => (
        <span
          key={ad.id}
          className={[
            'inline-flex max-w-full items-center rounded-status border px-2 py-0.5 text-table font-medium',
            advertisementAccent(ad),
          ].join(' ')}
        >
          <span className="truncate">{ad.name}</span>
        </span>
      ))}
    </span>
  );
}

function WorkgroupActivityStrip({
  events,
  loading,
  timeWindow,
  workgroupNameById,
  onSessionClick,
  fillWidth = false,
  showLabels = false,
}: {
  events: AuditEvent[];
  loading: boolean;
  timeWindow: TimeWindow;
  workgroupNameById?: Map<string, string>;
  onSessionClick?: (sessionId: string) => void;
  fillWidth?: boolean;
  showLabels?: boolean;
}) {
  const { lanePositions, violationTicks, minTime, maxTime } = useMemo(() => {
    if (events.length === 0) {
      return {
        lanePositions: WG_LANE_DEFS.map(() => [] as TickMark[]),
        violationTicks: [] as ViolationTickMark[],
        minTime: 0,
        maxTime: 0,
      };
    }
    let minT = Infinity;
    let maxT = -Infinity;
    const rawByLane: number[][] = WG_LANE_DEFS.map(() => []);
    const rawViolations: { t: number; event: AuditEvent }[] = [];
    for (const event of events) {
      const t = new Date(event.occurredAt).getTime();
      if (t < minT) minT = t;
      if (t > maxT) maxT = t;
      const lane = classifyWorkgroupEventToLane(event);
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

  const timeLabels = useMemo(
    () => buildWgTimeLabels(minTime, maxTime, timeWindow),
    [minTime, maxTime, timeWindow],
  );

  const totalLanesHeight =
    WG_LANE_DEFS.length * WG_STRIP_LANE_HEIGHT + (WG_LANE_DEFS.length - 1) * WG_STRIP_LANE_GAP;

  if (loading) {
    return (
      <div
        style={fillWidth ? { height: totalLanesHeight + 20 } : { width: 200, height: totalLanesHeight + 20 }}
        className={['animate-pulse rounded bg-panel-subtle', fillWidth ? 'w-full' : ''].join(' ')}
        aria-hidden="true"
      />
    );
  }

  if (events.length === 0 && !showLabels) {
    return (
      <div
        style={fillWidth ? { height: totalLanesHeight } : { width: 200, height: totalLanesHeight }}
        className={['flex items-center', fillWidth ? 'w-full' : ''].join(' ')}
      >
        <span className="text-label text-text-mute">No activity</span>
      </div>
    );
  }

  const LABEL_W = 80;
  const LABEL_GAP = 8;
  const timeAxisOffset = showLabels ? LABEL_W + LABEL_GAP : 0;

  return (
    <>
      <div style={fillWidth || showLabels ? {} : { width: 200 }} className={fillWidth || showLabels ? 'w-full' : ''}>
        {WG_LANE_DEFS.map((lane, i) => {
          const ticks = lanePositions[i] ?? [];
          const marginBottom = i < WG_LANE_DEFS.length - 1 ? WG_STRIP_LANE_GAP : 0;
          const laneBar = (
            <div
              className={['relative overflow-hidden rounded-sm', showLabels ? 'flex-1' : ''].join(' ')}
              style={{
                height: WG_STRIP_LANE_HEIGHT,
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

          if (showLabels) {
            return (
              <div
                key={lane.label}
                className="flex items-center"
                style={{ gap: LABEL_GAP, marginBottom }}
              >
                <span
                  className="shrink-0 text-right text-label text-text-mute"
                  style={{ width: LABEL_W }}
                >
                  {lane.label}
                </span>
                {laneBar}
              </div>
            );
          }

          return (
            <div key={lane.label} style={{ marginBottom }}>
              {laneBar}
            </div>
          );
        })}
        <div className="relative mt-1" style={{ height: 12, marginLeft: timeAxisOffset }}>
          {events.length === 0 && showLabels ? (
            <span className="text-label text-text-mute">No activity</span>
          ) : (
            timeLabels.map((item, i) => (
              <span
                key={i}
                className="absolute text-[10px] leading-none text-text-mute"
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
            ))
          )}
        </div>
      </div>
      {popover ? (
        <WgViolationTooltip
          event={popover.event}
          rect={popover.rect}
          workgroupNameById={workgroupNameById}
          onSessionClick={onSessionClick}
          containerRef={popoverRef}
        />
      ) : null}
    </>
  );
}

function WgViolationTooltip({
  event,
  rect,
  workgroupNameById,
  onSessionClick,
  containerRef,
}: {
  event: AuditEvent;
  rect: DOMRect;
  workgroupNameById?: Map<string, string>;
  onSessionClick?: (sessionId: string) => void;
  containerRef?: RefObject<HTMLDivElement | null>;
}) {
  const TOOLTIP_W = 260;
  const GAP = 6;
  const MARGIN = 8;
  const data = event.data ?? {};
  const closeDetail = wgDataString(data, 'close_detail');
  const workgroupName = event.workgroupId
    ? (workgroupNameById?.get(event.workgroupId) ?? event.workgroupId)
    : undefined;

  const renderBelow = rect.top - GAP < 130;
  const top = renderBelow ? rect.bottom + GAP : undefined;
  const bottom = renderBelow ? undefined : window.innerHeight - rect.top + GAP;
  const tickCenterX = rect.left + rect.width / 2;
  let left = tickCenterX - TOOLTIP_W / 2;
  left = Math.max(MARGIN, Math.min(left, window.innerWidth - TOOLTIP_W - MARGIN));
  const caretOffset = Math.max(8, Math.min(tickCenterX - left, TOOLTIP_W - 8));

  const pillClass = 'inline-flex max-w-full items-center gap-1 rounded-status border border-border bg-panel-subtle px-2 py-1 text-table text-text-mute-strong';

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
      <p className="font-mono text-table text-text-mute-strong">{wgFormatDateTime(event.occurredAt)}</p>
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
          {event.sessionId.startsWith('sess_seed_') ? (
            <span className={pillClass}>
              <span className="truncate font-mono text-text-mute">session unavailable</span>
            </span>
          ) : onSessionClick ? (
            <button
              type="button"
              className={[pillClass, 'cursor-pointer hover:border-brand-agora/50 hover:bg-panel focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora'].join(' ')}
              onClick={() => onSessionClick(event.sessionId!)}
            >
              <span className="text-text-mute">session</span>
              <span className="truncate font-mono">{event.sessionId}</span>
              <ChevronRight size={11} className="shrink-0 text-text-mute" aria-hidden="true" />
            </button>
          ) : (
            <span className={pillClass}>
              <span className="text-text-mute">session</span>
              <span className="truncate font-mono">{event.sessionId}</span>
            </span>
          )}
        </div>
      ) : null}
    </div>
  );
}

function wgDataString(data: Record<string, unknown>, key: string): string {
  const value = data[key];
  return typeof value === 'string' ? value : '';
}

function wgFormatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  }).format(date);
}

function WorkgroupDrawer({
  card,
  timeWindow,
  workgroupNameById,
  onClose,
}: {
  card: WorkgroupCardModel;
  timeWindow: TimeWindow;
  workgroupNameById?: Map<string, string>;
  onClose: () => void;
}) {
  const { width, dragHandleProps } = useResizableDrawer({
    defaultWidth: 576,
    minWidth: 360,
    maxWidth: 1000,
  });

  const [traceSessionId, setTraceSessionId] = useState<string | null>(null);
  const [traceSession, setTraceSession] = useState<Session | null>(null);
  const [traceLoading, setTraceLoading] = useState(false);
  const [traceError, setTraceError] = useState(false);

  const isTrace = traceSessionId !== null;

  function handleSessionPillClick(sessionId: string) {
    setTraceSessionId(sessionId);
    setTraceSession(null);
    setTraceLoading(true);
    setTraceError(false);
    getSession(sessionId)
      .then((session) => {
        setTraceSession(session);
        setTraceLoading(false);
      })
      .catch(() => {
        setTraceError(true);
        setTraceLoading(false);
      });
  }

  function handleBack() {
    setTraceSessionId(null);
    setTraceSession(null);
    setTraceLoading(false);
    setTraceError(false);
  }

  const command = `agora workgroup describe ${card.workgroup.id}`;

  function copyCommand() {
    void navigator.clipboard?.writeText(command);
  }

  return (
    <>
      <div className="fixed inset-0 z-[199] bg-text/20" aria-hidden="true" onClick={onClose} />
      <aside
        style={{ width, maxWidth: '100vw' }}
        className="fixed right-0 top-0 z-[200] flex h-full flex-col border-l border-border bg-page shadow-xl"
        role="dialog"
        aria-modal="true"
        aria-labelledby="workgroup-drawer-title"
      >
        <div
          {...dragHandleProps}
          aria-hidden="true"
          className="absolute left-0 top-1/2 z-10 hidden sm:flex h-9 w-5 -translate-x-full -translate-y-1/2 cursor-col-resize items-center justify-center rounded-md border border-border bg-page shadow-md text-text-mute transition-colors hover:bg-panel-subtle hover:text-text"
        >
          <GripVertical size={14} aria-hidden="true" />
        </div>
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

        <div className="relative flex flex-1 flex-col overflow-hidden">
          <div
            className="absolute inset-0 flex flex-col gap-4 overflow-y-auto p-6"
            style={{
              opacity: isTrace ? 0 : 1,
              transition: 'opacity 0.18s ease',
              pointerEvents: isTrace ? 'none' : undefined,
            }}
          >
            <p className="text-label text-text-mute">
              Data window: {formatTimeWindow(timeWindow)}
            </p>
            <SectionPanel title="Resource">
              <dl className="grid gap-3 sm:grid-cols-2">
                <DrawerField label="id" value={<span className="font-mono">{card.workgroup.id}</span>} />
                <DrawerField label="scope" value={<ScopePill scope={card.workgroup.scope} />} />
                <DrawerField label="state" value={card.workgroup.state} />
                <DrawerField label="members" value={formatInteger(card.memberCount)} />
                <DrawerField label="my role" value={card.callerRole ?? '-'} />
                <DrawerField label="envelopes" value={formatInteger(card.envelopes24h)} />
                <DrawerField
                  label="sessions"
                  value={card.metricsLoading ? <MetricSkeleton /> : formatInteger(card.sessionCount ?? 0)}
                />
                <DrawerField
                  label="violation rate"
                  value={card.metricsLoading ? <MetricSkeleton /> : `${(card.violationRate ?? 0).toFixed(1)}%`}
                />
                <DrawerField
                  label="avg duration"
                  value={
                    card.metricsLoading
                      ? <MetricSkeleton />
                      : card.avgSessionDuration !== undefined
                        ? formatSessionSeconds(card.avgSessionDuration)
                        : '—'
                  }
                />
              </dl>
            </SectionPanel>

            <SectionPanel title="Activity">
              <WorkgroupActivityStrip
                events={card.activityEvents ?? []}
                loading={card.activityLoading ?? false}
                timeWindow={timeWindow}
                workgroupNameById={workgroupNameById}
                onSessionClick={handleSessionPillClick}
                fillWidth
                showLabels
              />
            </SectionPanel>

            <SectionPanel title="Developer Reference">
              <p className="mb-3 text-table text-text-mute">
                Use the Agora CLI to inspect or manage this workgroup from your local environment. Copy the command below to get started.
              </p>
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

          <div
            className="absolute inset-0 flex flex-col overflow-hidden"
            style={{
              opacity: isTrace ? 1 : 0,
              transform: isTrace ? 'scale(1)' : 'scale(0.97)',
              transition: 'opacity 0.18s ease, transform 0.18s ease',
              pointerEvents: isTrace ? undefined : 'none',
            }}
          >
            <div className="flex shrink-0 items-center gap-1 border-b border-border px-4 py-3">
              <button
                type="button"
                onClick={handleBack}
                className="inline-flex items-center justify-center rounded p-1 text-text-mute hover:bg-panel-subtle hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
                aria-label="Back to workgroup detail"
              >
                <ChevronLeft size={13} aria-hidden="true" />
              </button>
              <span className="truncate font-mono text-table font-medium text-text">{traceSessionId}</span>
            </div>
            <div className="flex-1 overflow-y-auto p-6">
              {traceLoading ? (
                <LoadingPanel title="Loading session trace" />
              ) : traceError ? (
                <p className="text-body text-text-mute">Session trace could not be loaded. Close and try again.</p>
              ) : traceSession ? (
                <SessionTrace session={traceSession} auditEvents={card.activityEvents} />
              ) : null}
            </div>
          </div>
        </div>
      </aside>
    </>
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
      label: membership.email ?? membership.organizationId,
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

function ScopePill({ scope }: { scope: Workgroup['scope'] }) {
  const pill = <StatusPill status={scopeStatus(scope)} label={scope} />;

  if (scope !== 'inter-org') return pill;

  return <span title="Members span multiple organizations">{pill}</span>;
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

function MetricSkeleton() {
  return <div className="h-4 w-10 animate-pulse rounded bg-panel-subtle" aria-hidden="true" />;
}

function formatSessionSeconds(seconds: number): string {
  const total = Math.max(Math.floor(seconds), 0);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;

  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function formatTimeWindow(window: TimeWindow): string {
  switch (window) {
    case '24h': return 'Last 24 hours';
    case '7d': return 'Last 7 days';
    case '30d': return 'Last 30 days';
  }
}

function classifyWorkgroupEventToLane(event: AuditEvent): number {
  switch (event.eventType) {
    case 'session.proposed':
    case 'session.accepted':
    case 'session.rejected':
      return 0;
    case 'session.closed':
      return typeof event.data['close_reason'] === 'string' && event.data['close_reason'] === 'contract_violation'
        ? 1
        : 0;
    case 'envelope.flowed':
      return 2;
    case 'tunnel.attached':
    case 'tunnel.detached':
      return 3;
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

function buildWgTimeLabels(
  minTime: number,
  maxTime: number,
  timeWindow: TimeWindow,
): { position: number; label: string }[] {
  if (minTime === 0 && maxTime === 0) return [];
  if (minTime === maxTime) {
    return [{ position: 50, label: formatWgAxisTime(new Date(minTime), timeWindow) }];
  }
  const count = 3;
  return Array.from({ length: count }, (_, i) => ({
    position: (i / (count - 1)) * 100,
    label: formatWgAxisTime(new Date(minTime + (maxTime - minTime) * (i / (count - 1))), timeWindow),
  }));
}

function formatWgAxisTime(date: Date, timeWindow: TimeWindow): string {
  switch (timeWindow) {
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
