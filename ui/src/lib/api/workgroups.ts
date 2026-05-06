import { apiRequest } from './client';

import type { Workgroup, WorkgroupMembership } from './types';

export function listWorkgroups(signal?: AbortSignal) {
  return apiRequest<Workgroup[]>('/workgroups', { signal });
}

export function getWorkgroup(workgroupId: string, signal?: AbortSignal) {
  return apiRequest<Workgroup>(`/workgroups/${encodeURIComponent(workgroupId)}`, { signal });
}

export function listWorkgroupMembers(workgroupId: string, signal?: AbortSignal) {
  return apiRequest<WorkgroupMembership[]>(`/workgroups/${encodeURIComponent(workgroupId)}/members`, { signal });
}
