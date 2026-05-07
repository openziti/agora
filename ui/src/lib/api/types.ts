export type DateTimeString = string;

export type ApiErrorBody = {
  code: string;
  message: string;
};

export type AccountRole = 'admin' | 'member';

export type AccountTokenResponse = {
  accountToken: string;
};

export type LoginRequest = {
  email: string;
  password: string;
};

export type DashboardWindow = '24h' | '7d' | '30d';
export type DashboardBucket = '1h' | '6h' | '1d';

export type DashboardAccount = {
  accountId: string;
  email: string;
  organizationId: string;
  organizationName: string;
  role: AccountRole;
};

export type DashboardStats = {
  activeSessions: number;
  activeSessionsDelta7d: number;
  envelopesToday: number;
  envelopesYesterday: number;
  activeWorkgroups: number;
  activeTunnels: number;
};

export type DashboardRibbon = {
  workgroupCount: number;
  advertisementCount: number;
  sessionsToday: number;
  environmentCount: number;
};

export type DashboardSummaryResponse = {
  account: DashboardAccount;
  stats: DashboardStats;
  ribbon: DashboardRibbon;
};

export type DashboardActivityBucket = {
  start: DateTimeString;
  envelopes: number;
  sessions: number;
};

export type DashboardWorkgroupActivity = {
  workgroupId: string;
  workgroupName: string;
  envelopes: number;
};

export type DashboardActivityResponse = {
  buckets: DashboardActivityBucket[];
  byWorkgroup: DashboardWorkgroupActivity[];
};

export type DashboardEnvironmentStatus = 'online' | 'stale' | 'unknown' | 'disabled';

export type DashboardEnvironment = {
  id: string;
  name: string;
  accountId: string;
  status: DashboardEnvironmentStatus;
  lastHeartbeatAt?: DateTimeString;
};

export type DashboardEnvironmentsResponse = DashboardEnvironment[];

export type WorkgroupsActivityResponse = {
  byWorkgroup: DashboardWorkgroupActivity[];
};

export type AdvertisementTunnelMode = 'http' | 'tcp' | 'udp';
export type AdvertisementStatus = 'active' | 'retracted';
export type AdvertisementInteractionPatternKind = 'request-response' | 'stream' | 'broadcast' | 'custom';

export type AdvertisementCapability = {
  name: string;
  description?: string;
  metadata?: Record<string, string>;
};

export type AdvertisementInteractionPattern = {
  kind: AdvertisementInteractionPatternKind;
  customPattern?: string;
};

export type Advertisement = {
  id: string;
  organizationId: string;
  organizationName: string;
  accountId: string;
  name: string;
  description?: string;
  capabilities: AdvertisementCapability[];
  interactionPatterns: AdvertisementInteractionPattern[];
  workgroupScopes: string[];
  tunnelMode?: AdvertisementTunnelMode;
  contractId?: string;
  schemaVersion: number;
  status: AdvertisementStatus;
  retractedAt?: DateTimeString;
  createdAt: DateTimeString;
  updatedAt: DateTimeString;
};

export type CatalogSearchResponse = {
  items: Advertisement[];
  nextCursor?: string;
};

export type ContractAccessMode = 'open' | 'approval_required';

export type MaturityRequirements = {
  minAccountAgeDays?: number;
};

export type Contract = {
  id: string;
  accountId: string;
  organizationId: string;
  name: string;
  description?: string;
  schemaVersion: number;
  maxDurationSeconds: number;
  maxEnvelopeCount: number;
  maxEnvelopeBytes: number;
  allowedMessageTypes: string[];
  requiredWorkgroupMemberships: string[];
  maturityRequirements?: MaturityRequirements;
  accessMode: ContractAccessMode;
  createdAt: DateTimeString;
  updatedAt: DateTimeString;
};

export type ContractSnapshot = {
  contractId: string;
  name: string;
  description?: string;
  schemaVersion: number;
  maxDurationSeconds: number;
  maxEnvelopeCount: number;
  maxEnvelopeBytes: number;
  allowedMessageTypes: string[];
  requiredWorkgroupMemberships: string[];
  maturityRequirements?: MaturityRequirements;
  accessMode: ContractAccessMode;
  snapshottedAt: DateTimeString;
};

export type EnvironmentState = 'enabled' | 'disabled';

export type Environment = {
  id: string;
  organizationId: string;
  accountId: string;
  description?: string;
  host?: string;
  zitiIdentityId: string;
  state: EnvironmentState;
  lastSeenAt?: DateTimeString;
  createdAt: DateTimeString;
  updatedAt: DateTimeString;
};

export type WorkgroupScope = 'intra-org' | 'inter-org';
export type WorkgroupState = 'pending' | 'active' | 'declined';
export type WorkgroupMembershipRole = 'member' | 'admin';

export type Workgroup = {
  id: string;
  ownerOrganizationId: string;
  name: string;
  description?: string;
  scope: WorkgroupScope;
  state: WorkgroupState;
  participatingOrganizationIds: string[];
  createdAt: DateTimeString;
  updatedAt: DateTimeString;
};

export type WorkgroupMembership = {
  id: string;
  workgroupId: string;
  accountId: string;
  organizationId: string;
  email?: string;
  role: WorkgroupMembershipRole;
  joinedAt: DateTimeString;
};

export type SessionState = 'proposed' | 'accepting' | 'active' | 'closing' | 'closed';
export type SessionCloseReason =
  | 'rejected'
  | 'consumer_close'
  | 'provider_close'
  | 'contract_violation'
  | 'tunnel_failed'
  | 'admin_close'
  | 'workgroup_deleted'
  | 'environment_disabled';

export type Session = {
  id: string;
  advertisementId: string;
  workgroupId: string;
  providerAccountId: string;
  providerOrganizationId: string;
  consumerAccountId: string;
  consumerOrganizationId: string;
  advertisementName: string;
  workgroupName: string;
  providerOrganizationName: string;
  consumerOrganizationName: string;
  providerAccountEmail?: string;
  consumerAccountEmail?: string;
  tunnelMode: AdvertisementTunnelMode;
  tunnelId?: string;
  state: SessionState;
  closeReason?: SessionCloseReason;
  closeDetail?: string;
  proposerMessage?: string;
  proposedAt: DateTimeString;
  acceptedAt?: DateTimeString;
  closedAt?: DateTimeString;
  contractSnapshot?: ContractSnapshot;
  envelopeCount?: number;
};
