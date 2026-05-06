import { apiRequest } from './client';

import type { Environment } from './types';

export function listEnvironments(signal?: AbortSignal) {
  return apiRequest<Environment[]>('/environments', { signal });
}

export function getEnvironment(environmentId: string, signal?: AbortSignal) {
  return apiRequest<Environment>(`/environments/${encodeURIComponent(environmentId)}`, { signal });
}
