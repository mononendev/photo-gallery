import type { PhotoQuery } from '../api/client';

export const PAGE_SIZE = 60;

export function parseFilters(sp: URLSearchParams): PhotoQuery {
  return {
    q: sp.get('q') ?? undefined,
    keyword: sp.getAll('keyword'),
    rating: sp.getAll('rating'),
    tier: sp.getAll('tier'),
    min_score: sp.get('min_score') ?? undefined,
    keeper: sp.get('keeper') ?? undefined,
    lens: sp.get('lens') ?? undefined,
    album: sp.get('album') ?? undefined,
    sort: sp.get('sort') ?? undefined,
  };
}
