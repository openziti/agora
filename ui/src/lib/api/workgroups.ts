import { apiRequest } from './client';

import type { Workgroup, WorkgroupMembership, WorkgroupScope } from './types';

export function listWorkgroups(signal?: AbortSignal) {
  return apiRequest<Workgroup[]>('/workgroups', { signal });
}

export function getWorkgroup(workgroupId: string, signal?: AbortSignal) {
  return apiRequest<Workgroup>(`/workgroups/${encodeURIComponent(workgroupId)}`, { signal });
}

export function listWorkgroupMembers(workgroupId: string, signal?: AbortSignal) {
  return apiRequest<WorkgroupMembership[]>(`/workgroups/${encodeURIComponent(workgroupId)}/members`, { signal });
}

export function createWorkgroup(body: { name: string; scope: WorkgroupScope }, signal?: AbortSignal) {
  return apiRequest<Workgroup>('/workgroups', { method: 'POST', body, signal });
}
