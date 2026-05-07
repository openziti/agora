import { useCallback, useMemo, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import {
  AlertTriangle,
  Building2,
  Compass,
  FileText,
  RefreshCcw,
  ShieldCheck,
  Wifi,
} from 'lucide-react';

import { AppShell, EmptyState, SectionPanel, StatCard, StatusPill } from '../components';
import {
  ApiError,
  fetchAllVisibleAdvertisements,
  getContract,
  getDashboardSummary,
  useApiResource,
  type Advertisement,
  type Contract,
  type ContractAccessMode,
} from '../lib/api';

type VisibleContractsData = {
  advertisements: Advertisement[];
  contracts: VisibleContract[];
};

type VisibleContract = {
  contract: Contract;
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

const numberFormatter = new Intl.NumberFormat();

export default function Contracts() {
  const navigate = useNavigate();
  const account = useApiResource(getDashboardSummary);
  const visibleContractsLoad = useCallback((signal: AbortSignal) => loadVisibleContracts(signal), []);
  const visibleContracts = useApiResource(visibleContractsLoad);
  const callerAccount = account.data?.account;
  const hasError = Boolean(account.error || visibleContracts.error);
  const isLoading = account.loading || visibleContracts.loading;
  const data = visibleContracts.data;
  const ownerOrganizationCount = useMemo(
    () => new Set(data?.contracts.map((entry) => entry.contract.organizationId) ?? []).size,
    [data?.contracts],
  );

  function handleTabChange(tabId: string) {
    const route = routeByTab[tabId];

    if (route) {
      navigate(route);
    }
  }

  function refetchAll() {
    account.refetch();
    visibleContracts.refetch();
  }

  return (
    <AppShell
      product="agora"
      organizationName={callerAccount?.organizationName ?? 'Loading organization'}
      activeTab="contracts"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'All systems operational'}
      userInitials={callerAccount ? initialsFromEmail(callerAccount.email) : '--'}
      userLabel={callerAccount?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        {account.error ? <ErrorPanel title="Current account unavailable" error={account.error} onRetry={account.refetch} /> : null}

        <ContractsOverview
          contractCount={data?.contracts.length ?? 0}
          referencedAdvertisementCount={data?.contracts.reduce((total, entry) => total + entry.advertisements.length, 0) ?? 0}
          visibleAdvertisementCount={data?.advertisements.length ?? 0}
          ownerOrganizationCount={ownerOrganizationCount}
          loading={!callerAccount || isLoading}
        />

        {visibleContracts.error ? (
          <ErrorPanel title="Contracts unavailable" error={visibleContracts.error} onRetry={refetchAll} />
        ) : null}

        <SectionPanel title="Visible Contracts" bodyClassName="p-0">
          {!callerAccount || visibleContracts.loading ? (
            <div className="p-5">
              <LoadingPanel title="Loading contracts" compact />
            </div>
          ) : data && data.contracts.length > 0 ? (
            <div className="divide-y divide-border">
              {data.contracts.map((entry) => (
                <ContractCard key={entry.contract.id} entry={entry} />
              ))}
            </div>
          ) : (
            <div className="p-5">
              <EmptyState
                icon={FileText}
                title="No contracts"
                description="No visible advertisements reference a contract."
              />
            </div>
          )}
        </SectionPanel>
      </div>
    </AppShell>
  );
}

function ContractsOverview({
  contractCount,
  referencedAdvertisementCount,
  visibleAdvertisementCount,
  ownerOrganizationCount,
  loading,
}: {
  contractCount: number;
  referencedAdvertisementCount: number;
  visibleAdvertisementCount: number;
  ownerOrganizationCount: number;
  loading: boolean;
}) {
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <StatCard
        label="Visible Contracts"
        value={loading ? '-' : formatInteger(contractCount)}
        icon={FileText}
        accent="agora"
      />
      <StatCard
        label="Referenced Ads"
        value={loading ? '-' : formatInteger(referencedAdvertisementCount)}
        icon={Compass}
        accent="info"
      />
      <StatCard
        label="Visible Ads"
        value={loading ? '-' : formatInteger(visibleAdvertisementCount)}
        icon={ShieldCheck}
        accent="success"
      />
      <StatCard
        label="Owning Orgs"
        value={loading ? '-' : formatInteger(ownerOrganizationCount)}
        icon={Building2}
        accent="llm"
      />
    </div>
  );
}

function ContractCard({ entry }: { entry: VisibleContract }) {
  const { contract, advertisements } = entry;

  return (
    <article className="p-5">
      <header className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_auto]">
        <div className="min-w-0">
          <p className="text-label font-medium uppercase text-text-mute">Contract</p>
          <h2 className="mt-1 truncate text-section font-semibold text-text">{contract.name}</h2>
          {contract.description ? <p className="mt-2 max-w-3xl text-body text-text-mute">{contract.description}</p> : null}
          <code className="mt-3 block break-all font-mono text-table text-text-mute-strong">{contract.id}</code>
        </div>
        <div className="flex flex-wrap items-start gap-2 xl:justify-end">
          <StatusPill status={accessModeStatus(contract.accessMode)} label={formatAccessMode(contract.accessMode)} />
          <StatusPill status="neutral" label={`schema v${contract.schemaVersion}`} />
        </div>
      </header>

      <dl className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <Term label="max duration" value={formatDuration(contract.maxDurationSeconds)} />
        <Term label="max envelopes" value={formatInteger(contract.maxEnvelopeCount)} />
        <Term label="max bytes" value={formatBytes(contract.maxEnvelopeBytes)} />
        <Term label="maturity" value={formatMaturity(contract)} />
        <Term label="message types" value={<TokenList values={contract.allowedMessageTypes} emptyLabel="any" />} />
        <Term
          label="workgroups"
          value={<TokenList values={contract.requiredWorkgroupMemberships} emptyLabel="none" mono />}
        />
        <Term label="owner organization" value={<span className="font-mono">{contract.organizationId}</span>} />
        <Term label="owner account" value={<span className="font-mono">{contract.accountId}</span>} />
      </dl>

      <section className="mt-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-label font-medium uppercase text-text-mute">Referenced Advertisements</p>
          <StatusPill status="info" label={formatInteger(advertisements.length)} />
        </div>
        <div className="mt-3 grid gap-3 lg:grid-cols-2">
          {advertisements.map((advertisement) => (
            <AdvertisementReference key={advertisement.id} advertisement={advertisement} />
          ))}
        </div>
      </section>
    </article>
  );
}

