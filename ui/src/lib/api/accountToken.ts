import { apiRequest } from './client';
import type { AccountTokenResponse } from './types';

export type GetAccountTokenOptions = {
  signal?: AbortSignal;
  suppressUnauthorizedHandler?: boolean;
};

export function getAccountToken(options: GetAccountTokenOptions = {}): Promise<AccountTokenResponse> {
  return apiRequest<AccountTokenResponse>('/account/token', {
    signal: options.signal,
    suppressUnauthorizedHandler: options.suppressUnauthorizedHandler,
  });
}
