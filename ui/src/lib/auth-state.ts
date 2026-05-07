import { useSyncExternalStore } from 'react';

import type { DashboardAccount } from './api';

export type AuthState = {
  status: 'unknown' | 'authenticated' | 'unauthenticated';
  account: DashboardAccount | null;
};

const listeners = new Set<() => void>();

let authState: AuthState = {
  status: 'unknown',
  account: null,
};

export function getAuthState(): AuthState {
  return authState;
}

export function setAuthenticatedAccount(account: DashboardAccount) {
  authState = {
    status: 'authenticated',
    account,
  };
  emitAuthState();
}

export function clearAuthState() {
  authState = {
    status: 'unauthenticated',
    account: null,
  };
  emitAuthState();
}

export function useAuthState(): AuthState {
  return useSyncExternalStore(subscribeAuthState, getAuthState, getAuthState);
}

function subscribeAuthState(listener: () => void) {
  listeners.add(listener);

  return () => {
    listeners.delete(listener);
  };
}

function emitAuthState() {
  listeners.forEach((listener) => listener());
}
