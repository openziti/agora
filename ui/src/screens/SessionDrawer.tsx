import { type ReactNode, useState } from 'react';
import { ChevronDown, ChevronRight, GripVertical, X } from 'lucide-react';

import { useResizableDrawer } from '../hooks/useResizableDrawer';

import { KeyValueGrid, SectionPanel, SessionTrace, StatusPill } from '../components';
import type { AuditEvent, ContractSnapshot, Session } from '../lib/api';

type DrawerTab = 'details' | 'trace';

export type SessionDrawerProps = {
  session: Session;
  role: 'provider' | 'consumer' | 'unknown';
  counterpartyOrganizationName: string;
  auditEvents?: AuditEvent[];
  initialTab?: DrawerTab;
  onClose: () => void;
};

const closeReasonLabels: Record<string, string> = {
  consumer_close: 'Closed by consumer',
  provider_close: 'Closed by provider',
  tunnel_failed: 'Tunnel connection failed',
  rejected: 'Rejected',
  contract_violation: 'Contract violation',
  admin_close: 'Closed by administrator',
  workgroup_deleted: 'Workgroup deleted',
  environment_disabled: 'Environment disabled',
};

export function SessionDrawer({
  session,
  role,
  counterpartyOrganizationName,
  auditEvents,
  initialTab = 'details',
  onClose,
}: SessionDrawerProps) {
  const [activeTab, setActiveTab] = useState<DrawerTab>(initialTab);
  const { width, dragHandleProps } = useResizableDrawer({
    defaultWidth: 672,
    minWidth: 400,
    maxWidth: 1000,
  });

  return (
    <>
      <div className="fixed inset-0 z-[199] bg-text/20" aria-hidden="true" onClick={onClose} />
      <aside style={{ width, maxWidth: '100vw' }} className="fixed right-0 top-0 z-[200] flex h-full flex-col border-l border-border bg-page shadow-xl" role="dialog" aria-modal="true" aria-labelledby="session-drawer-title">
        <div
          {...dragHandleProps}
          aria-hidden="true"
          className="absolute left-0 top-1/2 z-10 hidden sm:flex h-9 w-5 -translate-x-full -translate-y-1/2 cursor-col-resize items-center justify-center rounded-md border border-border bg-page shadow-md text-text-mute transition-colors hover:bg-panel-subtle hover:text-text"
        >
          <GripVertical size={14} aria-hidden="true" />
        </div>
        <header className="flex min-h-16 items-center justify-between gap-4 bg-panel px-6">
          <div className="min-w-0">
            <p className="text-label font-medium uppercase text-text-mute">Session detail</p>
            <h2 id="session-drawer-title" className="truncate font-mono text-section font-semibold text-text">
              {session.id}
            </h2>
          </div>
          <button
            type="button"
            className="inline-flex size-9 shrink-0 items-center justify-center rounded-pill border border-border bg-panel text-text-mute-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            aria-label="Close session detail"
            onClick={onClose}
          >
            <X size={18} aria-hidden="true" />
          </button>
        </header>

        <div className="flex border-b border-t border-border bg-panel px-6" role="tablist">
          {(['details', 'trace'] as const).map((tab) => (
            <button
              key={tab}
              type="button"
              role="tab"
              aria-selected={activeTab === tab}
              className={[
                '-mb-px h-10 border-b-2 px-4 text-table font-medium',
                'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
                activeTab === tab
                  ? 'border-brand-agora text-text'
                  : 'border-transparent text-text-mute hover:text-text',
              ].join(' ')}
              onClick={() => setActiveTab(tab)}
            >
              {tab === 'details' ? 'Details' : 'Trace'}
            </button>
          ))}
        </div>

        <div className="flex flex-1 flex-col gap-4 overflow-y-auto p-6">
          {activeTab === 'details' ? (
            <DetailsContent session={session} role={role} counterpartyOrganizationName={counterpartyOrganizationName} />
          ) : (
            <SessionTrace session={session} auditEvents={auditEvents} />
          )}
        </div>
      </aside>
    </>
  );
}

