import { useSearchParams, useParams } from 'react-router-dom';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { publicApi, ApiError, type PhotoQuery } from '../api/client';
import { ErrorBox, Loading } from '../components/Status';
import { PhotoGrid } from '../components/PhotoGrid';
import { PAGE_SIZE, parseFilters } from '../lib/filters';
import { RATING_COLORS, RATING_NAMES, TIER_NAMES, fmtDate } from '../lib/rating';

const label = 'mb-1 block text-xs font-semibold uppercase tracking-wide text-gray-400';
const input = 'w-full rounded-md border border-gray-800 bg-gray-950 px-2 py-1.5 text-sm';

export default function EventPage() {
  const { slug = '' } = useParams();
  const [sp, setSp] = useSearchParams();
  const ev = useQuery({ queryKey: ['event', slug], queryFn: () => publicApi.event(slug) });
  const filters = parseFilters(sp);
  const params: PhotoQuery = { ...filters, event: slug };
  const photos = useInfiniteQuery({
    queryKey: ['photos', params],
    queryFn: ({ pageParam }) => publicApi.photos({ ...params, offset: pageParam, limit: PAGE_SIZE }),
    initialPageParam: 0,
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((n, p) => n + p.items.length, 0);
      return last.items.length > 0 && loaded < last.total ? loaded : undefined;
    },
  });

  const set = (key: string, value: string | null) =>
    setSp((prev) => {
      const n = new URLSearchParams(prev);
      if (value) n.set(key, value); else n.delete(key);
      return n;
    }, { replace: true });
  const toggle = (key: string, value: string) =>
    setSp((prev) => {
      const n = new URLSearchParams(prev);
      const cur = n.getAll(key);
      n.delete(key);
      (cur.includes(value) ? cur.filter((v) => v !== value) : [...cur, value]).forEach((v) => n.append(key, v));
      return n;
    }, { replace: true });

  if (ev.error) {
    return <ErrorBox error={ev.error instanceof ApiError && ev.error.status === 404 ? 'Event not found' : ev.error} />;
  }
  if (!ev.data) return <Loading />;
  const e = ev.data;
  const items = photos.data?.pages.flatMap((p) => p.items) ?? [];
  const total = photos.data?.pages[0]?.total;
  const chip = (active: boolean) =>
    `rounded-full border px-2.5 py-0.5 text-xs ${active ? 'border-gray-400 bg-gray-700 text-white' : 'border-gray-800 text-gray-400 hover:text-gray-200'}`;

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-semibold">{e.title}</h1>
        <div className="mt-1 text-sm text-gray-400">
          {fmtDate(e.event_date)}{e.series && ` · ${e.series}`}
        </div>
        {e.description && <p className="mt-2 max-w-3xl whitespace-pre-line text-gray-300">{e.description}</p>}
        <div className="mt-2 flex flex-wrap gap-4 text-sm">
          {e.albums.map((a) => (
            <a key={a.share_url} href={a.share_url} target="_blank" rel="noreferrer" className="text-blue-400 hover:underline">
              Open in Google Photos{e.albums.length > 1 ? `: ${a.name}` : ''}
            </a>
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-6 min-[900px]:flex-row min-[900px]:items-start">
        <aside className="w-full min-[900px]:sticky min-[900px]:top-20 min-[900px]:w-[260px] min-[900px]:shrink-0 space-y-4 rounded-lg border border-gray-800 bg-gray-900 p-4 min-[900px]:max-h-[calc(100vh-6rem)] min-[900px]:overflow-y-auto">
          <div>
            <label className={label} htmlFor="f-q">Search</label>
            <input id="f-q" className={input} value={filters.q ?? ''} placeholder="carving, jump…"
              onChange={(ev) => set('q', ev.target.value)} />
          </div>
          <div>
            <span className={label}>Your rating</span>
            <div className="flex flex-wrap gap-1.5">
              {[...RATING_NAMES.map((n, i) => [String(i), n, RATING_COLORS[i]] as const), ['none', 'Not rated', '#6b7280'] as const].map(([v, n, c]) => (
                <button key={v} className={chip(filters.rating!.includes(v))} onClick={() => toggle('rating', v)}>
                  <span className="mr-1 inline-block h-2 w-2 rounded-full" style={{ background: c }} />{n}
                </button>
              ))}
            </div>
          </div>
          <div>
            <span className={label}>Focus tier</span>
            <div className="flex flex-wrap gap-1.5">
              {TIER_NAMES.map((n, i) => (
                <button key={n} className={chip(filters.tier!.includes(String(i)))} onClick={() => toggle('tier', String(i))}>
                  <span className="mr-1 inline-block h-2 w-2 rounded-full" style={{ background: RATING_COLORS[i] }} />{n}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className={label} htmlFor="f-score">Min score</label>
            <select id="f-score" className={input} value={filters.min_score ?? ''} onChange={(ev) => set('min_score', ev.target.value)}>
              <option value="">Any</option>
              {[1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{'★'.repeat(n)}</option>)}
            </select>
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={filters.keeper === '1'} onChange={(ev) => set('keeper', ev.target.checked ? '1' : null)} />
            Keepers only
          </label>
          {e.facets.keywords.length > 0 && (
            <div>
              <span className={label}>Keywords</span>
              <div className="flex flex-wrap gap-1.5">
                {e.facets.keywords.map((k) => (
                  <button key={k.keyword} className={chip(filters.keyword!.includes(k.keyword))} onClick={() => toggle('keyword', k.keyword)}>
                    {k.keyword} <span className="text-gray-500">{k.count}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
          {e.facets.lenses.length > 0 && (
            <div>
              <label className={label} htmlFor="f-lens">Lens</label>
              <select id="f-lens" className={input} value={filters.lens ?? ''} onChange={(ev) => set('lens', ev.target.value)}>
                <option value="">Any</option>
                {e.facets.lenses.map((l) => <option key={l} value={l}>{l}</option>)}
              </select>
            </div>
          )}
          {e.facets.albums.length > 1 && (
            <div>
              <label className={label} htmlFor="f-album">Album</label>
              <select id="f-album" className={input} value={filters.album ?? ''} onChange={(ev) => set('album', ev.target.value)}>
                <option value="">All</option>
                {e.facets.albums.map((a) => <option key={a} value={a}>{a}</option>)}
              </select>
            </div>
          )}
          <button className="text-sm text-gray-400 hover:text-gray-200" onClick={() => setSp({}, { replace: true })}>
            Clear filters
          </button>
        </aside>

        <div className="min-w-0 flex-1">
          <div className="mb-3 flex items-center justify-between gap-3 text-sm text-gray-400">
            <span>{total === undefined ? '' : `${total} photos`}</span>
            <label className="flex items-center gap-2">
              Sort
              <select className="rounded-md border border-gray-800 bg-gray-950 px-2 py-1" value={filters.sort ?? 'taken'}
                onChange={(ev) => set('sort', ev.target.value === 'taken' ? null : ev.target.value)}>
                <option value="taken">Time taken</option>
                <option value="score">Model score</option>
                <option value="rating">Your rating</option>
              </select>
            </label>
          </div>
          {photos.isLoading && <Loading />}
          {photos.error && <ErrorBox error={photos.error} />}
          {photos.data && items.length === 0 && <p className="text-gray-400">No photos match.</p>}
          <PhotoGrid items={items} />
          {photos.hasNextPage && (
            <div className="mt-6 text-center">
              <button disabled={photos.isFetchingNextPage} onClick={() => photos.fetchNextPage()}
                className="rounded-lg border border-gray-700 bg-gray-900 px-5 py-2 hover:bg-gray-800 disabled:opacity-50">
                {photos.isFetchingNextPage ? 'Loading…' : 'Load more'}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
