import { apiRequest } from './client';
import type { AccountTokenResponse, LoginRequest } from './types';

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
