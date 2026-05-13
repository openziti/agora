import { useCallback, useMemo, useState, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import {
  AlertTriangle,
  Compass,
  FileText,
  Network,
  RefreshCcw,
  Search,
  Send,
  ShieldCheck,
  Wifi,
} from 'lucide-react';

import {
  AppShell,
  EmptyState,
  SectionPanel,
  SidebarBreakdown,
  StatCard,
  StatusPill,
  type SidebarBreakdownItem,
} from '../components';
import {
  ApiError,
  fetchAllVisibleAdvertisements,
  getContract,
  getDashboardSummary,
  listWorkgroups,
  useApiResource,
  type Advertisement,
  type AdvertisementTunnelMode,
  type Workgroup,
} from '../lib/api';

type TunnelFilter = 'all' | AdvertisementTunnelMode;
type FilterButtonOption = {
  id: string;
  label: string;
};

const routeByTab: Record<string, string> = {
  dashboard: '/',
  sessions: '/sessions',
  workgroups: '/workgroups',
  catalog: '/catalog',
  contracts: '/contracts',
  audit: '/audit',
};

const tunnelFilters: TunnelFilter[] = ['all', 'http', 'tcp', 'udp'];
const numberFormatter = new Intl.NumberFormat();

export default function Catalog() {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');
  const [tunnelFilter, setTunnelFilter] = useState<TunnelFilter>('all');
  const [workgroupFilter, setWorkgroupFilter] = useState('all');
  const [organizationFilter, setOrganizationFilter] = useState('all');
  const account = useApiResource(getDashboardSummary);
  const advertisements = useApiResource(fetchAllVisibleAdvertisements);
  const workgroups = useApiResource(listWorkgroups);
  const contractsLoad = useCallback(
    (signal: AbortSignal) => fetchContractsById(advertisements.data ?? [], signal),
    [advertisements.data],
  );
  const contracts = useApiResource(contractsLoad);
  const callerAccount = account.data?.account;
  const hasError = Boolean(account.error || advertisements.error || workgroups.error || contracts.error);
  const isLoading = account.loading || advertisements.loading || workgroups.loading || contracts.loading;

  const workgroupNameById = useMemo(
    () => new Map((workgroups.data ?? []).map((workgroup) => [workgroup.id, workgroup.name])),
    [workgroups.data],
  );
  const contractNameById = useMemo(() => contracts.data ?? new Map<string, string>(), [contracts.data]);
  const organizationOptions = useMemo(
    () => buildOrganizationOptions(advertisements.data ?? []),
    [advertisements.data],
  );
  const workgroupOptions = useMemo(
    () => buildWorkgroupOptions(advertisements.data ?? [], workgroups.data ?? []),
    [advertisements.data, workgroups.data],
  );
  const filteredAdvertisements = useMemo(
    () =>
      filterAdvertisements(
        advertisements.data ?? [],
        search,
        tunnelFilter,
        workgroupFilter,
        organizationFilter,
        workgroupNameById,
        contractNameById,
      ),
    [advertisements.data, search, tunnelFilter, workgroupFilter, organizationFilter, workgroupNameById, contractNameById],
  );
  const tunnelBreakdown = useMemo(
    () => buildTunnelBreakdown(advertisements.data ?? []),
    [advertisements.data],
  );
  const contractBreakdown = useMemo(
    () => buildContractBreakdown(advertisements.data ?? [], contractNameById),
    [advertisements.data, contractNameById],
  );

  function handleTabChange(tabId: string) {
    const route = routeByTab[tabId];

    if (route) {
      navigate(route);
    }
  }

  function refetchAll() {
    account.refetch();
    advertisements.refetch();
    workgroups.refetch();
    contracts.refetch();
  }

  return (
    <AppShell
      product="agora"
      organizationName={callerAccount?.organizationName ?? 'Loading organization'}
      activeTab="catalog"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'All systems operational'}
      userInitials={callerAccount ? initialsFromEmail(callerAccount.email) : '--'}
      userLabel={callerAccount?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        {account.error ? <ErrorPanel title="Current account unavailable" error={account.error} onRetry={account.refetch} /> : null}

        <CatalogOverview
          advertisements={advertisements.data ?? []}
          filteredCount={filteredAdvertisements.length}
          workgroupCount={workgroupOptions.length}
          loading={!callerAccount || isLoading}
        />

        {advertisements.error || workgroups.error || contracts.error ? (
          <ErrorPanel
            title="Catalog data unavailable"
            error={advertisements.error ?? workgroups.error ?? contracts.error}
            onRetry={refetchAll}
          />
        ) : null}

        <CatalogFilters
          search={search}
          tunnelFilter={tunnelFilter}
          workgroupFilter={workgroupFilter}
          organizationFilter={organizationFilter}
          workgroupOptions={workgroupOptions}
          organizationOptions={organizationOptions}
          onSearchChange={setSearch}
          onTunnelFilterChange={setTunnelFilter}
          onWorkgroupFilterChange={setWorkgroupFilter}
          onOrganizationFilterChange={setOrganizationFilter}
        />

        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
          <SectionPanel title="Visible Advertisements" bodyClassName="p-0">
            {!callerAccount || (advertisements.loading && !advertisements.data) || (workgroups.loading && !workgroups.data) ? (
              <div className="p-5">
                <LoadingPanel title="Loading catalog" compact />
              </div>
            ) : filteredAdvertisements.length > 0 ? (
              <div className="grid gap-4 p-5 lg:grid-cols-2 2xl:grid-cols-3">
                {filteredAdvertisements.map((advertisement) => (
                  <AdvertisementCard
                    key={advertisement.id}
                    advertisement={advertisement}
                    workgroupNameById={workgroupNameById}
                    contractName={contractLabel(advertisement, contractNameById)}
                  />
                ))}
              </div>
            ) : (
              <div className="p-5">
                <EmptyState
                  icon={Compass}
                  title="No advertisements"
                  description="No visible advertisements match the current catalog filters."
                />
              </div>
            )}
          </SectionPanel>

          <div className="flex flex-col gap-4">
            <SectionPanel title="Tunnel Modes">
              {advertisements.loading && !advertisements.data ? (
                <LoadingPanel title="Loading tunnel modes" compact />
              ) : tunnelBreakdown.length > 0 ? (
                <SidebarBreakdown items={tunnelBreakdown} />
              ) : (
                <EmptyState icon={Network} title="No tunnel modes" description="No tunnel mode counts are available." />
              )}
            </SectionPanel>

            <SectionPanel title="Contracts">
              {contracts.loading && !contracts.data ? (
                <LoadingPanel title="Loading contracts" compact />
              ) : contractBreakdown.length > 0 ? (
                <SidebarBreakdown items={contractBreakdown} />
              ) : (
                <EmptyState icon={FileText} title="No contracts" description="No contract breakdown is available." />
              )}
            </SectionPanel>
          </div>
        </div>
      </div>
    </AppShell>
  );
}

function CatalogOverview({
  advertisements,
  filteredCount,
  workgroupCount,
  loading,
}: {
  advertisements: Advertisement[];
  filteredCount: number;
  workgroupCount: number;
  loading: boolean;
}) {
  const contractCount = new Set(advertisements.map((advertisement) => advertisement.contractId).filter(Boolean)).size;

  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <StatCard
        label="Visible Ads"
        value={loading ? '-' : formatInteger(advertisements.length)}
        icon={Compass}
        accent="agora"
      />
      <StatCard
        label="Filtered Results"
        value={loading ? '-' : formatInteger(filteredCount)}
        icon={Search}
        accent="info"
      />
      <StatCard
        label="Workgroup Scopes"
        value={loading ? '-' : formatInteger(workgroupCount)}
        icon={ShieldCheck}
        accent="success"
      />
      <StatCard
        label="Contracts"
        value={loading ? '-' : formatInteger(contractCount)}
        icon={FileText}
        accent="llm"
      />
    </div>
  );
}

