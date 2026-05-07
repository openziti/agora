import { apiRequest } from './client';

import type { AuditEvent, AuditEventType, ListAuditEventsResponse } from './types';

export type ListAuditEventsParams = {
  eventTypes?: AuditEventType[];
  workgroupId?: string;
  accountId?: string;
  from?: string;
  to?: string;
  cursor?: string;
  limit?: number;
};

const auditPageLimit = 200;

export function listAuditEvents(params: ListAuditEventsParams = {}, signal?: AbortSignal) {
  const search = new URLSearchParams();

  params.eventTypes?.forEach((eventType) => search.append('eventType', eventType));

  if (params.workgroupId) {
    search.set('workgroupId', params.workgroupId);
  }

  if (params.accountId) {
    search.set('accountId', params.accountId);
  }

  if (params.from) {
    search.set('from', params.from);
  }

  if (params.to) {
    search.set('to', params.to);
  }

  if (params.cursor) {
    search.set('cursor', params.cursor);
  }

  if (params.limit !== undefined) {
    search.set('limit', String(params.limit));
  }

  return apiRequest<ListAuditEventsResponse>('/audit-events', { params: search, signal });
}

export async function fetchAllAuditEvents(params: Omit<ListAuditEventsParams, 'cursor' | 'limit'> = {}, signal?: AbortSignal): Promise<AuditEvent[]> {
  const events: AuditEvent[] = [];
  let cursor: string | undefined;

  do {
    const page = await listAuditEvents({ ...params, cursor, limit: auditPageLimit }, signal);

    events.push(...page.items);
    cursor = page.nextCursor;
  } while (cursor);

  return events;
}
