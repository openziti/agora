import { X } from 'lucide-react';

import { KeyValueGrid, SectionPanel, StatusPill } from '../components';
import type { Session } from '../lib/api';

export type SessionDrawerProps = {
  session: Session;
  role: 'provider' | 'consumer' | 'unknown';
  counterpartyLabel: string;
  counterpartyOrganizationName: string;
  onClose: () => void;
};

export function SessionDrawer({
  session,
  role,
  counterpartyLabel,
  counterpartyOrganizationName,
  onClose,
}: SessionDrawerProps) {
  return (
    <div className="fixed inset-0 z-40 flex justify-end bg-text/20" role="dialog" aria-modal="true" aria-labelledby="session-drawer-title">
      <button type="button" className="absolute inset-0 cursor-default" aria-label="Close session detail" onClick={onClose} />
      <aside className="relative flex h-full w-full max-w-2xl flex-col border-l border-border bg-page shadow-xl">
        <header className="flex min-h-16 items-center justify-between gap-4 border-b border-border bg-panel px-6">
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

        <div className="flex flex-1 flex-col gap-4 overflow-y-auto p-6">
          <SectionPanel title="Resource">
            <KeyValueGrid
              entries={[
                { key: 'state', value: <StatusPill status={statusForState(session.state)} label={session.state} /> },
                { key: 'role', value: role },
                { key: 'counterparty', value: counterpartyLabel },
                { key: 'counterparty org', value: counterpartyOrganizationName },
                { key: 'advertisement', value: session.advertisementName },
                { key: 'workgroup', value: session.workgroupName },
                { key: 'tunnel mode', value: session.tunnelMode },
                { key: 'tunnel', value: session.tunnelId ?? '-' },
                { key: 'envelopes', value: session.envelopeCount ?? '-' },
                { key: 'close reason', value: session.closeReason ?? '-' },
              ]}
            />
          </SectionPanel>

          <SectionPanel title="Timeline">
            <KeyValueGrid
              entries={[
                { key: 'proposed', value: formatDateTime(session.proposedAt) },
                { key: 'accepted', value: formatDateTime(session.acceptedAt) },
                { key: 'closed', value: formatDateTime(session.closedAt) },
                { key: 'close detail', value: session.closeDetail ?? '-' },
                { key: 'proposer message', value: session.proposerMessage ?? '-' },
              ]}
            />
          </SectionPanel>

          <SectionPanel title="Contract Snapshot" bodyClassName="p-0">
            {session.contractSnapshot ? (
              <pre className="max-h-96 overflow-auto p-5 font-mono text-table text-text-mute-strong">
                {JSON.stringify(session.contractSnapshot, null, 2)}
              </pre>
            ) : (
              <div className="p-5 text-body text-text-mute">No contract snapshot is attached to this session.</div>
            )}
          </SectionPanel>
        </div>
      </aside>
    </div>
  );
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
    return '-';
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