function CatalogFilters({
  search,
  tunnelFilter,
  workgroupFilter,
  organizationFilter,
  workgroupOptions,
  organizationOptions,
  onSearchChange,
  onTunnelFilterChange,
  onWorkgroupFilterChange,
  onOrganizationFilterChange,
}: {
  search: string;
  tunnelFilter: TunnelFilter;
  workgroupFilter: string;
  organizationFilter: string;
  workgroupOptions: FilterButtonOption[];
  organizationOptions: FilterButtonOption[];
  onSearchChange: (value: string) => void;
  onTunnelFilterChange: (value: TunnelFilter) => void;
  onWorkgroupFilterChange: (value: string) => void;
  onOrganizationFilterChange: (value: string) => void;
}) {
  return (
    <section className="rounded-card border border-border bg-panel p-4">
      <div className="grid gap-4 xl:grid-cols-[minmax(18rem,1fr)_auto]">
        <label className="relative block min-w-0 self-start">
          <span className="sr-only">Search catalog</span>
          <Search size={17} aria-hidden="true" className="absolute left-3 top-1/2 -translate-y-1/2 text-text-mute" />
          <input
            type="search"
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder="Search advertisements, organizations, contracts, workgroups, or capabilities"
            className="h-10 w-full rounded-pill border border-border bg-panel-subtle pl-10 pr-3 text-body text-text outline-none placeholder:text-text-mute focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
          />
        </label>

        <div className="grid grid-cols-[auto_1fr] items-center gap-x-3 gap-y-2">
          <FilterGroup label="Mode">
            {tunnelFilters.map((mode) => (
              <FilterButton
                key={mode}
                active={tunnelFilter === mode}
                label={mode === 'all' ? 'all modes' : mode}
                onClick={() => onTunnelFilterChange(mode)}
              />
            ))}
          </FilterGroup>
          <FilterGroup label="Workgroup">
            <FilterButton active={workgroupFilter === 'all'} label="all workgroups" onClick={() => onWorkgroupFilterChange('all')} />
            {workgroupOptions.map((option) => (
              <FilterButton
                key={option.id}
                active={workgroupFilter === option.id}
                label={option.label}
                onClick={() => onWorkgroupFilterChange(option.id)}
              />
            ))}
          </FilterGroup>
          <FilterGroup label="Owner">
            <FilterButton
              active={organizationFilter === 'all'}
              label="all organizations"
              onClick={() => onOrganizationFilterChange('all')}
            />
            {organizationOptions.map((option) => (
              <FilterButton
                key={option.id}
                active={organizationFilter === option.id}
                label={option.label}
                onClick={() => onOrganizationFilterChange(option.id)}
              />
            ))}
          </FilterGroup>
        </div>
      </div>
    </section>
  );
}

