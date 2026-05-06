import { apiRequest } from './client';

import type { Contract } from './types';

export function listContracts(signal?: AbortSignal) {
  return apiRequest<Contract[]>('/contracts', { signal });
}

export function getContract(contractId: string, signal?: AbortSignal) {
  return apiRequest<Contract>(`/contracts/${encodeURIComponent(contractId)}`, { signal });
}
