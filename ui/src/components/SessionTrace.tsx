import { SectionPanel } from './SectionPanel';
import type { AuditEvent, Session } from '../lib/api';

export type SessionTraceProps = {
  session: Session;
  auditEvents?: AuditEvent[];
};

type TraceStepState = 'recorded' | 'not_recorded' | 'success' | 'danger' | 'warning';

type TraceStep = {
  id: string;
  label: string;
  timestamp?: string;
  detail?: string;
  state: TraceStepState;
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

export function SessionTrace({ session, auditEvents }: SessionTraceProps) {
  const steps = buildTraceSteps(session, auditEvents);

  return (
    <SectionPanel title="Session Trace">
      <div className="flex flex-col">
        {steps.map((step, index) => {
          const isLast = index === steps.length - 1;
          const isRecorded = step.state !== 'not_recorded';

          return (
            <div key={step.id} className="flex gap-3">
              <div className="flex shrink-0 flex-col items-center">
                <div className={['mt-1 size-2.5 rounded-full', dotClassName(step.state)].join(' ')} />
                {!isLast && <div className="mt-1 min-h-6 w-px flex-1 bg-border" />}
              </div>
              <div className={['min-w-0', isLast ? '' : 'pb-5'].join(' ')}>
                <p className={['text-table font-semibold', isRecorded ? 'text-text' : 'text-text-mute'].join(' ')}>
                  {step.label}
                </p>
                {step.timestamp ? (
                  <p className="mt-0.5 font-mono text-label text-text-mute">{formatDateTime(step.timestamp)}</p>
                ) : !isRecorded ? (
                  <p className="mt-0.5 text-label text-text-mute">Not recorded</p>
                ) : null}
                {step.detail ? (
                  <p className="mt-0.5 text-label text-text-mute-strong">{step.detail}</p>
                ) : null}
              </div>
            </div>
          );
        })}
      </div>
    </SectionPanel>
  );
}

function buildTraceSteps(session: Session, auditEvents?: AuditEvent[]): TraceStep[] {
  const sessionEvents = auditEvents?.filter((e) => e.sessionId === session.id) ?? [];
  const findEvent = (type: AuditEvent['eventType']) => sessionEvents.find((e) => e.eventType === type);

  const proposed: TraceStep = {
    id: 'proposed',
    label: 'Session Proposed',
    timestamp: session.proposedAt,
    detail: [session.consumerAccountEmail ?? session.consumerAccountId, session.advertisementName]
      .filter(Boolean)
      .join(' · '),
    state: 'recorded',
  };

  let outcome: TraceStep;
  if (session.acceptedAt) {
    outcome = { id: 'outcome', label: 'Session Accepted', timestamp: session.acceptedAt, state: 'success' };
  } else if (session.closeReason === 'rejected') {
    outcome = { id: 'outcome', label: 'Session Rejected', timestamp: session.closedAt, state: 'danger' };
  } else {
    outcome = { id: 'outcome', label: 'Session Accepted', state: 'not_recorded' };
  }

  const tunnelAttachedEvent = findEvent('tunnel.attached');
  let tunnelProvision: TraceStep;
  if (tunnelAttachedEvent) {
    const tunnelId = eventDataString(tunnelAttachedEvent.data, 'tunnel_id') || session.tunnelId;
    tunnelProvision = {
      id: 'tunnel_attached',
      label: 'Tunnel Provisioned',
      timestamp: tunnelAttachedEvent.occurredAt,
      detail: tunnelId || undefined,
      state: 'recorded',
    };
  } else if (session.tunnelId) {
    tunnelProvision = {
      id: 'tunnel_attached',
      label: 'Tunnel Provisioned',
      detail: session.tunnelId,
      state: 'recorded',
    };
  } else {
    tunnelProvision = { id: 'tunnel_attached', label: 'Tunnel Provisioned', state: 'not_recorded' };
  }

  const envelopeFlowedEvents = sessionEvents.filter((e) => e.eventType === 'envelope.flowed');
  const envelopeCount =
    auditEvents !== undefined
      ? envelopeFlowedEvents.reduce((sum, e) => sum + eventDataNumber(e.data, 'count_delta'), 0)
      : (session.envelopeCount ?? 0);
  const envelopes: TraceStep = {
    id: 'envelopes',
    label: 'Envelopes Transferred',
    detail: envelopeCount === 1 ? '1 envelope' : `${envelopeCount} envelopes`,
    state: envelopeCount > 0 ? 'recorded' : 'not_recorded',
  };

  const tunnelDetachedEvent = findEvent('tunnel.detached');
  let tunnelRelease: TraceStep;
  if (tunnelDetachedEvent) {
    const finalState = eventDataString(tunnelDetachedEvent.data, 'final_state');
    tunnelRelease = {
      id: 'tunnel_detached',
      label: 'Tunnel Released',
      timestamp: tunnelDetachedEvent.occurredAt,
      detail: finalState || undefined,
      state: 'recorded',
    };
  } else {
    tunnelRelease = { id: 'tunnel_detached', label: 'Tunnel Released', state: 'not_recorded' };
  }

  let sessionClose: TraceStep;
  if (session.closedAt) {
    let closeLabel = session.closeReason
      ? (closeReasonLabels[session.closeReason] ?? session.closeReason)
      : undefined;
    if (session.closeReason === 'contract_violation' && session.closeDetail) {
      closeLabel = `${closeReasonLabels.contract_violation}: ${session.closeDetail}`;
    }
    sessionClose = {
      id: 'closed',
      label: 'Session Closed',
      timestamp: session.closedAt,
      detail: closeLabel,
      state: closeReasonToState(session.closeReason),
    };
  } else {
    sessionClose = { id: 'closed', label: 'Session Closed', state: 'not_recorded' };
  }

  return [proposed, outcome, tunnelProvision, envelopes, tunnelRelease, sessionClose];
}

function closeReasonToState(reason: Session['closeReason']): TraceStepState {
  if (reason === 'consumer_close' || reason === 'provider_close') return 'success';
  if (reason === 'contract_violation') return 'danger';
  if (reason === 'tunnel_failed') return 'warning';
  return 'recorded';
}

function dotClassName(state: TraceStepState): string {
  switch (state) {
    case 'success': return 'bg-success';
    case 'danger': return 'bg-danger';
    case 'warning': return 'bg-warning';
    case 'not_recorded': return 'bg-border';
    default: return 'bg-brand-agora';
  }
}

function eventDataString(data: Record<string, unknown> | undefined, key: string): string {
  const value = data?.[key];
  return typeof value === 'string' ? value : '';
}

function eventDataNumber(data: Record<string, unknown> | undefined, key: string): number {
  const value = data?.[key];
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function formatDateTime(value: string | undefined): string {
  if (!value) return '—';
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