function AdvertisementCard({
  advertisement,
  workgroupNameById,
  contractName,
}: {
  advertisement: Advertisement;
  workgroupNameById: Map<string, string>;
  contractName: string;
}) {
  const gateway = gatewayAccent(advertisement);
  const command = `agora session propose ${advertisement.id}`;

  function copyCommand() {
    void navigator.clipboard?.writeText(command);
  }

  return (
    <article
      className={[
        'relative flex min-h-72 flex-col justify-between overflow-hidden rounded-card border bg-panel p-5',
        gateway.borderClassName,
      ].join(' ')}
    >
      {gateway.stripClassName ? <div className={`absolute inset-x-0 top-0 h-1 ${gateway.stripClassName}`} /> : null}

      <div className="min-w-0">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <p className="text-label font-medium uppercase text-text-mute">{advertisement.organizationName}</p>
            <h2 className="mt-1 truncate text-section font-semibold text-text">{advertisement.name}</h2>
          </div>
          <StatusPill status="info" label={advertisement.tunnelMode ?? 'tcp'} className="shrink-0" />
        </div>

        {advertisement.description ? (
          <p className="mt-3 line-clamp-2 text-body text-text-mute">{advertisement.description}</p>
        ) : null}

        <div className="mt-4 flex flex-wrap gap-2">
          <CompactPill label={contractName} accent="contract" />
          {advertisement.workgroupScopes.map((workgroupId) => (
            <CompactPill key={workgroupId} label={workgroupNameById.get(workgroupId) ?? workgroupId} accent="workgroup" />
          ))}
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          {advertisement.capabilities.map((capability) => (
            <CompactPill key={capability.name} label={capability.name} accent={gateway.pillAccent} />
          ))}
        </div>
      </div>

      <div className="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
        <code className="min-w-0 flex-1 break-all font-mono text-table text-text-mute-strong">{advertisement.id}</code>
        <button
          type="button"
          className="inline-flex h-9 shrink-0 items-center gap-2 rounded-pill border border-border bg-panel-subtle px-3 text-table font-medium text-text-mute-strong hover:bg-panel focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
          title={`CLI: ${command}`}
          aria-label={`Copy session proposal command for ${advertisement.name}`}
          onClick={copyCommand}
        >
          <Send size={14} aria-hidden="true" />
          Propose Session
        </button>
      </div>
    </article>
  );
}

