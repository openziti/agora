import { searchCatalogAdvertisements } from './advertisements';

import type { Advertisement } from './types';

const catalogPageLimit = 200;

export async function fetchAllVisibleAdvertisements(signal?: AbortSignal): Promise<Advertisement[]> {
  const advertisements: Advertisement[] = [];
  let cursor: string | undefined;

  do {
    const page = await searchCatalogAdvertisements({ cursor, limit: catalogPageLimit }, signal);

    advertisements.push(...page.items);
    cursor = page.nextCursor;
  } while (cursor);

  return advertisements;
}
