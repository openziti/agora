import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import {
  Activity,
  AlertTriangle,
  Boxes,
  Check,
  Clock3,
  Copy,
  Eye,
  EyeOff,
  Gauge,
  RefreshCcw,
  Server,
  Terminal,
  Users,
  Wifi,
  X,
} from 'lucide-react';

import {
  AppShell,
  BarChart,
  DataTable,
  EmptyState,
  SectionPanel,
  SidebarBreakdown,
  StatCard,
  StatusPill,
  type BarChartDatum,
  type DataTableColumn,
  type SidebarBreakdownItem,
} from '../components';
import {
  ApiError,
  getAccountToken,
  getDashboardActivity,
  getDashboardEnvironments,
  getDashboardSummary,
  useApiResource,
  type DashboardActivityResponse,
  type DashboardBucket,
  type DashboardEnvironment,
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

const environmentColumns: DataTableColumn<DashboardEnvironment>[] = [
  {
    id: 'name',
    header: 'Environment',
    accessor: (environment) => environment.name,
    sortable: true,
  },
  {
    id: 'status',
    header: 'Status',
    accessor: (environment) => ({ status: environment.status, label: environment.status }),
    kind: 'pill',
    sortable: true,
  },
  {
    id: 'lastHeartbeatAt',
    header: 'Last heartbeat',
    accessor: (environment) => formatDateTime(environment.lastHeartbeatAt),
    sortable: true,
    sortValue: (environment) => timestampSortValue(environment.lastHeartbeatAt),
  },
  {
    id: 'accountId',
    header: 'Owning account',
    accessor: (environment) => environment.accountId,
    kind: 'mono',
    sortable: true,
  },
];

const numberFormatter = new Intl.NumberFormat();

const DASHBOARD_POLL_MS = 5000;

export default function Dashboard() {
  const navigate = useNavigate();
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
  const environments = useApiResource(getDashboardEnvironments, { intervalMs: DASHBOARD_POLL_MS });
  const account = summary.data?.account;
  const hasError = Boolean(summary.error || activity.error || environments.error);
  const isLoading =
    (summary.loading && !summary.data) ||
    (activity.loading && !activity.data) ||
    (environments.loading && !environments.data);

  const chartData = useMemo(
    () => toChartData(activity.data, activityWindow),
    [activity.data, activityWindow],
  );
  const workgroupBreakdown = useMemo(
    () => toWorkgroupBreakdown(activity.data),
    [activity.data],
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
      organizationName={account?.organizationName ?? 'Loading organization'}
      activeTab="dashboard"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'All systems operational'}
      userInitials={account ? initialsForEmail(account.email) : '--'}
      userLabel={account?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        {summary.error ? (
          <ErrorPanel title="Dashboard summary unavailable" error={summary.error} onRetry={summary.refetch} />
        ) : summary.data ? (
          <AccountCallout summary={summary.data} />
        ) : (
          <LoadingPanel title="Loading current account" />
        )}

        {summary.error ? null : summary.data ? <SummaryStats summary={summary.data} /> : <StatsLoading />}

        <div className="grid gap-4 xl:grid-cols-[1fr_22rem]">
          <SectionPanel
            title="Envelope Flow"
            actions={
              <select
                value={activityWindow}
                onChange={(event) => {
                  setActivityWindow(event.target.value as DashboardWindow);
                  activity.refetch();
                }}
                className="h-9 rounded-pill border border-border bg-panel-subtle px-3 text-table font-medium text-text-mute-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
                aria-label="Activity window"
              >
                {activityWindowOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            }
          >
            {activity.error ? (
              <ErrorPanel title="Activity unavailable" error={activity.error} onRetry={activity.refetch} compact />
            ) : activity.loading && !activity.data ? (
              <LoadingPanel title="Loading activity" compact />
            ) : chartData.length > 0 ? (
              <BarChart data={chartData} accent="agora" />
            ) : (
              <EmptyState
                icon={Activity}
                title="No envelope activity"
                description="No envelope flow has been recorded in this window."
              />
            )}
          </SectionPanel>

          <SectionPanel title="Top Workgroups">
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

        <SectionPanel title="Environment Status" bodyClassName="p-0">
          {environments.error ? (
            <div className="p-5">
              <ErrorPanel title="Environments unavailable" error={environments.error} onRetry={environments.refetch} compact />
            </div>
          ) : environments.loading && !environments.data ? (
            <div className="p-5">
              <LoadingPanel title="Loading environments" compact />
            </div>
          ) : (
            <DataTable
              columns={environmentColumns}
              rows={environments.data ?? []}
              getRowKey={(environment) => environment.id}
              className="rounded-none border-0"
              emptyState={
                <div className="p-5">
                  <EmptyState
                    icon={Server}
                    title="No environments"
                    description="No environments are registered for this organization."
                  />
                </div>
              }
            />
          )}
        </SectionPanel>
      </div>
    </AppShell>
  );
}

function AccountCallout({ summary }: { summary: DashboardSummaryResponse }) {
  const [cliOpen, setCliOpen] = useState(false);

  return (
    <section className="rounded-card border border-border bg-panel p-5">
      <div className="grid gap-5 lg:grid-cols-[1fr_1.4fr]">
        <div className="min-w-0">
          <p className="text-label font-medium uppercase text-text-mute">Current account</p>
          <h1 className="mt-2 truncate text-2xl font-semibold text-text">{summary.account.email}</h1>
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <StatusPill status="info" label={summary.account.organizationName} />
            <StatusPill status="neutral" label={formatRole(summary.account.role)} />
            <button
              type="button"
              onClick={() => setCliOpen(true)}
              className="inline-flex h-7 items-center gap-1.5 rounded-pill border border-border bg-panel-subtle px-3 text-label font-medium text-text-mute-strong hover:bg-panel focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            >
              <Terminal className="h-3.5 w-3.5" aria-hidden="true" />
              CLI access
            </button>
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <RibbonMetric label="workgroups" value={summary.ribbon.workgroupCount} />
          <RibbonMetric label="advertisements" value={summary.ribbon.advertisementCount} />
          <RibbonMetric label="sessions today" value={summary.ribbon.sessionsToday} />
          <RibbonMetric label="environments" value={summary.ribbon.environmentCount} />
        </div>
      </div>

      {cliOpen && <CliAccessModal onClose={() => setCliOpen(false)} />}
    </section>
  );
}

const TOKEN_MASK = '••••••••••••••••••••••••••••••••';

function CliAccessModal({ onClose }: { onClose: () => void }) {
  const [token, setToken] = useState<string | null>(null);
  const [revealed, setRevealed] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<'token' | 'command' | null>(null);

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

  const flashCopied = useCallback((field: 'token' | 'command') => {
    setCopied(field);
    setTimeout(() => {
      setCopied((current) => (current === field ? null : current));
    }, 1500);
  }, []);

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
      className="fixed inset-0 z-40 flex items-center justify-center bg-text/30 p-4"
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

        <div className="flex flex-col gap-4 p-5">
          <p className="text-table text-text-mute">
            Use this account token to enable a local environment with{' '}
            <code className="font-mono text-text">agora enable</code>.
          </p>

          <div className="grid gap-2">
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
          </div>

          {error && <p className="text-label text-danger">{error}</p>}
        </div>
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
    <div className="flex items-center gap-3 rounded-pill border border-border bg-panel-subtle px-3 py-2">
      <span className="w-28 shrink-0 text-label font-medium uppercase text-text-mute">{label}</span>
      <span className="min-w-0 flex-1 truncate font-mono text-table text-text">{value}</span>
      {reveal && (
        <button
          type="button"
          onClick={reveal.onToggle}
          disabled={reveal.loading}
          aria-label={reveal.revealed ? 'Hide account token' : 'Reveal account token'}
          className="inline-flex h-8 w-8 items-center justify-center rounded-pill text-text-mute hover:bg-panel hover:text-text focus-visible:outline-brand-agora disabled:opacity-50"
        >
          {reveal.revealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      )}
      <button
        type="button"
        onClick={onCopy}
        aria-label={copied ? 'Copied' : copyAriaLabel}
        className="inline-flex h-8 w-8 items-center justify-center rounded-pill text-text-mute hover:bg-panel hover:text-text focus-visible:outline-brand-agora"
      >
        {copied ? <Check className="h-4 w-4 text-success" /> : <Copy className="h-4 w-4" />}
      </button>
    </div>
  );
}

function RibbonMetric({ label, value }: { label: string; value: number }) {
  return (
    <div className="border-l border-border pl-4">
      <p className="text-label font-medium uppercase text-text-mute">{label}</p>
      <p className="mt-1 text-section font-semibold text-text">{formatInteger(value)}</p>
    </div>
  );
}

function SummaryStats({ summary }: { summary: DashboardSummaryResponse }) {
  const sessionDelta = summary.stats.activeSessionsDelta7d;
  const envelopeDelta = summary.stats.envelopesToday - summary.stats.envelopesYesterday;

  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <StatCard
        label="Active Sessions"
        value={formatInteger(summary.stats.activeSessions)}
        icon={Activity}
        accent="agora"
        delta={{
          direction: deltaDirection(sessionDelta),
          value: formatSignedInteger(sessionDelta),
          label: 'vs 7d',
        }}
      />
      <StatCard
        label="Envelopes Today"
        value={formatInteger(summary.stats.envelopesToday)}
        icon={Wifi}
        accent="success"
        delta={{
          direction: deltaDirection(envelopeDelta),
          value: formatSignedInteger(envelopeDelta),
          label: 'vs prior 24h',
        }}
      />
      <StatCard
        label="Active Workgroups"
        value={formatInteger(summary.stats.activeWorkgroups)}
        icon={Boxes}
        accent="info"
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
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      {['Active Sessions', 'Envelopes Today', 'Active Workgroups', 'Active Tunnels'].map((label) => (
        <section key={label} className="flex min-h-32 flex-col justify-between rounded-card border border-border bg-panel p-4">
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

function formatDateTime(value: string | undefined): string {
  if (!value) {
    return 'never';
  }

  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(date);
}

function timestampSortValue(value: string | undefined): number {
  if (!value) {
    return 0;
  }

  const timestamp = Date.parse(value);

  return Number.isNaN(timestamp) ? 0 : timestamp;
}

function formatInteger(value: number): string {
  return numberFormatter.format(value);
}

function formatSignedInteger(value: number): string {
  if (value === 0) {
    return '0';
  }

  return `${value > 0 ? '+' : ''}${formatInteger(value)}`;
}

function deltaDirection(value: number): 'up' | 'down' | 'flat' {
  if (value > 0) {
    return 'up';
  }

  if (value < 0) {
    return 'down';
  }

  return 'flat';
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
