import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import {
  Activity,
  AlertTriangle,
  ArrowLeftRight,
  Ban,
  BookOpen,
  Boxes,
  Check,
  ChevronDown,
  ChevronLeft,
  Clock3,
  Copy,
  Eye,
  EyeOff,
  FileCheck2,
  Gauge,
  Layers,
  LogIn,
  Mail,
  Megaphone,
  Network,
  RefreshCcw,
  ShieldAlert,
  ShieldCheck,
  Terminal,
  Users,
  Wifi,
  X,
  type LucideIcon,
} from 'lucide-react';

import {
  AppShell,
  BarChart,
  Button,
  DrawerCard,
  DrawerDivider,
  DrawerTip,
  EmptyState,
  InfoDrawer,
  InfoTooltip,
  PageHeader,
  Select,
  SectionPanel,
  SidebarBreakdown,
  StatCard,
  StatusPill,
  type BarChartDatum,
  type SidebarBreakdownItem,
} from '../components';
import {
  ApiError,
  fetchAllVisibleAdvertisements,
  getAccountToken,
  getDashboardActivity,
  getDashboardSummary,
  listAuditEvents,
  listWorkgroups,
  useApiResource,
  type Advertisement,
  type AuditEvent,
  type Workgroup,
  type DashboardActivityResponse,
  type DashboardBucket,
  type DashboardSummaryResponse,
  type DashboardWindow,
} from '../lib/api';

type ActivityWindowOption = {
  value: DashboardWindow;
  label: string;
  bucket: DashboardBucket;
};

const activityWindowOptions: ActivityWindowOption[] = [
  { value: '24h', label: 'Last 24 hours', bucket: '1h' },
  { value: '7d', label: 'Last 7 days', bucket: '1d' },
  { value: '30d', label: 'Last 30 days', bucket: '1d' },
];
const routeByTab: Record<string, string> = {
  dashboard: '/',
  sessions: '/sessions',
  workgroups: '/workgroups',
  catalog: '/catalog',
  contracts: '/contracts',
  audit: '/audit',
};

// the topology card's org graph is sourced from fetchAllVisibleAdvertisements:
// demo@agora.local belongs to every inter-org workgroup, so all provider
// advertisements (organizationName, status, workgroupScopes) are visible
// cross-org — /dashboard/environments would only ever return the caller's own org.

const numberFormatter = new Intl.NumberFormat();

const DASHBOARD_POLL_MS = 5000;

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

function dataString(data: Record<string, unknown>, key: string): string {
  const value = data[key];
  return typeof value === 'string' ? value : '';
}