function Term({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="rounded-card border border-border bg-panel-subtle p-3">
      <dt className="text-label font-medium uppercase text-text-mute">{label}</dt>
      <dd className="mt-1 break-words text-body font-medium text-text">{value}</dd>
    </div>
  );
}

function TokenList({ values, emptyLabel, mono = false }: { values: string[]; emptyLabel: string; mono?: boolean }) {
  if (values.length === 0) {
    return <span>{emptyLabel}</span>;
  }

  return (
    <span className="flex flex-wrap gap-2">
      {values.map((value) => (
        <span
          key={value}
          className={[
            'inline-flex max-w-full items-center rounded-status border border-border bg-panel px-2 py-0.5 text-table text-text-mute-strong',
            mono ? 'font-mono' : '',
          ]
            .filter(Boolean)
            .join(' ')}
        >
          <span className="truncate">{value}</span>
        </span>
      ))}
    </span>
  );
}

function AdvertisementReference({ advertisement }: { advertisement: Advertisement }) {
  return (
    <div className="rounded-card border border-border bg-panel-subtle p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-body font-semibold text-text">{advertisement.name}</p>
          <p className="mt-1 truncate text-table text-text-mute">{advertisement.organizationName}</p>
        </div>
        <StatusPill status="info" label={advertisement.tunnelMode ?? 'tcp'} className="shrink-0" />
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        {advertisement.capabilities.slice(0, 4).map((capability) => (
          <span
            key={capability.name}
            className="inline-flex max-w-full items-center rounded-status border border-brand-agora/30 bg-brand-agora/10 px-2 py-0.5 text-table font-medium text-brand-agora"
          >
            <span className="truncate">{capability.name}</span>
          </span>
        ))}
      </div>
      <code className="mt-3 block break-all font-mono text-table text-text-mute-strong">{advertisement.id}</code>
    </div>
  );
}

async function loadVisibleContracts(signal: AbortSignal): Promise<VisibleContractsData> {
  const advertisements = await fetchAllVisibleAdvertisements(signal);
  const advertisementsByContractId = groupAdvertisementsByContractId(advertisements);
  const contractIds = Array.from(advertisementsByContractId.keys()).sort();

  const contracts = await Promise.all(contractIds.map((contractId) => getContract(contractId, signal)));

  return {
    advertisements,
    contracts: contracts
      .map((contract) => ({
        contract,
        advertisements: advertisementsByContractId.get(contract.id) ?? [],
      }))
      .sort((left, right) => left.contract.name.localeCompare(right.contract.name) || left.contract.id.localeCompare(right.contract.id)),
  };
}

function groupAdvertisementsByContractId(advertisements: Advertisement[]): Map<string, Advertisement[]> {
  const grouped = new Map<string, Advertisement[]>();

  advertisements.forEach((advertisement) => {
    if (!advertisement.contractId) {
      return;
    }

    const group = grouped.get(advertisement.contractId) ?? [];

    group.push(advertisement);
    grouped.set(advertisement.contractId, group);
  });

  return grouped;
}

function accessModeStatus(accessMode: ContractAccessMode) {
  return accessMode === 'open' ? 'success' : 'warning';
}

function formatAccessMode(accessMode: ContractAccessMode): string {
  return accessMode.replace(/_/g, ' ');
}

function formatMaturity(contract: Contract): string {
  const minAge = contract.maturityRequirements?.minAccountAgeDays;

  if (!minAge) {
    return 'none';
  }

  return `${formatInteger(minAge)}d account age`;
}

function formatDuration(seconds: number): string {
  if (seconds < 60) {
    return `${formatInteger(seconds)}s`;
  }

  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;

  if (minutes < 60) {
    return remainingSeconds === 0
      ? `${formatInteger(minutes)}m`
      : `${formatInteger(minutes)}m ${formatInteger(remainingSeconds)}s`;
  }

  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;

  return remainingMinutes === 0
    ? `${formatInteger(hours)}h`
    : `${formatInteger(hours)}h ${formatInteger(remainingMinutes)}m`;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) {
    return `${formatInteger(bytes)} B`;
  }

  const units = ['KB', 'MB', 'GB', 'TB'];
  let value = bytes / 1024;
  let unitIndex = 0;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: value >= 10 ? 0 : 1 }).format(value)} ${units[unitIndex]}`;
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

function formatInteger(value: number): string {
  return numberFormatter.format(value);
}
