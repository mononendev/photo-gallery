// ---------------------------------------------------------------------------
// Types (mirror internal/api)
// ---------------------------------------------------------------------------

export interface Health { ok: boolean; version: string; database: boolean }

// ---------------------------------------------------------------------------
// Requests
// ---------------------------------------------------------------------------

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

const statusText = (res: Response) =>
  res.statusText || `HTTP ${res.status}${res.status === 504 ? ' (gateway timeout)' : ''}`;

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { headers: { 'Content-Type': 'application/json' }, ...init });
  if (!res.ok) {
    let msg = statusText(res);
    try {
      const body = await res.json();
      msg = typeof body.detail === 'string' ? body.detail : JSON.stringify(body.detail ?? body);
    } catch { /* ignore */ }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export function qs(params: Record<string, unknown>): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue;
    p.set(k, String(v));
  }
  const s = p.toString();
  return s ? `?${s}` : '';
}

export const api = {
  health: () => request<Health>('/api/health'),
};

// ---------------------------------------------------------------------------
// Public gallery API (plan step 10)
// ---------------------------------------------------------------------------

export interface AlbumLink { name: string; share_url: string }

export interface EventSummary {
  slug: string;
  title: string;
  series: string | null;
  event_date: string;
  description: string;
  photo_count: number;
  cover_url: string | null;
  albums: AlbumLink[];
}

export interface EventDetail extends Omit<EventSummary, 'photo_count' | 'cover_url'> {
  facets: {
    keywords: { keyword: string; count: number }[];
    lenses: string[];
    albums: string[];
  };
}

export interface Exif {
  camera?: string | null;
  lens?: string | null;
  f_number?: number | null;
  shutter_s?: number | null;
  iso?: number | null;
  focal_mm?: number | null;
}

export interface Photo {
  id: number;
  event_slug: string;
  album: string;
  url: string;
  width: number;
  height: number;
  taken_at: string;
  rating: number | null;
  focus_tier: number | null;
  quality_score: number | null;
  keeper: boolean | null;
  subject: string | null;
  composition: string | null;
  action: string | null;
  description: string | null;
  keywords: string[];
  adjectives: string[];
  exif: Exif;
}

export interface PhotoDetail extends Photo { prev_id: number | null; next_id: number | null }

export interface PhotoList { items: Photo[]; total: number }

export interface PhotoQuery {
  event?: string;
  q?: string;
  keyword?: string[];
  rating?: string[];
  tier?: string[];
  min_score?: string;
  keeper?: string;
  lens?: string;
  album?: string;
  sort?: string;
  offset?: number;
  limit?: number;
}

function photoQs(p: PhotoQuery): string {
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(p)) {
    if (v === undefined || v === null || v === '') continue;
    if (Array.isArray(v)) v.forEach((x) => sp.append(k, x));
    else sp.set(k, String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : '';
}

export const publicApi = {
  events: () => request<EventSummary[]>('/api/events'),
  event: (slug: string) => request<EventDetail>(`/api/events/${encodeURIComponent(slug)}`),
  photos: (p: PhotoQuery) => request<PhotoList>(`/api/photos${photoQs(p)}`),
  photo: (id: number | string) => request<PhotoDetail>(`/api/photos/${id}`),
};
