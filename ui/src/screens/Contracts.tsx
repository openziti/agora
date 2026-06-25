import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import {
  AlertTriangle,
  Building2,
  ChevronDown,
  ChevronUp,
  Clock,
  Compass,
  FileCheck2,
  FileText,
  Hash,
  Lock,
  MessageSquare,
  RefreshCcw,
  Search,
  ShieldCheck,
  Wifi,
} from 'lucide-react';

import { AppShell, DrawerCard, DrawerCodeChip, DrawerDivider, DrawerTip, EmptyState, InfoDrawer, Input, PageHeader, SectionPanel, StatCard, StatusPill } from '../components';
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
  const [infoOpen, setInfoOpen] = useState(false);
  const [search, setSearch] = useState('');
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
  const orgNameById = useMemo(
    () => new Map((data?.advertisements ?? []).map((ad) => [ad.organizationId, ad.organizationName])),
    [data?.advertisements],
  );
  const filteredContracts = useMemo(() => {
    if (!data) return [];
    const q = search.trim().toLowerCase();
    if (!q) return data.contracts;
    return data.contracts.filter(({ contract }) =>
      contract.name.toLowerCase().includes(q) ||
      contract.id.toLowerCase().includes(q) ||
      (contract.description ?? '').toLowerCase().includes(q),
    );
  }, [data, search]);

  const [expandedId, setExpandedId] = useState<string | undefined>(undefined);

  useEffect(() => {
    setExpandedId((prev) => prev ?? filteredContracts[0]?.contract.id);
  }, [filteredContracts]);

  const groupedContracts = useMemo(() => {
    const groups = new Map<string, { orgId: string; orgName: string; contracts: VisibleContract[] }>();
    filteredContracts.forEach((entry) => {
      const orgId = entry.contract.organizationId;
      const existing = groups.get(orgId);
      if (existing) {
        existing.contracts.push(entry);
      } else {
        groups.set(orgId, { orgId, orgName: orgNameById.get(orgId) ?? orgId, contracts: [entry] });
      }
    });
    return Array.from(groups.values());
  }, [filteredContracts, orgNameById]);

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
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'Connected'}
      userInitials={callerAccount ? initialsFromEmail(callerAccount.email) : '--'}
      userLabel={callerAccount?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        <PageHeader
          icon={FileCheck2}
          label="GOVERNANCE"
          title="Contracts"
          description="Engagement terms attached to every session — defining allowed message types, time limits, and envelope counts, enforced by the controller, not by the agents themselves."
          onInfoClick={() => setInfoOpen(true)}
        />
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

        <SectionPanel title="Visible Contracts" bodyClassName="p-0" className="overflow-hidden">
          {!callerAccount || visibleContracts.loading ? (
            <div className="p-5">
              <LoadingPanel title="Loading contracts" compact />
            </div>
          ) : data && data.contracts.length > 0 ? (
            <>
              <div className="border-b border-border py-4">
                <label className="relative block min-w-0">
                  <span className="sr-only">Search contracts</span>
                  <Search size={17} aria-hidden="true" className="absolute left-3 top-1/2 -translate-y-1/2 text-text-mute" />
                  <Input
                    type="search"
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    placeholder="Search contracts by name, ID, or description"
                    className="pl-10 pr-3"
                  />
                </label>
              </div>
              {filteredContracts.length > 0 ? (
                <div>
                  {groupedContracts.map((group) => (
                    <div key={group.orgId}>
                      <div className="border-b border-border bg-panel-subtle px-4 py-2">
                        <p className="text-[0.6875rem] font-medium uppercase tracking-[0.04em] text-text-mute-strong">
                          {group.orgName}
                        </p>
                      </div>
                      {group.contracts.map((entry) => (
                        <ContractAccordionRow
                          key={entry.contract.id}
                          entry={entry}
                          orgNameById={orgNameById}
                          isExpanded={expandedId === entry.contract.id}
                          onToggle={() =>
                            setExpandedId(
                              expandedId === entry.contract.id ? undefined : entry.contract.id,
                            )
                          }
                        />
                      ))}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="p-5">
                  <EmptyState
                    icon={FileText}
                    title="No matching contracts"
                    description="No contracts match your search."
                  />
                </div>
              )}
            </>
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

      {infoOpen ? (
        <InfoDrawer title="About Contracts" onClose={() => setInfoOpen(false)}>
          <div className="flex flex-col gap-5">
            <section>
              <h3 className="mb-2 font-semibold text-text">What is a Contract?</h3>
              <p className="leading-relaxed text-text-mute">
                A Contract defines the engagement terms for a session before it begins. Contracts are
                evaluated by the controller at session establishment time and enforced throughout the
                session. The agents themselves do not need to implement any enforcement logic.
              </p>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-3 font-semibold text-text">Contract Terms</h3>
              <div className="flex flex-col gap-2">
                <DrawerCard icon={Clock} title="max_duration_seconds" description={<>Maximum session lifetime. Exceeded sessions are closed by the controller with a recorded close reason. Field: <DrawerCodeChip>max_duration_seconds</DrawerCodeChip></>} />
                <DrawerCard icon={Hash} title="max_envelope_count" description={<>Maximum number of envelopes that can be exchanged. Attempts beyond this limit are rejected. Field: <DrawerCodeChip>max_envelope_count</DrawerCodeChip></>} />
                <DrawerCard icon={MessageSquare} title="allowed_message_types" description={<>The set of message types permitted in this session. Any envelope with a type outside this set is rejected at the controller. Field: <DrawerCodeChip>allowed_message_types</DrawerCodeChip></>} />
                <DrawerCard icon={ShieldCheck} title="Required workgroup memberships" description="Preconditions that must be satisfied for the session to be established." />
                <DrawerCard icon={Lock} title="access_mode" description={<>Controls how sessions are established. <DrawerCodeChip>approval_required</DrawerCodeChip> means the provider must explicitly accept before the session goes active.</>} />
              </div>
            </section>

            <DrawerDivider />

            <DrawerTip>
              Providers do not need to be online to decide whether to accept an engagement — the contract speaks for them. Governance is structural, not dependent on agent-side logic.
            </DrawerTip>
          </div>
        </InfoDrawer>
      ) : null}
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

function ContractAccordionRow({
  entry,
  orgNameById,
  isExpanded,
  onToggle,
}: {
  entry: VisibleContract;
  orgNameById: Map<string, string>;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const { contract, advertisements } = entry;
  const orgName = orgNameById.get(contract.organizationId);

  return (
    <div className="border-b border-border last:border-b-0">
      <button
        type="button"
        className="flex w-full flex-col gap-2 px-4 py-3 text-left hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-brand-agora sm:flex-row sm:items-center sm:gap-4"
        onClick={onToggle}
        aria-expanded={isExpanded}
      >
        <div className="flex min-w-0 items-center justify-between gap-2 sm:w-48 sm:shrink-0 sm:min-w-0">
          <div className="min-w-0 flex-1">
            <p className="font-semibold text-text">{contract.name}</p>
            <p className="truncate font-mono text-[0.6875rem] text-text-mute-2">{contract.id}</p>
          </div>
          <span className="shrink-0 sm:hidden" aria-hidden="true">
            {isExpanded
              ? <ChevronUp size={16} className="text-text-mute" />
              : <ChevronDown size={16} className="text-text-mute" />}
          </span>
        </div>
        <p className="min-w-0 text-[0.76rem] text-text-mute sm:flex-1 sm:truncate">
          {contract.description ?? '—'}
        </p>
        <div className="flex flex-wrap items-center gap-2 sm:shrink-0 sm:flex-nowrap">
          <StatusPill status={accessModeStatus(contract.accessMode)} label={formatAccessMode(contract.accessMode)} />
          <StatusPill status="neutral" label={`schema v${contract.schemaVersion}`} />
          {isExpanded
            ? <ChevronUp size={16} className="hidden shrink-0 text-text-mute sm:block" aria-hidden="true" />
            : <ChevronDown size={16} className="hidden shrink-0 text-text-mute sm:block" aria-hidden="true" />}
        </div>
      </button>

      <div
        className={[
          'overflow-hidden bg-panel-subtle transition-[max-height] duration-200 ease-in-out',
          isExpanded ? 'max-h-[3000px]' : 'max-h-0',
        ].join(' ')}
      >
        <div className="px-4 py-3">
          <dl className="grid gap-2 md:grid-cols-2 xl:grid-cols-4">
            <Term label="max duration" value={formatDuration(contract.maxDurationSeconds)} />
            <Term label="max envelopes" value={formatInteger(contract.maxEnvelopeCount)} />
            <Term label="max bytes" value={formatBytes(contract.maxEnvelopeBytes)} />
            <Term label="maturity" value={formatMaturity(contract)} />
            <Term
              label="message types"
              value={
                isWildcardMessageTypes(contract.allowedMessageTypes)
                  ? <span className="text-text-mute">All message types permitted</span>
                  : <TokenList values={contract.allowedMessageTypes} emptyLabel="any" />
              }
            />
            <Term
              label="workgroups"
              value={<TokenList values={contract.requiredWorkgroupMemberships} emptyLabel="none" mono />}
            />
            <Term label="owner organization" value={<IdWithName name={orgName} id={contract.organizationId} />} />
            <Term label="owner account" value={<IdWithName name={undefined} id={contract.accountId} />} />
          </dl>

          {advertisements.length > 0 ? (
            <section className="mt-3">
              <p className="text-label font-medium uppercase text-text-mute">
                Referenced Advertisements ({advertisements.length})
              </p>
              <div className="mt-2 grid gap-2 md:grid-cols-2 xl:grid-cols-4">
                {advertisements.map((advertisement) => (
                  <AdvertisementReference key={advertisement.id} advertisement={advertisement} />
                ))}
              </div>
            </section>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function Term({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="rounded-card border border-border bg-panel p-2">
      <dt className="text-label font-medium uppercase text-text-mute">{label}</dt>
      <dd className="mt-0.5 break-words text-body font-medium text-text">{value}</dd>
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

function IdWithName({ name, id }: { name?: string; id: string }) {
  if (name) {
    return (
      <span>
        <span className="block">{name}</span>
        <span className="block font-mono text-table text-text-mute">{id}</span>
      </span>
    );
  }
  return <span className="font-mono">{id}</span>;
}

function AdvertisementReference({ advertisement }: { advertisement: Advertisement }) {
  return (
    <div className="rounded-card border border-border bg-panel-subtle p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-body font-semibold text-text">{advertisement.name}</p>
          <p className="mt-0.5 break-all text-table text-text-mute">
            Advertisement ID: <span className="font-mono">{advertisement.id}</span>
          </p>
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

function isWildcardMessageTypes(values: string[] | null | undefined): boolean {
  if (!values || values.length === 0) return true;
  return values.length === 1 && values[0] === '*';
}