export default function Dashboard() {
  const navigate = useNavigate();
  const [infoOpen, setInfoOpen] = useState(false);
  const [activityWindow, setActivityWindow] = useState<DashboardWindow>('24h');
  const summary = useApiResource(getDashboardSummary, { intervalMs: DASHBOARD_POLL_MS });
  const activityLoad = useCallback(
    (signal: AbortSignal) =>
      getDashboardActivity(
        {
          window: activityWindow,
          bucket: bucketForWindow(activityWindow),
        },
        signal,
      ),
    [activityWindow],
  );
  const activity = useApiResource(activityLoad, { intervalMs: DASHBOARD_POLL_MS });
  const advertisements = useApiResource(fetchAllVisibleAdvertisements, { intervalMs: DASHBOARD_POLL_MS });
  const workgroupsResource = useApiResource(listWorkgroups);
  const liveEventsLoad = useCallback(
    (signal: AbortSignal) => listAuditEvents({ limit: 50 }, signal),
    [],
  );
  const liveEvents = useApiResource(liveEventsLoad, { intervalMs: DASHBOARD_POLL_MS });
  const account = summary.data?.account;

  const agentRows = useMemo(
    () => advertisements.data ?? [],
    [advertisements.data],
  );

  const hasError = Boolean(summary.error || activity.error || advertisements.error);
  const isLoading =
    (summary.loading && !summary.data) ||
    (activity.loading && !activity.data) ||
    (advertisements.loading && !advertisements.data);

  const chartData = useMemo(
    () => toChartData(activity.data, activityWindow),
    [activity.data, activityWindow],
  );
  const workgroupBreakdown = useMemo(
    () => toWorkgroupBreakdown(activity.data),
    [activity.data],
  );
  const orgStatuses = useMemo(
    () => computeOrgStatuses(agentRows),
    [agentRows],
  );
  const workgroupNameMap = useMemo(() => {
    const map: Record<string, string> = {};
    workgroupsResource.data?.forEach((wg) => { map[wg.id] = wg.name; });
    return map;
  }, [workgroupsResource.data]);

  function handleTabChange(tabId: string) {
    const route = routeByTab[tabId];

    if (route) {
      navigate(route);
    }
  }

  return (
    <AppShell
      product="agora"
      organizationName={account?.organizationName ?? 'Loading organization'}
      activeTab="dashboard"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'Connected'}
      userInitials={account ? initialsForEmail(account.email) : '--'}
      userLabel={account?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6 xl:flex-row xl:items-start">
        {/* Main content column — a container-query context so the panels below reflow
            to the column's width (not the viewport) once the rail claims ~33%. */}
        <div className="@container flex min-w-0 flex-1 flex-col gap-6">
          <PageHeader
            icon={Network}
            label="AGENT NETWORK"
            title="Agora"
            description="The secure network layer where AI agents discover, connect, and collaborate — with every interaction governed by identity, bounded by contract, and fully auditable."
            onInfoClick={() => setInfoOpen(true)}
          />
          {summary.error ? (
            <ErrorPanel title="Dashboard summary unavailable" error={summary.error} onRetry={summary.refetch} />
          ) : summary.data ? (
            <AccountCallout summary={summary.data} />
          ) : (
            <LoadingPanel title="Loading current account" />
          )}

          {summary.error ? null : summary.data ? <SummaryStats summary={summary.data} /> : <StatsLoading />}

          {/* Row 1: Topology (centerpiece) and Envelope Activity share a 50/50 row at
              equal height; they stack to one column when the main column narrows. */}
          <div className="grid grid-cols-1 gap-4 @2xl:grid-cols-2">
            <NetworkOverviewPanel
              hubOrgName={account?.organizationName ?? ''}
              orgStatuses={orgStatuses}
              hasActivity={chartData.length > 0}
              workgroups={workgroupsResource.data ?? []}
              advertisements={agentRows}
            />

            <SectionPanel
              title="Envelope Activity"
              className="h-[320px] flex flex-col"
              bodyClassName="flex flex-col flex-1 min-h-0"
              actions={
                <div className="relative">
                  <Select
                    value={activityWindow}
                    onChange={(event) => {
                      setActivityWindow(event.target.value as DashboardWindow);
                      activity.refetch();
                    }}
                    aria-label="Activity window"
                    className="pr-7 font-medium"
                  >
                    {activityWindowOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </Select>
                  <ChevronDown size={13} aria-hidden="true" className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-text-mute" />
                </div>
              }
            >
              {activity.error ? (
                <ErrorPanel title="Activity unavailable" error={activity.error} onRetry={activity.refetch} compact />
              ) : activity.loading && !activity.data ? (
                <LoadingPanel title="Loading activity" compact />
              ) : chartData.length > 0 ? (
                <div className="h-full w-full">
                  {/* fewer time-axis labels at the narrower half-column width so they
                      thin out (every Nth) instead of crowding/overlapping. */}
                  <BarChart data={chartData} accent="agora" maxLabels={6} />
                </div>
              ) : (
                <EmptyState
                  icon={Activity}
                  title="No envelope activity"
                  description="No envelope activity has been recorded in this window."
                />
              )}
            </SectionPanel>
          </div>

          {/* Row 2: Top Workgroups spans the full main-column width below Row 1. */}
          <SectionPanel
            title="Top Workgroups"
            className="h-[320px] flex flex-col"
            bodyClassName="flex-1 min-h-0 overflow-auto"
          >
            {activity.error ? (
              <ErrorPanel title="Workgroup activity unavailable" error={activity.error} onRetry={activity.refetch} compact />
            ) : activity.loading && !activity.data ? (
              <LoadingPanel title="Loading workgroups" compact />
            ) : workgroupBreakdown.length > 0 ? (
              <SidebarBreakdown items={workgroupBreakdown} />
            ) : (
              <EmptyState
                icon={Users}
                title="No workgroup activity"
                description="No workgroup envelope totals are available for this window."
              />
            )}
          </SectionPanel>
        </div>

        {/* Persistent activity rail — full-height and sticky beside the content on wide
            viewports; drops below the main column under xl so it never crushes either side. */}
        <aside className="flex w-full flex-col xl:sticky xl:top-0 xl:h-[calc(100vh-60px-3rem)] xl:w-[33%] xl:min-w-[320px] xl:max-w-[440px] xl:shrink-0">
          <SectionPanel
            title="Live Activity"
            className="flex flex-col max-h-[32rem] xl:h-full xl:max-h-none"
            bodyClassName="p-0 flex flex-1 min-h-0 flex-col"
            actions={
              <LiveIndicator
                state={
                  liveEvents.error
                    ? 'error'
                    : liveEvents.loading && !liveEvents.data
                      ? 'loading'
                      : 'live'
                }
              />
            }
          >
            {liveEvents.error ? (
              <div className="p-5">
                <ErrorPanel
                  title="Live activity unavailable"
                  error={liveEvents.error}
                  onRetry={() => liveEvents.refetch()}
                  compact
                />
              </div>
            ) : (
              <LiveActivityContent
                events={liveEvents.data?.items}
                loading={liveEvents.loading}
                workgroupNames={workgroupNameMap}
              />
            )}
          </SectionPanel>
        </aside>
      </div>

      {infoOpen ? (
        <InfoDrawer title="What is Agora?" onClose={() => setInfoOpen(false)} wide>
          <div className="flex flex-col gap-5">
            <section>
              <h3 className="mb-2 font-semibold text-text">What is Agora?</h3>
              <p className="leading-relaxed text-text-mute">
                Agora is the network layer underneath your AI agents — not a framework, not a protocol,
                but the governed fabric that connects agents to each other securely across organizational
                boundaries. Built on OpenZiti, every agent gets a cryptographic identity, every connection
                is mutually authenticated and encrypted, and nothing is reachable unless the network
                explicitly creates a path. The governance lives in the network — not in each agent's code.
              </p>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-3 font-semibold text-text">The Three Layers</h3>
              <div className="flex flex-col gap-2">
                <DrawerCard icon={Layers} title="Layer 0 – Fabric" description="OpenZiti. Cryptographic identity (X.509) per agent, mutual authentication, end-to-end encryption, and dark-by-default connectivity — no listening ports, no public endpoints, agents are invisible unless the network creates a path." />
                <DrawerCard icon={Network} title="Layer 1 – Network" description="Agora's connectivity primitives: organizations, accounts, environments, and encrypted tunnels. Useful on its own for any secure service-connectivity use case." />
                <DrawerCard icon={Users} title="Layer 2 – Collaboration" description="The governance layer. Six concepts that make Agora agent-aware: Workgroups, Catalog, Advertisements, Sessions, Contracts, and Envelopes." />
              </div>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-3 font-semibold text-text">Layer 2 Concepts</h3>
              <div className="flex flex-col gap-2">
                <DrawerCard icon={ShieldCheck} title="Workgroups" description="Policy boundaries that control visibility. Agents outside a workgroup cannot discover, interact with, or detect agents inside it. Zero knowledge, not a filtered view." />
                <DrawerCard icon={BookOpen} title="Catalog" description="The discovery surface. Every query is filtered by workgroup membership. Agents you're not authorized to see don't appear in results at all." />
                <DrawerCard icon={Megaphone} title="Advertisements" description="An agent's persistent declaration of its capabilities, visibility scope, and required contract terms. Survives agent restarts." />
                <DrawerCard icon={ArrowLeftRight} title="Sessions" description="Governed communication channels with explicit lifecycle (proposed → active → closed). Retained after close for audit." />
                <DrawerCard icon={FileCheck2} title="Contracts" description="Engagement terms enforced by the controller: max duration, max envelope count, allowed message types. Not the agent's job to enforce." />
                <DrawerCard icon={Mail} title="Envelopes" description="The structured message format. Infrastructure-visible headers carry correlation IDs for end-to-end audit trail reconstruction. Payloads are opaque to the network." />
              </div>
            </section>

            <DrawerDivider />

            <DrawerTip>
              The Macro Pulse demo running in this dashboard spans 5 organizations and 8 agents across 4 inter-org workgroups. The market data provider and internet signals provider do not share a workgroup — neither knows the other exists. Every session is contract-bounded. Every envelope is auditable.
            </DrawerTip>
          </div>
        </InfoDrawer>
      ) : null}
    </AppShell>
  );
}

function AccountCallout({ summary }: { summary: DashboardSummaryResponse }) {
  const [cliOpen, setCliOpen] = useState(false);

  return (
    <section className="rounded-card border border-border bg-panel p-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
          <h1 className="truncate text-body font-semibold text-text">{summary.account.organizationName}</h1>
          <p className="truncate text-table text-text-mute">{summary.account.email}</p>
          <StatusPill status="neutral" label={formatRole(summary.account.role)} />
          <Button variant="secondary" onClick={() => setCliOpen(true)}>
            <Terminal className="h-3.5 w-3.5" aria-hidden="true" />
            Enable an environment
          </Button>
        </div>

        <div className="grid grid-cols-2 gap-x-4 gap-y-2 sm:flex sm:flex-wrap sm:gap-y-1">
          <RibbonMetric label="workgroups" value={summary.ribbon.workgroupCount} />
          <RibbonMetric
            label="advertisements"
            value={summary.ribbon.advertisementCount}
            tooltip="Active advertisements published by your organization. The catalog may show additional advertisements shared with you by other organizations."
          />
          <RibbonMetric label="sessions today" value={summary.ribbon.sessionsToday} />
          <RibbonMetric label="environments" value={summary.ribbon.environmentCount} />
        </div>
      </div>

      {cliOpen && <CliAccessModal onClose={() => setCliOpen(false)} />}
    </section>
  );
}

const TOKEN_MASK = '••••••••••••••••••••••••••••••••';
const CONFIG_COMMAND = 'agora config set api_endpoint <controller-endpoint>';

function CliAccessModal({ onClose }: { onClose: () => void }) {
  const [token, setToken] = useState<string | null>(null);
  const [revealed, setRevealed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<'token' | 'command' | 'config' | null>(null);

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') onClose();
    }
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  const ensureToken = useCallback(async (): Promise<string | null> => {
    if (token) return token;
    setLoading(true);
    setError(null);
    try {
      const res = await getAccountToken();
      setToken(res.accountToken);
      return res.accountToken;
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'failed to load account token';
      setError(message);
      return null;
    } finally {
      setLoading(false);
    }
  }, [token]);

  const flashCopied = useCallback((field: 'token' | 'command' | 'config') => {
    setCopied(field);
    setTimeout(() => {
      setCopied((current) => (current === field ? null : current));
    }, 1500);
  }, []);

  const handleCopyConfig = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(CONFIG_COMMAND);
      flashCopied('config');
    } catch {
      setError('failed to copy to clipboard');
    }
  }, [flashCopied]);

  const handleToggleReveal = useCallback(async () => {
    if (revealed) {
      setRevealed(false);
      return;
    }
    const value = await ensureToken();
    if (value) setRevealed(true);
  }, [revealed, ensureToken]);

  const handleCopyToken = useCallback(async () => {
    const value = await ensureToken();
    if (!value) return;
    try {
      await navigator.clipboard.writeText(value);
      flashCopied('token');
    } catch {
      setError('failed to copy to clipboard');
    }
  }, [ensureToken, flashCopied]);

  const handleCopyCommand = useCallback(async () => {
    const value = await ensureToken();
    if (!value) return;
    try {
      await navigator.clipboard.writeText(`agora enable ${value}`);
      flashCopied('command');
    } catch {
      setError('failed to copy to clipboard');
    }
  }, [ensureToken, flashCopied]);

  const tokenDisplay = revealed && token ? token : TOKEN_MASK;
  const commandDisplay = `agora enable ${revealed && token ? token : TOKEN_MASK}`;

  return (
    <div
      className="fixed inset-0 z-[110] flex items-center justify-center bg-text/30 p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="cli-access-title"
    >
      <button
        type="button"
        className="absolute inset-0 cursor-default"
        aria-label="Close CLI access"
        onClick={onClose}
      />
      <div className="relative w-full max-w-xl rounded-card border border-border bg-page shadow-xl">
        <header className="flex items-start justify-between gap-4 border-b border-border bg-panel px-5 py-4">
          <div className="min-w-0">
            <p className="text-label font-medium uppercase text-text-mute">CLI access</p>
            <h2 id="cli-access-title" className="mt-1 text-section font-semibold text-text">
              Enable a local environment
            </h2>
          </div>
          <button
            type="button"
            className="inline-flex size-9 shrink-0 items-center justify-center rounded-pill border border-border bg-panel text-text-mute-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            aria-label="Close CLI access"
            onClick={onClose}
          >
            <X size={18} aria-hidden="true" />
          </button>
        </header>

        <div className="flex flex-col gap-5 p-5">
          <p className="text-table text-text-mute">
            Enable a local Agora environment so an agent can join this network using your account token.
          </p>

          <ModalStep number={1} title="Point the CLI at the controller">
            <TokenRow
              label="Command"
              value={CONFIG_COMMAND}
              copied={copied === 'config'}
              copyAriaLabel="Copy config command"
              onCopy={handleCopyConfig}
            />
            <p className="text-label text-text-mute">
              Run once to tell the CLI which controller to use. Agora stores local CLI settings under{' '}
              <code className="font-mono text-text">~/.agora</code>.
            </p>
          </ModalStep>

          <ModalStep number={2} title="Enable the environment">
            <TokenRow
              label="Account token"
              value={tokenDisplay}
              copied={copied === 'token'}
              copyAriaLabel="Copy account token"
              onCopy={handleCopyToken}
              reveal={{
                revealed,
                onToggle: handleToggleReveal,
                loading,
              }}
            />
            <TokenRow
              label="Enable command"
              value={commandDisplay}
              copied={copied === 'command'}
              copyAriaLabel="Copy enable command"
              onCopy={handleCopyCommand}
            />
            <p className="text-label text-text-mute">
              Provide the account token above, or omit it to log in interactively with your account
              email and password. Optional flags: <code className="font-mono text-text">--description</code>,{' '}
              <code className="font-mono text-text">--host</code>.
            </p>
            <p className="text-label text-text-mute">
              This enrolls the environment and writes its identity material under{' '}
              <code className="font-mono text-text">~/.agora</code>.
            </p>
          </ModalStep>

          <div className="border-t border-border pt-5">
            <ModalStep number={3} title="Run an agent (next step)">
              <p className="text-label text-text-mute">
                Enabling an environment does not start an agent by itself. Next, run an agent process
                pointed at this environment — either start the local network runtime for CLI tunnel
                operations (<code className="font-mono text-text">agora network start</code>), or run an
                agent built with the Agora SDK. See the{' '}
                <a
                  href="https://github.com/openziti/agora/blob/main/docs/current/sdk/overview.md"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-medium text-brand-agora underline transition-colors hover:text-brand-agora-end focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
                >
                  Agora SDK overview
                </a>{' '}
                and the{' '}
                <a
                  href="https://github.com/openziti/agora/blob/main/examples/macro-pulse/README.md"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="font-medium text-brand-agora underline transition-colors hover:text-brand-agora-end focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
                >
                  Macro Pulse example
                </a>{' '}
                in the repo for the agent quickstart.
              </p>
            </ModalStep>
          </div>

          {error && <p className="text-label text-danger">{error}</p>}
        </div>
      </div>
    </div>
  );
}

