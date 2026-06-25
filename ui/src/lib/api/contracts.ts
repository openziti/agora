import { apiRequest } from './client';

import type { Contract, ContractAccessMode, MaturityRequirements } from './types';

export type CreateContractBody = {
  name: string;
  description?: string;
  accessMode: ContractAccessMode;
  maxDurationSeconds?: number;
  maxEnvelopeCount?: number;
  maxEnvelopeBytes?: number;
  allowedMessageTypes?: string[];
  maturityRequirements?: MaturityRequirements;
};

export function listContracts(signal?: AbortSignal) {
  return apiRequest<Contract[]>('/contracts', { signal });
}

export function getContract(contractId: string, signal?: AbortSignal) {
  return apiRequest<Contract>(`/contracts/${encodeURIComponent(contractId)}`, { signal });
}

export function createContract(body: CreateContractBody, signal?: AbortSignal) {
  return apiRequest<Contract>('/contracts', { method: 'POST', body, signal });
}
