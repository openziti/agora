import { apiRequest } from './client';
import type { DashboardAccount } from './types';

export type WhoamiOptions = {
  signal?: AbortSignal;
  suppressUnauthorizedHandler?: boolean;
};

export function getWhoami(options: WhoamiOptions = {}): Promise<DashboardAccount> {
  return apiRequest<DashboardAccount>('/account/whoami', {
    signal: options.signal,
    suppressUnauthorizedHandler: options.suppressUnauthorizedHandler,
  });
}