function ModalStep({
  number,
  title,
  children,
}: {
  number: number;
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="flex gap-3">
      <span className="flex size-6 shrink-0 items-center justify-center rounded-pill border border-border bg-panel-subtle text-label font-semibold text-text-mute-strong">
        {number}
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <p className="text-table font-semibold text-text">{title}</p>
        {children}
      </div>
    </div>
  );
}

type TokenRowProps = {
  label: string;
  value: string;
  copied: boolean;
  copyAriaLabel: string;
  onCopy: () => void;
  reveal?: {
    revealed: boolean;
    onToggle: () => void;
    loading: boolean;
  };
};

function TokenRow({ label, value, copied, copyAriaLabel, onCopy, reveal }: TokenRowProps) {
  return (
    <div className="flex items-center gap-3 overflow-hidden rounded-pill border border-border bg-panel-subtle px-3 py-2">
      <span className="w-28 shrink-0 text-label font-medium uppercase text-text-mute">{label}</span>
      <span className="min-w-0 flex-1 truncate font-mono text-table text-text">{value}</span>
      {reveal && (
        <button
          type="button"
          onClick={reveal.onToggle}
          disabled={reveal.loading}
          aria-label={reveal.revealed ? 'Hide account token' : 'Reveal account token'}
          className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-pill text-text-mute hover:bg-panel hover:text-text focus-visible:outline-brand-agora disabled:opacity-50"
        >
          {reveal.revealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      )}
      <button
        type="button"
        onClick={onCopy}
        aria-label={copied ? 'Copied' : copyAriaLabel}
        className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-pill text-text-mute hover:bg-panel hover:text-text focus-visible:outline-brand-agora"
      >
        {copied ? <Check className="h-4 w-4 text-success" /> : <Copy className="h-4 w-4" />}
      </button>
    </div>
  );
}

function RibbonMetric({ label, value, tooltip }: { label: string; value: number; tooltip?: string }) {
  return (
    <div className="border-l border-border pl-3">
      <p className="flex items-center gap-1 text-label font-medium uppercase text-text-mute">
        {label}
        {tooltip && <InfoTooltip content={tooltip} ariaLabel={tooltip} />}
      </p>
      <p className="text-body font-semibold text-text">{formatInteger(value)}</p>
    </div>
  );
}

function SummaryStats({ summary }: { summary: DashboardSummaryResponse }) {
  const navigate = useNavigate();
  return (
    <div className="grid gap-4 @md:grid-cols-2 @3xl:grid-cols-4">
      <StatCard
        label="Active Sessions"
        value={formatInteger(summary.stats.activeSessions)}
        icon={Activity}
        accent="agora"
        onClick={() => navigate('/sessions')}
        caret
      />
      <StatCard
        label="Envelopes Today"
        value={formatInteger(summary.stats.envelopesToday)}
        icon={Wifi}
        accent="success"
        onClick={() => navigate('/audit', { state: { eventTypeFilter: 'envelope.flowed', timeRange: '24h' } })}
        caret
      />
      <StatCard
        label="Active Workgroups"
        value={formatInteger(summary.stats.activeWorkgroups)}
        icon={Boxes}
        accent="info"
        onClick={() => navigate('/workgroups')}
        caret
      />
      <StatCard
        label="Active Tunnels"
        value={formatInteger(summary.stats.activeTunnels)}
        icon={Gauge}
        accent="warning"
      />
    </div>
  );
}

function StatsLoading() {
  return (
    <div className="grid gap-4 @md:grid-cols-2 @3xl:grid-cols-4">
      {['Active Sessions', 'Envelopes Today', 'Active Workgroups', 'Active Tunnels'].map((label) => (
        <section key={label} className="flex flex-col justify-between rounded-[7px] border border-border bg-panel p-3">
          <p className="text-label font-medium uppercase text-text-mute">{label}</p>
          <p className="text-body text-text-mute">Loading</p>
        </section>
      ))}
    </div>
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

function toChartData(activity: DashboardActivityResponse | undefined, window: DashboardWindow): BarChartDatum[] {
  return (
    activity?.buckets.map((bucket) => ({
      label: formatBucketLabel(bucket.start, window),
      value: bucket.envelopes,
    })) ?? []
  );
}

function toWorkgroupBreakdown(activity: DashboardActivityResponse | undefined): SidebarBreakdownItem[] {
  return (
    activity?.byWorkgroup.map((workgroup, index) => ({
      label: workgroup.workgroupName,
      value: formatInteger(workgroup.envelopes),
      barValue: workgroup.envelopes,
      accent: index % 3 === 0 ? 'agora' : index % 3 === 1 ? 'info' : 'success',
    })) ?? []
  );
}

function bucketForWindow(window: DashboardWindow): DashboardBucket {
  return activityWindowOptions.find((option) => option.value === window)?.bucket ?? '1h';
}

function formatBucketLabel(value: string, window: DashboardWindow): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  if (window === '24h') {
    return new Intl.DateTimeFormat(undefined, { hour: 'numeric' }).format(date);
  }

  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(date);
}


function formatInteger(value: number): string {
  return numberFormatter.format(value);
}


function formatRole(role: string): string {
  return role.replace(/_/g, ' ');
}

function initialsForEmail(email: string): string {
  const localPart = email.split('@')[0] ?? email;
  const parts = localPart.split(/[^a-zA-Z0-9]+/).filter(Boolean);
  const initials = (parts.length > 1 ? parts : [localPart])
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();

  return initials || 'AG';
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

type OrgHealthStatus = 'active' | 'stale' | 'unknown';

function computeOrgStatuses(ads: Advertisement[]): Map<string, OrgHealthStatus> {
  const map = new Map<string, OrgHealthStatus>();
  for (const ad of ads) {
    const org = ad.organizationName;
    const prev = map.get(org);
    if (ad.status === 'retracted') {
      map.set(org, 'stale');
    } else if (prev !== 'stale') {
      map.set(org, 'active');
    }
  }
  return map;
}

// Data-driven topology layout.
// The hub is the logged-in org; provider orgs are laid out radially around it
// (see buildSpokeNodes). Org labels sit on the outer side of each node.
const T_CX = 200;  // center x
const T_CY = 130;  // center y
const T_CR = 34;   // center node radius
const T_PR = 17;   // provider node radius
const T_R  = 88;   // spoke length (center-to-center)
const T_RING = 114; // dashed isolation ring radius

const T_PAD_SIDE = 24; // gap between node edge and label

type SpokeNode = {
  id: string;
  name: string;
  monogram: string;
  cx: number;
  cy: number;
  // org name label (outward from center)
  olx: number;
  oly: number;
  olAnchor: 'start' | 'middle' | 'end';
};

// place N provider orgs evenly around the hub, starting at the top (-90deg).
// the label anchor is derived from each node's angle so labels sit on the outer
// side of the node (right half -> start, left half -> end, top/bottom -> middle).
function buildSpokeNodes(orgNames: string[]): SpokeNode[] {
  const n = orgNames.length;
  const labelDist = T_R + T_PR + T_PAD_SIDE;
  return orgNames.map((name, i) => {
    const angle = -Math.PI / 2 + (2 * Math.PI * i) / n;
    const cos = Math.cos(angle);
    const sin = Math.sin(angle);
    let olAnchor: 'start' | 'middle' | 'end' = 'middle';
    if (cos > 0.3) olAnchor = 'start';
    else if (cos < -0.3) olAnchor = 'end';
    return {
      id: name,
      name,
      monogram: name.trim().slice(0, 2).toUpperCase() || '?',
      cx: T_CX + T_R * cos,
      cy: T_CY + T_R * sin,
      olx: T_CX + labelDist * cos,
      oly: T_CY + labelDist * sin + 4,
      olAnchor,
    };
  });
}

function statusDotFill(status: OrgHealthStatus | undefined): string {
  if (status === 'active') return '#22c55e';
  if (status === 'stale')  return '#d97706';
  return '#9ca3af';
}

function NetworkOverviewPanel({
  hubOrgName,
  orgStatuses,
  hasActivity,
  workgroups,
  advertisements,
}: {
  hubOrgName: string;
  orgStatuses: Map<string, OrgHealthStatus>;
  hasActivity: boolean;
  workgroups: Workgroup[];
  advertisements: Advertisement[];
}) {
  // distinct provider orgs (excluding the hub, which may also advertise)
  const spokeOrgNames = useMemo(() => {
    const seen = new Set<string>();
    const names: string[] = [];
    for (const ad of advertisements) {
      const name = ad.organizationName;
      if (!name || name === hubOrgName || seen.has(name)) continue;
      seen.add(name);
      names.push(name);
    }
    return names;
  }, [advertisements, hubOrgName]);

  // distinct workgroup ids across all visible advertisements -> governed channels
  const channelCount = useMemo(() => {
    const ids = new Set<string>();
    for (const ad of advertisements) {
      for (const id of ad.workgroupScopes ?? []) ids.add(id);
    }
    return ids.size;
  }, [advertisements]);

  const orgCount = spokeOrgNames.length + (hubOrgName ? 1 : 0);

  return (
    <section className="h-[320px] flex flex-col rounded-[7px] border border-border bg-panel">
      <header className="flex min-h-10 items-center justify-between gap-4 border-b border-border bg-panel px-3 py-2">
        <h2 className="text-body font-semibold text-text">Network Topology</h2>
        <span className="shrink-0 text-[0.6875rem] text-text-mute-2">
          {orgCount} {orgCount === 1 ? 'org' : 'orgs'} &middot; {channelCount} {channelCount === 1 ? 'channel' : 'channels'}
        </span>
      </header>
      <div className="flex flex-1 flex-col min-h-0">
        <TopologyContent
          hubOrgName={hubOrgName}
          spokeOrgNames={spokeOrgNames}
          orgStatuses={orgStatuses}
          hasActivity={hasActivity}
          workgroups={workgroups}
          advertisements={advertisements}
        />
      </div>
    </section>
  );
}

function TopologyContent({
  hubOrgName,
  spokeOrgNames,
  orgStatuses,
  hasActivity,
  workgroups,
  advertisements,
}: {
  hubOrgName: string;
  spokeOrgNames: string[];
  orgStatuses: Map<string, OrgHealthStatus>;
  hasActivity: boolean;
  workgroups: Workgroup[];
  advertisements: Advertisement[];
}) {
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const [selectedOrgName, setSelectedOrgName] = useState<string | null>(null);

  const spokeNodes = useMemo(() => buildSpokeNodes(spokeOrgNames), [spokeOrgNames]);

  useEffect(() => {
    if (!selectedOrgName) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setSelectedOrgName(null);
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [selectedOrgName]);

  const isDetail = selectedOrgName !== null;

  return (
    <div className="relative min-h-0 flex-1 flex flex-col">
      {/* Topology layer */}
      <div
        className="absolute inset-0 flex flex-col"
        style={{
          opacity: isDetail ? 0 : 1,
          transform: isDetail ? 'scale(0.97)' : 'scale(1)',
          transition: 'opacity 0.18s ease, transform 0.18s ease',
          pointerEvents: isDetail ? 'none' : undefined,
        }}
      >
        <div className="min-h-0 flex-1">
          <svg
            viewBox="0 -12 400 284"
            width="100%"
            height="100%"
            aria-label="Macro Pulse network topology"
            style={{ display: 'block' }}
          >
            {/* Dashed isolation ring */}
            <circle
              cx={T_CX} cy={T_CY} r={T_RING}
              fill="none" stroke="#d1d5db" strokeWidth="1" strokeDasharray="5 4"
            />

            {/* Spoke lines + animated flow dots */}
            {spokeNodes.map((p) => {
              const pathD = `M ${T_CX} ${T_CY} L ${p.cx} ${p.cy}`;
              const isActive = orgStatuses.get(p.name) === 'active' && hasActivity;
              const op = (!hoveredId || hoveredId === hubOrgName) ? 1 : hoveredId === p.id ? 1 : 0.15;
              return (
                <g
                  key={`spoke-${p.id}`}
                  style={{ opacity: op, transition: 'opacity 0.18s ease' }}
                >
                  <path d={pathD} stroke="#d1d5db" strokeWidth="1.5" fill="none" />
                  {isActive && (
                    <circle r="3" fill="#4f46e5" opacity="0.6">
                      <animateMotion dur="2.4s" repeatCount="indefinite" path={pathD} />
                    </circle>
                  )}
                </g>
              );
            })}

            {/* Provider nodes — drawn before labels so labels render on top */}
            {spokeNodes.map((p) => {
              const dotFill = statusDotFill(orgStatuses.get(p.name));
              const op = (!hoveredId || hoveredId === hubOrgName) ? 1 : hoveredId === p.id ? 1 : 0.15;
              return (
                <g
                  key={`node-${p.id}`}
                  style={{ opacity: op, transition: 'opacity 0.18s ease', cursor: 'pointer' }}
                  onMouseEnter={() => setHoveredId(p.id)}
                  onMouseLeave={() => setHoveredId(null)}
                  onClick={() => setSelectedOrgName(p.name)}
                >
                  {/* Larger transparent hit area */}
                  <circle cx={p.cx} cy={p.cy} r={T_PR + 6} fill="transparent" />
                  <circle cx={p.cx} cy={p.cy} r={T_PR} fill="#eef2ff" stroke="#818cf8" strokeWidth="1.5" />
                  <text
                    x={p.cx} y={p.cy + 3.5}
                    textAnchor="middle" fontSize="10" fontWeight="600"
                    fill="#3730a3" fontFamily="inherit"
                  >
                    {p.monogram}
                  </text>
                  <circle cx={p.cx + 12} cy={p.cy - 12} r="4" fill={dotFill} stroke="#eef2ff" strokeWidth="1.5" />
                </g>
              );
            })}

            {/* Org name labels drawn last so they are never occluded */}
            {spokeNodes.map((p) => {
              const op = (!hoveredId || hoveredId === hubOrgName) ? 1 : hoveredId === p.id ? 1 : 0.15;
              return (
                <text
                  key={`label-${p.id}`}
                  x={p.olx} y={p.oly}
                  textAnchor={p.olAnchor} fontSize="9.5" fontWeight="500"
                  style={{ fill: 'var(--color-text-mute)', opacity: op, transition: 'opacity 0.18s ease' }}
                  fontFamily="inherit"
                >
                  {p.name}
                </text>
              );
            })}

            {/* hub — the logged-in org (orchestrator), all text inside the circle */}
            <g
              onMouseEnter={() => setHoveredId(hubOrgName)}
              onMouseLeave={() => setHoveredId(null)}
            >
              <circle cx={T_CX} cy={T_CY} r={T_CR + 6} fill="transparent" />
              <circle cx={T_CX} cy={T_CY} r={T_CR} fill="#4f46e5" />
              <text
                x={T_CX} y={T_CY - 5}
                textAnchor="middle" fontSize="16" fontWeight="700"
                fill="#ffffff" fontFamily="inherit"
              >
                {hubOrgName.trim().slice(0, 2).toUpperCase() || '?'}
              </text>
              <text
                x={T_CX} y={T_CY + 9}
                textAnchor="middle" fontSize="6.5" fontWeight="500"
                fill="rgba(255,255,255,0.8)" fontFamily="inherit"
              >
                {hubOrgName}
              </text>
            </g>
          </svg>
        </div>

        {/* Isolation callout — rendered as HTML so it's fully clear of SVG coordinates */}
        <div className="flex items-center gap-2 border-t border-border-light px-3 py-2">
          <ShieldCheck size={12} aria-hidden="true" className="shrink-0 text-info" />
          <span className="text-[0.6875rem] text-text-mute">
            Providers connect only through the orchestrator
          </span>
        </div>
      </div>

      {/* Detail layer */}
      <div
        className="absolute inset-0 flex flex-col overflow-hidden"
        style={{
          opacity: isDetail ? 1 : 0,
          transform: isDetail ? 'scale(1)' : 'scale(0.97)',
          transition: 'opacity 0.18s ease, transform 0.18s ease',
          pointerEvents: isDetail ? undefined : 'none',
        }}
      >
        {selectedOrgName && (
          <OrgDetailView
            orgName={selectedOrgName}
            workgroups={workgroups}
            advertisements={advertisements}
            onBack={() => setSelectedOrgName(null)}
          />
        )}
      </div>
    </div>
  );
}

function OrgDetailView({
  orgName,
  workgroups,
  advertisements,
  onBack,
}: {
  orgName: string;
  workgroups: Workgroup[];
  advertisements: Advertisement[];
  onBack: () => void;
}) {
  const orgWorkgroupIds = useMemo(() => {
    const ids = new Set<string>();
    for (const ad of advertisements) {
      if (ad.organizationName === orgName) {
        for (const id of ad.workgroupScopes) ids.add(id);
      }
    }
    return ids;
  }, [advertisements, orgName]);

  const orgWorkgroups = useMemo(
    () => workgroups.filter((wg) => orgWorkgroupIds.has(wg.id)),
    [workgroups, orgWorkgroupIds],
  );

  const orgAds = useMemo(
    () => advertisements.filter((ad) => ad.organizationName === orgName),
    [advertisements, orgName],
  );

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-1 border-b border-border px-2 py-1.5">
        <button
          type="button"
          onClick={onBack}
          className="inline-flex items-center justify-center rounded p-1 text-text-mute hover:bg-panel-subtle hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
          aria-label="Back to topology"
        >
          <ChevronLeft size={13} aria-hidden="true" />
        </button>
        <span className="truncate text-table font-semibold text-text">{orgName}</span>
      </div>
      <div className="flex-1 space-y-3 overflow-y-auto p-2.5">
        <div>
          <p className="mb-1.5 text-[0.6rem] font-semibold uppercase tracking-wide text-text-mute">
            Workgroups
          </p>
          {orgWorkgroups.length === 0 ? (
            <p className="text-label text-text-mute-2">No workgroups found</p>
          ) : (
            <div className="space-y-1">
              {orgWorkgroups.map((wg) => (
                <div
                  key={wg.id}
                  className="flex items-center gap-2 rounded border border-border bg-panel-subtle px-2 py-1.5"
                >
                  <span className="min-w-0 flex-1 truncate text-table font-medium text-text">{wg.name}</span>
                  <StatusPill
                    status={wg.scope === 'inter-org' ? 'info' : 'success'}
                    label={wg.scope}
                  />
                  <StatusPill
                    status={wg.state === 'active' ? 'active' : wg.state === 'pending' ? 'info' : 'danger'}
                    label={wg.state}
                  />
                </div>
              ))}
            </div>
          )}
        </div>
        <div>
          <p className="mb-1.5 text-[0.6rem] font-semibold uppercase tracking-wide text-text-mute">
            Advertisements
          </p>
          {orgAds.length === 0 ? (
            <p className="text-label text-text-mute-2">No advertisements found</p>
          ) : (
            <div className="space-y-1">
              {orgAds.map((ad) => (
                <div
                  key={ad.id}
                  className="flex items-center gap-2 rounded border border-border bg-panel-subtle px-2 py-1.5"
                >
                  <span className="min-w-0 flex-1 truncate text-table font-medium text-text">{ad.name}</span>
                  <StatusPill
                    status={ad.status === 'active' ? 'active' : 'neutral'}
                    label={ad.status}
                  />
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function agentRelativeTime(isoString: string): string {
  const date = new Date(isoString);
  if (Number.isNaN(date.getTime())) return '';
  const seconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

// presentation tone for a feed row, ordered by severity
type FeedTone = 'positive' | 'neutral' | 'warning' | 'danger';

type FeedEventMeta = {
  label: string;
  icon: LucideIcon;
  tone: FeedTone;
};

const CLOSE_REASON_LABELS: Partial<Record<string, string>> = {
  consumer_close: 'consumer closed',
  provider_close: 'provider closed',
  admin_close: 'admin closed',
  tunnel_failed: 'tunnel failed',
};

// derive a human label, icon, and severity tone from the fields already on the
// audit event — presentation only, no extra data is fetched.
function feedEventMeta(event: AuditEvent): FeedEventMeta {
  switch (event.eventType) {
    case 'session.proposed':
      return { label: 'Session proposed', icon: ArrowLeftRight, tone: 'neutral' };
    case 'session.accepted':
      return { label: 'Session accepted', icon: Check, tone: 'positive' };
    case 'session.rejected':
      return { label: 'Session rejected', icon: Ban, tone: 'warning' };
    case 'session.closed': {
      const reason = dataString(event.data, 'close_reason');
      if (reason === 'contract_violation') {
        return { label: 'Session closed — contract violation', icon: ShieldAlert, tone: 'danger' };
      }
      const detail = CLOSE_REASON_LABELS[reason];
      return {
        label: detail ? `Session closed — ${detail}` : 'Session closed',
        icon: ArrowLeftRight,
        tone: 'neutral',
      };
    }
    case 'envelope.flowed':
      return { label: 'Envelope flowed', icon: Mail, tone: 'positive' };
    case 'advertisement.published':
      return { label: 'Advertisement published', icon: Megaphone, tone: 'neutral' };
    case 'advertisement.retracted':
      return { label: 'Advertisement retracted', icon: Megaphone, tone: 'neutral' };
    case 'environment.heartbeat':
      return { label: 'Environment heartbeat', icon: Activity, tone: 'neutral' };
    case 'account.login':
      return { label: 'Account login', icon: LogIn, tone: 'neutral' };
    case 'account.login_failed':
      return { label: 'Login failed', icon: AlertTriangle, tone: 'warning' };
    default:
      return { label: formatEventType(event.eventType), icon: Activity, tone: 'neutral' };
  }
}

function feedToneBorder(tone: FeedTone): string {
  switch (tone) {
    case 'positive':
      return 'var(--color-success)';
    case 'warning':
      return 'var(--color-warning)';
    case 'danger':
      return 'var(--color-danger)';
    default:
      return 'var(--color-border)';
  }
}

function feedToneChipClass(tone: FeedTone): string {
  switch (tone) {
    case 'positive':
      return 'bg-success/15 text-success-strong';
    case 'warning':
      return 'bg-warning/15 text-warning-strong';
    case 'danger':
      return 'bg-danger/15 text-danger';
    default:
      return 'bg-panel-subtle text-text-mute';
  }
}

function LiveEventEntry({ event, workgroupName }: { event: AuditEvent; workgroupName?: string }) {
  const meta = feedEventMeta(event);
  const Icon = meta.icon;

  return (
    <article
      className="flex items-center gap-3 border-l-2 py-2.5 pl-3 pr-4 [animation:feedIn_0.25s_ease-out]"
      style={{ borderLeftColor: feedToneBorder(meta.tone) }}
    >
      <span className={`flex size-7 shrink-0 items-center justify-center rounded-pill ${feedToneChipClass(meta.tone)}`}>
        <Icon size={13} aria-hidden="true" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-table font-medium text-text">{meta.label}</p>
        {workgroupName ? (
          <span className="mt-1 inline-flex max-w-full items-center rounded-status border border-border bg-panel-subtle px-2 py-0.5 text-[0.625rem] font-medium text-text-mute">
            <span className="truncate">{workgroupName}</span>
          </span>
        ) : null}
      </div>
      <time className="shrink-0 text-label tabular-nums text-text-mute-2">{agentRelativeTime(event.occurredAt)}</time>
    </article>
  );
}

// honest live indicator: pulses green only when the feed resource has loaded
// without error; muted while loading and red on error.
function LiveIndicator({ state }: { state: 'live' | 'loading' | 'error' }) {
  if (state === 'live') {
    return (
      <span className="flex items-center gap-1.5 text-[0.6875rem] font-medium text-text-mute">
        <span className="relative flex h-1.5 w-1.5 shrink-0">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-success opacity-75" aria-hidden="true" />
          <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-success" />
        </span>
        Live
      </span>
    );
  }

  return (
    <span className="flex items-center gap-1.5 text-[0.6875rem] font-medium text-text-mute-2">
      <span
        className={`h-1.5 w-1.5 shrink-0 rounded-full ${state === 'error' ? 'bg-danger' : 'bg-text-mute-2'}`}
        aria-hidden="true"
      />
      {state === 'error' ? 'Offline' : 'Connecting'}
    </span>
  );
}

function LiveActivitySkeleton() {
  return (
    <div className="flex flex-col divide-y divide-border">
      {Array.from({ length: 8 }, (_, i) => (
        <div key={i} className="flex items-start gap-3 border-l-2 border-l-border-light py-2.5 pl-3 pr-4">
          <div className="min-w-0 flex-1 space-y-1.5">
            <div className="h-3 w-3/4 animate-pulse rounded bg-panel-subtle" aria-hidden="true" />
            <div className="h-2.5 w-1/2 animate-pulse rounded bg-panel-subtle" aria-hidden="true" />
          </div>
          <div className="h-2.5 w-10 animate-pulse rounded bg-panel-subtle" aria-hidden="true" />
        </div>
      ))}
    </div>
  );
}

function LiveActivityContent({
  events,
  loading,
  workgroupNames,
}: {
  events: AuditEvent[] | undefined;
  loading: boolean;
  workgroupNames: Record<string, string>;
}) {
  // the rail is full-height now, so show the whole fetched window and let it scroll
  // internally rather than capping the feed to a fixed handful of rows.
  const recent = useMemo(() => events ?? [], [events]);

  if (loading && !events) {
    return <LiveActivitySkeleton />;
  }

  if (recent.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center p-4">
        <p className="text-table text-text-mute-2">No recent activity</p>
      </div>
    );
  }

  return (
    <div className="flex-1 divide-y divide-border overflow-auto">
      {recent.map((event) => (
        <LiveEventEntry
          key={event.id}
          event={event}
          workgroupName={event.workgroupId ? (workgroupNames[event.workgroupId] ?? event.workgroupId) : undefined}
        />
      ))}
    </div>
  );
}
