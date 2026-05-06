import { apiRequest } from './client';

import type { Session, SessionState } from './types';

export type SessionRole = 'provider' | 'consumer' | 'both';
export type SessionSort = 'proposedAtDesc' | 'closedAtDesc';

export type ListSessionsParams = {
  states?: SessionState[];
  role?: SessionRole;
  advertisementId?: string;
  sort?: SessionSort;
  limit?: number;
};

export function listSessions(params: ListSessionsParams = {}, signal?: AbortSignal) {
  const search = new URLSearchParams();

  params.states?.forEach((state) => search.append('state', state));

  if (params.role) {
    search.set('role', params.role);
  }

  if (params.advertisementId) {
    search.set('advertisementId', params.advertisementId);
  }

  if (params.sort) {
    search.set('sort', params.sort);
  }

  if (params.limit !== undefined) {
    search.set('limit', String(params.limit));
  }

  return apiRequest<Session[]>('/sessions', { params: search, signal });
}

export function getSession(sessionId: string, signal?: AbortSignal) {
  return apiRequest<Session>(`/sessions/${encodeURIComponent(sessionId)}`, { signal });
}
