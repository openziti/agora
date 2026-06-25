import { apiRequest } from './client';

import type {
  Advertisement,
  AdvertisementInteractionPatternKind,
  AdvertisementStatus,
  AdvertisementTunnelMode,
  CatalogSearchResponse,
} from './types';

export type CreateAdvertisementBody = {
  name: string;
  description?: string;
  capabilities: Array<{ name: string }>;
  interactionPatterns: Array<{ kind: AdvertisementInteractionPatternKind; customPattern?: string }>;
  tunnelMode: AdvertisementTunnelMode;
  workgroupScopes: string[];
  contractId?: string;
};

export type ListAdvertisementsParams = {
  status?: AdvertisementStatus;
};

export type SearchCatalogParams = {
  workgroups?: string[];
  capability?: string;
  interactionPatterns?: AdvertisementInteractionPatternKind[];
  ownerOrganizationId?: string;
  cursor?: string;
  limit?: number;
};

export function listAdvertisements(params: ListAdvertisementsParams = {}, signal?: AbortSignal) {
  return apiRequest<Advertisement[]>('/advertisements', { params, signal });
}

export function getAdvertisement(advertisementId: string, signal?: AbortSignal) {
  return apiRequest<Advertisement>(`/advertisements/${encodeURIComponent(advertisementId)}`, { signal });
}

export function createAdvertisement(body: CreateAdvertisementBody, signal?: AbortSignal) {
  return apiRequest<Advertisement>('/advertisements', { method: 'POST', body, signal });
}

export function searchCatalogAdvertisements(params: SearchCatalogParams = {}, signal?: AbortSignal) {
  const search = new URLSearchParams();

  params.workgroups?.forEach((workgroupId) => search.append('workgroup', workgroupId));
  params.interactionPatterns?.forEach((pattern) => search.append('interactionPattern', pattern));

  if (params.capability) {
    search.set('capability', params.capability);
  }

  if (params.ownerOrganizationId) {
    search.set('ownerOrganizationId', params.ownerOrganizationId);
  }

  if (params.cursor) {
    search.set('cursor', params.cursor);
  }

  if (params.limit !== undefined) {
    search.set('limit', String(params.limit));
  }

  return apiRequest<CatalogSearchResponse>('/catalog/advertisements', { params: search, signal });
}