function CompactPill({ label, accent }: { label: string; accent: 'contract' | 'workgroup' | 'agora' | 'llm' | 'mcp' }) {
  return (
    <span
      className={[
        'inline-flex max-w-full items-center rounded-status border px-3 py-1 text-table font-medium',
        compactPillClassNames[accent],
      ].join(' ')}
    >
      <span className="truncate">{label}</span>
    </span>
  );
}

const compactPillClassNames = {
  contract: 'border-info/30 bg-info/10 text-info',
  workgroup: 'border-success/30 bg-success/10 text-success-strong',
  agora: 'border-brand-agora/30 bg-brand-agora/10 text-brand-agora',
  llm: 'border-brand-llm/30 bg-brand-llm/10 text-brand-llm',
  mcp: 'border-brand-mcp/30 bg-brand-mcp/10 text-brand-mcp',
};

function FilterGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="contents">
      <span className="text-right text-label font-medium uppercase text-text-mute">{label}</span>
      <div className="flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

function FilterButton({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className={[
        'h-7 max-w-48 truncate rounded-pill border px-3 text-label font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
        active
          ? 'border-brand-agora bg-brand-agora/10 text-brand-agora'
          : 'border-border bg-panel-subtle text-text-mute-strong hover:bg-panel',
      ].join(' ')}
      aria-pressed={active}
      title={label}
      onClick={onClick}
    >
      {label}
    </button>
  );
}

async function fetchContractsById(advertisements: Advertisement[], signal: AbortSignal): Promise<Map<string, string>> {
  const contractIds = Array.from(
    new Set(advertisements.map((advertisement) => advertisement.contractId).filter((id): id is string => Boolean(id))),
  );

  if (contractIds.length === 0) {
    return new Map();
  }

  const entries = await Promise.all(
    contractIds.map(async (contractId) => {
      try {
        const contract = await getContract(contractId, signal);

        return [contractId, contract.name] as const;
      } catch (error) {
        if (signal.aborted) {
          throw error;
        }

        return [contractId, contractId] as const;
      }
    }),
  );

  return new Map(entries);
}

