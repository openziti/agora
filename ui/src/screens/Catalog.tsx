import { useCallback, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import {
  AlertTriangle,
  BookOpen,
  ChevronDown,
  Compass,
  FileCheck2,
  FileText,
  Globe,
  Network,
  RefreshCcw,
  Search,
  Send,
  ShieldCheck,
  Tag,
  Wifi,
} from 'lucide-react';

import {
  AppShell,
  Button,
  DrawerCard,
  DrawerCodeChip,
  DrawerDivider,
  DrawerTip,
  EmptyState,
  InfoDrawer,
  Input,
  PageHeader,
  Select,
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

const routeByTab: Record<string, string> = {
  dashboard: '/',
  sessions: '/sessions',
  workgroups: '/workgroups',
  catalog: '/catalog',
  contracts: '/contracts',
  audit: '/audit',
};

const numberFormatter = new Intl.NumberFormat();

export default function Catalog() {
  const navigate = useNavigate();
  const [infoOpen, setInfoOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [tunnelFilter, setTunnelFilter] = useState<AdvertisementTunnelMode | 'all'>('all');
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

  const isFiltered =
    search !== '' ||
    tunnelFilter !== 'all' ||
    workgroupFilter !== 'all' ||
    organizationFilter !== 'all';

  function handleResetFilters() {
    setSearch('');
    setTunnelFilter('all');
    setWorkgroupFilter('all');
    setOrganizationFilter('all');
  }

  return (
    <AppShell
      product="agora"
      organizationName={callerAccount?.organizationName ?? 'Loading organization'}
      activeTab="catalog"
      status={hasError ? 'warning' : isLoading ? 'info' : 'success'}
      statusLabel={hasError ? 'Data refresh issue' : isLoading ? 'Loading data' : 'Connected'}
      userInitials={callerAccount ? initialsFromEmail(callerAccount.email) : '--'}
      userLabel={callerAccount?.email ?? 'Account loading'}
      onTabChange={handleTabChange}
    >
      <div className="flex flex-col gap-6">
        <PageHeader
          icon={BookOpen}
          label="DISCOVERY"
          title="Catalog"
          description="The discovery surface for agent capabilities — every result is filtered by your workgroup memberships, so agents you're not authorized to see simply don't appear."
          onInfoClick={() => setInfoOpen(true)}
        />
        {account.error ? <ErrorPanel title="Current account unavailable" error={account.error} onRetry={account.refetch} /> : null}

        <CatalogOverview
          advertisements={advertisements.data ?? []}
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

        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
          <SectionPanel title="Visible Advertisements" bodyClassName="p-0">
            <CatalogFilters
              search={search}
              tunnelFilter={tunnelFilter}
              workgroupFilter={workgroupFilter}
              organizationFilter={organizationFilter}
              workgroupOptions={workgroupOptions}
              organizationOptions={organizationOptions}
              onSearchChange={setSearch}
              onTunnelFilterChange={(v) => setTunnelFilter(v as AdvertisementTunnelMode | 'all')}
              onWorkgroupFilterChange={setWorkgroupFilter}
              onOrganizationFilterChange={setOrganizationFilter}
              onResetFilters={isFiltered ? handleResetFilters : undefined}
            />
            {!callerAccount || (advertisements.loading && !advertisements.data) || (workgroups.loading && !workgroups.data) ? (
              <div className="p-5">
                <LoadingPanel title="Loading catalog" compact />
              </div>
            ) : filteredAdvertisements.length > 0 ? (
              <div className="grid gap-3 p-3 grid-cols-1 lg:grid-cols-2">
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
            <SectionPanel title="Protocols">
              {advertisements.loading && !advertisements.data ? (
                <LoadingPanel title="Loading protocols" compact />
              ) : tunnelBreakdown.length > 0 ? (
                <SidebarBreakdown items={tunnelBreakdown} />
              ) : (
                <EmptyState icon={Network} title="No protocols" description="No protocol counts are available." />
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

      {infoOpen ? (
        <InfoDrawer title="About the Catalog" onClose={() => setInfoOpen(false)}>
          <div className="flex flex-col gap-5">
            <section>
              <h3 className="mb-2 font-semibold text-text">What is the Catalog?</h3>
              <p className="leading-relaxed text-text-mute">
                The Catalog is Agora's built-in discovery surface. Agents query it to find other agents
                and their capabilities. Every catalog query is filtered by the caller's workgroup
                memberships — entries you are not entitled to see do not appear in results at all.
                The Catalog is an intrinsic controller service, not a separate registry to configure or maintain.
              </p>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-3 font-semibold text-text">What an Advertisement contains</h3>
              <div className="flex flex-col gap-2">
                <DrawerCard icon={Tag} title="Name and capabilities" description={<>Declared service name and capability identifiers, e.g. <DrawerCodeChip>markets.equity</DrawerCodeChip>, <DrawerCodeChip>weather.forecast</DrawerCodeChip>.</>} />
                <DrawerCard icon={Globe} title="Visibility scope" description="Which workgroups can discover this advertisement. Agents in other workgroups see nothing." />
                <DrawerCard icon={Network} title="Tunnel protocols" description={<>Supported transport: <DrawerCodeChip>http</DrawerCodeChip>, <DrawerCodeChip>tcp</DrawerCodeChip>, or <DrawerCodeChip>udp</DrawerCodeChip>.</>} />
                <DrawerCard icon={FileCheck2} title="Contract terms" description="The required contract any consumer must accept to open a session with this agent." />
              </div>
            </section>

            <DrawerDivider />

            <section>
              <h3 className="mb-2 font-semibold text-text">How discovery works</h3>
              <p className="leading-relaxed text-text-mute">
                An agent calls the catalog search. Results are limited to advertisements published by
                agents in workgroups the caller shares. Agents in workgroups the caller does not share
                are fully absent — not hidden, not redacted, not visible at all.
              </p>
            </section>

            <DrawerDivider />

            <DrawerTip>
              When an agent loses workgroup access, its advertisements vanish from the catalog immediately — no cache invalidation delay.
            </DrawerTip>
          </div>
        </InfoDrawer>
      ) : null}
    </AppShell>
  );
}

function CatalogOverview({
  advertisements,
  workgroupCount,
  loading,
}: {
  advertisements: Advertisement[];
  workgroupCount: number;
  loading: boolean;
}) {
  const contractCount = new Set(advertisements.map((advertisement) => advertisement.contractId).filter(Boolean)).size;

  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      <StatCard
        label="Visible Ads"
        value={loading ? '-' : formatInteger(advertisements.length)}
        icon={Compass}
        accent="agora"
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
  onResetFilters,
}: {
  search: string;
  tunnelFilter: AdvertisementTunnelMode | 'all';
  workgroupFilter: string;
  organizationFilter: string;
  workgroupOptions: { id: string; label: string }[];
  organizationOptions: { id: string; label: string }[];
  onSearchChange: (value: string) => void;
  onTunnelFilterChange: (value: string) => void;
  onWorkgroupFilterChange: (value: string) => void;
  onOrganizationFilterChange: (value: string) => void;
  onResetFilters?: () => void;
}) {
  return (
    <div className="flex flex-col gap-3 border-b border-border p-4 sm:flex-row sm:flex-wrap sm:items-center sm:gap-3">
      <label className="relative block w-full sm:min-w-48 sm:flex-1">
        <span className="sr-only">Search catalog</span>
        <Search size={17} aria-hidden="true" className="absolute left-3 top-1/2 -translate-y-1/2 text-text-mute" />
        <Input
          type="search"
          value={search}
          onChange={(event) => onSearchChange(event.target.value)}
          placeholder="Search advertisements, organizations, contracts, workgroups, or capabilities"
          className="pl-10 pr-3"
        />
      </label>

      <div className="flex flex-col gap-2 sm:flex-row sm:w-auto sm:shrink-0 sm:items-center sm:gap-2">
        <div className="relative">
          <Select
            value={tunnelFilter}
            onChange={(event) => onTunnelFilterChange(event.target.value)}
            className="w-full pr-7 sm:w-auto"
          >
            <option value="all">All Protocols</option>
            <option value="http">http</option>
            <option value="tcp">tcp</option>
            <option value="udp">udp</option>
          </Select>
          <ChevronDown size={13} aria-hidden="true" className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-text-mute" />
        </div>
        <div className="relative">
          <Select
            value={workgroupFilter}
            onChange={(event) => onWorkgroupFilterChange(event.target.value)}
            className="w-full pr-7 sm:w-auto"
          >
            <option value="all">All Channels</option>
            {workgroupOptions.map((option) => (
              <option key={option.id} value={option.id}>{option.label}</option>
            ))}
          </Select>
          <ChevronDown size={13} aria-hidden="true" className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-text-mute" />
        </div>
        <div className="relative">
          <Select
            value={organizationFilter}
            onChange={(event) => onOrganizationFilterChange(event.target.value)}
            className="w-full pr-7 sm:w-auto"
          >
            <option value="all">All Owners</option>
            {organizationOptions.map((option) => (
              <option key={option.id} value={option.id}>{option.label}</option>
            ))}
          </Select>
          <ChevronDown size={13} aria-hidden="true" className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-text-mute" />
        </div>
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
  );
}


const MAX_VISIBLE_TAGS = 4;

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

  const allTags: { label: string; accent: 'contract' | 'workgroup' | 'agora' | 'llm' | 'mcp' }[] = [
    { label: contractName, accent: 'contract' },
    ...advertisement.workgroupScopes.map((workgroupId) => ({
      label: workgroupNameById.get(workgroupId) ?? workgroupId,
      accent: 'workgroup' as const,
    })),
    ...advertisement.capabilities.map((capability) => ({
      label: capability.name,
      accent: gateway.pillAccent,
    })),
  ];
  const visibleTags = allTags.slice(0, MAX_VISIBLE_TAGS);
  const overflowCount = allTags.length - MAX_VISIBLE_TAGS;

  return (
    <article
      className={[
        'relative flex overflow-hidden rounded-card border bg-panel p-3',
        gateway.borderClassName,
      ].join(' ')}
    >
      {gateway.stripClassName ? (
        <div className={`absolute inset-x-0 top-0 h-1 ${gateway.stripClassName}`} />
      ) : null}

      {/* Mobile: protocol badge absolutely positioned top-right */}
      <StatusPill status="info" label={advertisement.tunnelMode ?? 'tcp'} className="absolute right-3 top-3 shrink-0 sm:hidden" />

      <div className="flex min-w-0 flex-1 flex-col gap-1 pr-14 sm:pr-4">
        <p className="text-[0.6875rem] font-semibold uppercase tracking-[0.04em] leading-none text-text-mute-strong">
          {advertisement.organizationName}
        </p>
        <h2 className="truncate text-[0.875rem] font-semibold leading-snug text-text">
          {advertisement.name}
        </h2>
        <p className="truncate font-mono text-[0.6875rem] leading-none text-text-mute-2">
          {advertisement.id}
        </p>
        {advertisement.description ? (
          <p className="line-clamp-2 text-[0.76rem] leading-snug text-text-mute">
            {advertisement.description}
          </p>
        ) : null}
        {visibleTags.length > 0 ? (
          <div className="mt-auto flex flex-wrap items-center gap-1">
            {visibleTags.map((tag) => (
              <CompactPill key={tag.label} label={tag.label} accent={tag.accent} />
            ))}
            {overflowCount > 0 ? (
              <span className="text-[0.6875rem] text-text-mute">+{overflowCount} more</span>
            ) : null}
          </div>
        ) : null}
        {/* Mobile: full-width propose session button below pills */}
        <Button
          variant="ghost"
          title={`CLI: ${command}`}
          aria-label={`Copy session proposal command for ${advertisement.name}`}
          onClick={copyCommand}
          className="mt-2 w-full !justify-start sm:hidden !text-[0.6875rem] gap-1"
        >
          <Send size={11} aria-hidden="true" />
          Propose Session
        </Button>
      </div>

      {/* Desktop: right column with badge and button */}
      <div className="hidden w-[140px] shrink-0 flex-col items-end justify-between sm:flex">
        <StatusPill status="info" label={advertisement.tunnelMode ?? 'tcp'} className="shrink-0" />
        <Button
          variant="ghost"
          title={`CLI: ${command}`}
          aria-label={`Copy session proposal command for ${advertisement.name}`}
          onClick={copyCommand}
          className="!text-[0.6875rem] gap-1"
        >
          <Send size={11} aria-hidden="true" />
          Propose Session
        </Button>
      </div>
    </article>
  );
}

function CompactPill({ label, accent }: { label: string; accent: 'contract' | 'workgroup' | 'agora' | 'llm' | 'mcp' }) {
  return (
    <span
      className={[
        'inline-flex max-w-full items-center rounded-status border px-2 py-0.5 text-[0.6875rem] font-medium',
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

function buildOrganizationOptions(advertisements: Advertisement[]): { id: string; label: string }[] {
  const options = new Map<string, string>();

  advertisements.forEach((advertisement) => {
    options.set(advertisement.organizationId, advertisement.organizationName || advertisement.organizationId);
  });

  return Array.from(options.entries())
    .map(([id, label]) => ({ id, label }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

function buildWorkgroupOptions(advertisements: Advertisement[], workgroups: Workgroup[]): { id: string; label: string }[] {
  const workgroupNameById = new Map(workgroups.map((workgroup) => [workgroup.id, workgroup.name]));
  const scopedIds = new Set(advertisements.flatMap((advertisement) => advertisement.workgroupScopes));

  return Array.from(scopedIds)
    .map((id) => ({ id, label: workgroupNameById.get(id) ?? id }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

function filterAdvertisements(
  advertisements: Advertisement[],
  search: string,
  tunnelFilter: AdvertisementTunnelMode | 'all',
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
      ...(mode === 'tcp' ? { tooltip: 'Standard encrypted TCP connection' } : {}),
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