function DetailsContent({
  session,
  role,
  counterpartyOrganizationName,
}: {
  session: Session;
  role: 'provider' | 'consumer' | 'unknown';
  counterpartyOrganizationName: string;
}) {
  const closeReasonValue = renderCloseReason(session.closeReason);

  return (
    <>
      <SectionPanel title="Resource">
        <KeyValueGrid
          entries={[
            { key: 'status', value: <StatusPill status={statusForState(session.state)} label={session.state} /> },
            { key: 'role', value: role },
            { key: 'session with', value: counterpartyOrganizationName },
            { key: 'service', value: session.advertisementName },
            { key: 'workgroup', value: session.workgroupName },
            { key: 'protocol', value: session.tunnelMode },
            ...(session.tunnelId
              ? [{ key: 'tunnel id', value: <span className="font-mono text-text-mute">{session.tunnelId}</span> }]
              : []),
            { key: 'envelopes', value: String(session.envelopeCount ?? 0) },
            ...(closeReasonValue !== undefined
              ? [{ key: 'close reason', value: closeReasonValue }]
              : []),
          ]}
        />
      </SectionPanel>

      <SectionPanel title="Timeline">
        <KeyValueGrid
          entries={[
            { key: 'proposed', value: formatDateTime(session.proposedAt) },
            { key: 'accepted', value: formatDateTime(session.acceptedAt) },
            { key: 'closed', value: formatDateTime(session.closedAt) },
            { key: 'close detail', value: session.closeDetail ? <ExpandableText text={session.closeDetail} /> : '-' },
            { key: 'proposer message', value: session.proposerMessage ?? '-' },
          ]}
        />
      </SectionPanel>

      <ContractSnapshotPanel snapshot={session.contractSnapshot} />
    </>
  );
}

function ContractSnapshotPanel({ snapshot }: { snapshot: ContractSnapshot | undefined }) {
  const [showRaw, setShowRaw] = useState(false);

  if (!snapshot) {
    return (
      <SectionPanel title="Contract Snapshot" bodyClassName="p-0">
        <div className="p-5 text-body text-text-mute">No contract snapshot is attached to this session.</div>
      </SectionPanel>
    );
  }

  return (
    <SectionPanel title="Contract Snapshot">
      <KeyValueGrid
        entries={[
          { key: 'name', value: snapshot.name },
          ...(snapshot.description ? [{ key: 'description', value: snapshot.description }] : []),
          { key: 'max duration', value: formatSeconds(snapshot.maxDurationSeconds) },
          { key: 'max envelopes', value: snapshot.maxEnvelopeCount === 0 ? 'Unlimited' : String(snapshot.maxEnvelopeCount) },
          { key: 'max bytes', value: snapshot.maxEnvelopeBytes === 0 ? 'Unlimited' : formatBytes(snapshot.maxEnvelopeBytes) },
          { key: 'access mode', value: formatAccessMode(snapshot.accessMode) },
          { key: 'schema version', value: String(snapshot.schemaVersion) },
        ]}
      />
      <div className="mt-3">
        <button
          type="button"
          className="inline-flex items-center gap-1 rounded-pill text-label font-medium text-text-mute hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
          onClick={() => setShowRaw((v) => !v)}
        >
          {showRaw ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
          {showRaw ? 'Hide raw JSON' : 'Show raw JSON'}
        </button>
        {showRaw && (
          <pre className="mt-2 max-h-64 overflow-auto rounded-card border border-border bg-panel-subtle p-4 font-mono text-table text-text-mute-strong">
            {JSON.stringify(snapshot, null, 2)}
          </pre>
        )}
      </div>
    </SectionPanel>
  );
}

function ExpandableText({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="min-w-0">
      {expanded ? (
        <pre className="whitespace-pre-wrap break-all font-mono text-table text-text-mute-strong">{text}</pre>
      ) : (
        <p className="truncate font-mono text-table text-text-mute-strong">{text}</p>
      )}
      <button
        type="button"
        className="mt-1 inline-flex items-center gap-1 rounded-pill text-label font-medium text-text-mute hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
        onClick={() => setExpanded((v) => !v)}
      >
        {expanded ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
        {expanded ? 'Show less' : 'Show more'}
      </button>
    </div>
  );
}

function renderCloseReason(reason: Session['closeReason']): ReactNode | undefined {
  if (!reason) {
    return undefined;
  }

  const label = closeReasonLabels[reason];

  if (label) {
    return label;
  }

  return <span className="text-text-mute">{reason}</span>;
}

function statusForState(state: Session['state']) {
  if (state === 'closed') {
    return 'neutral';
  }

  if (state === 'proposed' || state === 'accepting' || state === 'closing') {
    return 'info';
  }

  return 'active';
}

function formatDateTime(value: string | undefined): string {
  if (!value) {
    return '—';
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
    second: '2-digit',
  }).format(date);
}

function formatSeconds(seconds: number): string {
  if (seconds === 0) {
    return '0s';
  }

  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;

  if (hours > 0) {
    return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  }

  if (minutes > 0) {
    return secs > 0 ? `${minutes}m ${secs}s` : `${minutes}m`;
  }

  return `${secs}s`;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`;
  }

  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }

  if (bytes < 1024 * 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function formatAccessMode(mode: string): string {
  if (mode === 'approval_required') {
    return 'Approval Required';
  }

  return mode.charAt(0).toUpperCase() + mode.slice(1);
}
