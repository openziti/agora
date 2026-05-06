import { apiRequest } from './client';

import type {
  DashboardActivityResponse,
  DashboardBucket,
  DashboardEnvironmentsResponse,
  DashboardSummaryResponse,
  DashboardWindow,
  WorkgroupsActivityResponse,
} from './types';

export type DashboardActivityParams = {
  window?: DashboardWindow;
  bucket?: DashboardBucket;
};

export type WorkgroupsActivityParams = {
  window?: DashboardWindow;
};

export function getDashboardSummary(signal?: AbortSignal) {
  return apiRequest<DashboardSummaryResponse>('/dashboard/summary', { signal });
}

export function getDashboardActivity(params: DashboardActivityParams = {}, signal?: AbortSignal) {
  return apiRequest<DashboardActivityResponse>('/dashboard/activity', { params, signal });
}

export function getDashboardEnvironments(signal?: AbortSignal) {
  return apiRequest<DashboardEnvironmentsResponse>('/dashboard/environments', { signal });
}

export function getWorkgroupsActivity(params: WorkgroupsActivityParams = {}, signal?: AbortSignal) {
  return apiRequest<WorkgroupsActivityResponse>('/dashboard/workgroups-activity', { params, signal });
}
