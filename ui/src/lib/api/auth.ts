import { apiRequest } from './client';
import type { AccountTokenResponse, DashboardAccount, LoginRequest } from './types';

export type LoginCredentials = LoginRequest;

export function login(credentials: LoginCredentials, signal?: AbortSignal): Promise<AccountTokenResponse> {
  return apiRequest<AccountTokenResponse>('/account/login', {
    method: 'POST',
    body: credentials,
    signal,
  });
}

export function logout(signal?: AbortSignal): Promise<void> {
  return apiRequest<void>('/account/logout', {
    method: 'POST',
    signal,
  });
}

export function getWhoami(signal?: AbortSignal): Promise<DashboardAccount> {
  return apiRequest<DashboardAccount>('/account/whoami', { signal });
}