function buildOrganizationOptions(advertisements: Advertisement[]): FilterButtonOption[] {
  const options = new Map<string, string>();

  advertisements.forEach((advertisement) => {
    options.set(advertisement.organizationId, advertisement.organizationName || advertisement.organizationId);
  });

  return Array.from(options.entries())
    .map(([id, label]) => ({ id, label }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

function buildWorkgroupOptions(advertisements: Advertisement[], workgroups: Workgroup[]): FilterButtonOption[] {
  const workgroupNameById = new Map(workgroups.map((workgroup) => [workgroup.id, workgroup.name]));
  const scopedIds = new Set(advertisements.flatMap((advertisement) => advertisement.workgroupScopes));

  return Array.from(scopedIds)
    .map((id) => ({ id, label: workgroupNameById.get(id) ?? id }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

function filterAdvertisements(
  advertisements: Advertisement[],
  search: string,
  tunnelFilter: TunnelFilter,
  workgroupFilter: string,
  organizationFilter: string,
  workgroupNameById: Map<string, string>,
  contractNameById: Map<string, string>,
): Advertisement[] {
  const normalizedSearch = search.trim().toLowerCase();

  return advertisements.filter((advertisement) => {
    if (tunnelFilter !== 'all' && (advertisement.tunnelMode ?? 'tcp') !== tunnelFilter) {
      return false;
    }

    if (workgroupFilter !== 'all' && !advertisement.workgroupScopes.includes(workgroupFilter)) {
      return false;
    }

    if (organizationFilter !== 'all' && advertisement.organizationId !== organizationFilter) {
      return false;
    }

    if (!normalizedSearch) {
      return true;
    }

    return searchableValues(advertisement, workgroupNameById, contractNameById).some((value) =>
      value.toLowerCase().includes(normalizedSearch),
    );
  });
}

function searchableValues(
  advertisement: Advertisement,
  workgroupNameById: Map<string, string>,
  contractNameById: Map<string, string>,
): string[] {
  return [
    advertisement.id,
    advertisement.name,
    advertisement.description ?? '',
    advertisement.organizationId,
    advertisement.organizationName,
    advertisement.accountId,
    advertisement.tunnelMode ?? 'tcp',
    contractLabel(advertisement, contractNameById),
    ...advertisement.workgroupScopes.flatMap((workgroupId) => [workgroupId, workgroupNameById.get(workgroupId) ?? '']),
    ...advertisement.capabilities.flatMap((capability) => [capability.name, capability.description ?? '']),
    ...advertisement.interactionPatterns.map((pattern) => pattern.customPattern ?? pattern.kind),
  ];
}

function buildTunnelBreakdown(advertisements: Advertisement[]): SidebarBreakdownItem[] {
  const counts = new Map<AdvertisementTunnelMode, number>([
    ['http', 0],
    ['tcp', 0],
    ['udp', 0],
  ]);

  advertisements.forEach((advertisement) => {
    const mode = advertisement.tunnelMode ?? 'tcp';

    counts.set(mode, (counts.get(mode) ?? 0) + 1);
  });

  return Array.from(counts.entries())
    .filter(([, count]) => count > 0)
    .map(([mode, count]) => ({
      label: mode,
      value: count,
      accent: tunnelAccent(mode),
    }));
}

function buildContractBreakdown(
  advertisements: Advertisement[],
  contractNameById: Map<string, string>,
): SidebarBreakdownItem[] {
  const counts = new Map<string, number>();

  advertisements.forEach((advertisement) => {
    const label = contractLabel(advertisement, contractNameById);

    counts.set(label, (counts.get(label) ?? 0) + 1);
  });

  return Array.from(counts.entries())
    .map(([label, count]) => {
      const accent: SidebarBreakdownItem['accent'] = label === 'no contract' ? 'neutral' : 'llm';

      return {
        label,
        value: count,
        accent,
      };
    })
    .sort((a, b) => Number(b.value) - Number(a.value) || a.label.localeCompare(b.label));
}

function contractLabel(advertisement: Advertisement, contractNameById: Map<string, string>): string {
  if (!advertisement.contractId) {
    return 'no contract';
  }

  return contractNameById.get(advertisement.contractId) ?? advertisement.contractId;
}

function gatewayAccent(advertisement: Advertisement) {
  const capabilityNames = advertisement.capabilities.map((capability) => capability.name);
  const name = advertisement.name.toLowerCase();

  if (capabilityNames.includes('llm-routing') || capabilityNames.includes('llm-gateway') || name.includes('llm-gateway')) {
    return {
      borderClassName: 'border-brand-llm/40',
      stripClassName: 'bg-[linear-gradient(90deg,var(--color-brand-llm),var(--color-brand-llm-end))]',
      pillAccent: 'llm' as const,
    };
  }

  if (capabilityNames.includes('mcp-tools') || capabilityNames.includes('mcp-gateway') || name.includes('mcp-gateway')) {
    return {
      borderClassName: 'border-brand-mcp/40',
      stripClassName: 'bg-[linear-gradient(90deg,var(--color-brand-mcp),var(--color-brand-mcp-end))]',
      pillAccent: 'mcp' as const,
    };
  }

  return {
    borderClassName: 'border-border',
    stripClassName: '',
    pillAccent: 'agora' as const,
  };
}

function tunnelAccent(mode: AdvertisementTunnelMode): SidebarBreakdownItem['accent'] {
  if (mode === 'http') {
    return 'info';
  }

  if (mode === 'udp') {
    return 'warning';
  }

  return 'agora';
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
