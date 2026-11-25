import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import type { InfiniteData } from '@tanstack/react-query';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminApi } from '../../api/client';
import type { AdminEvent as AdminEventT, AdminPhoto, AdminPhotoPage } from '../../api/client';
import EventForm from './EventForm';
import { btn, btnDanger, btnGhost, panel } from './ui';

const PAGE = 100;

export default function AdminEvent() {
  const { slug = '' } = useParams();
  const events = useQuery({ queryKey: ['admin-events'], queryFn: adminApi.events });
  const ev = events.data?.find((e) => e.slug === slug);

  if (events.isLoading) return <p className="text-gray-400 text-sm">Loading…</p>;
  if (events.error) return <p className="text-red-400 text-sm">{events.error.message}</p>;
  if (!ev) {
    return (
      <p className="text-sm">
        Event {slug} not found. <Link className="text-blue-400" to="/admin">Back</Link>
      </p>
    );
  }
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link className="text-blue-400 text-sm" to="/admin">← Events</Link>
        <h1 className="text-xl font-semibold">{ev.title}</h1>
        <span className="text-gray-400 text-sm">{ev.slug}</span>
      </div>
      <EditPanel ev={ev} />
      <SyncPanel ev={ev} />
      <PhotoGrid slug={slug} />
      <DeletePanel ev={ev} />
    </div>
  );
}

function EditPanel({ ev }: { ev: AdminEventT }) {
  const qc = useQueryClient();
  const save = useMutation({
    mutationFn: (body: Parameters<typeof adminApi.putEvent>[1]) => adminApi.putEvent(ev.slug, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-events'] });
      qc.invalidateQueries({ queryKey: ['admin-photos', ev.slug] });
    },
  });
  return (
    <section className={panel}>
      <h2 className="font-semibold mb-3">Event</h2>
      <EventForm
        initial={ev}
        submitLabel="Save"
        pending={save.isPending}
        error={save.error?.message}
        onSubmit={(b) => save.mutate(b)}
      />
      {save.isSuccess && <p className="text-sm text-green-400 mt-2">Saved.</p>}
    </section>
  );
}

function SyncPanel({ ev }: { ev: AdminEventT }) {
  const qc = useQueryClient();
  const status = useQuery({
    queryKey: ['admin-sync', ev.slug],
    queryFn: () => adminApi.syncStatus(ev.slug),
    refetchInterval: (q) => (q.state.data?.sync_state === 'running' ? 2000 : false),
  });
  const start = useMutation({
    mutationFn: () => adminApi.startSync(ev.slug),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin-sync', ev.slug] }),
  });
  const s = status.data;
  const running = s?.sync_state === 'running';

  // Refresh counts and photos whenever a run finishes (sync_finished_at changes).
  const finished = s?.sync_finished_at ?? null;
  const [seen, setSeen] = useState(finished);
  if (finished !== seen) {
    setSeen(finished);
    qc.invalidateQueries({ queryKey: ['admin-events'] });
    qc.invalidateQueries({ queryKey: ['admin-photos', ev.slug] });
  }

  const r = s?.sync_result;
  return (
    <section className={panel}>
      <div className="flex items-center gap-3 mb-3">
        <h2 className="font-semibold">Sync</h2>
        <span className={`text-sm ${s?.sync_state === 'failed' ? 'text-red-400' : 'text-gray-400'}`}>
          {s?.sync_state ?? '…'}
        </span>
        <button className={`${btn} ml-auto`} disabled={running || start.isPending} onClick={() => start.mutate()}>
          {running ? 'Syncing…' : 'Sync albums'}
        </button>
      </div>
      {start.error && <p className="text-sm text-red-400">{start.error.message}</p>}
      {s?.sync_error && <p className="text-sm text-red-400 whitespace-pre-wrap">{s.sync_error}</p>}
      {s?.sync_finished_at && (
        <p className="text-xs text-gray-400 mb-2">Finished {new Date(s.sync_finished_at).toLocaleString()}</p>
      )}
      {r && (
        <>
          <dl className="grid grid-cols-2 sm:grid-cols-6 gap-3 text-sm">
            {([
              ['Albums', r.albums],
              ['Items', r.items],
              ['By time', r.matched_time],
              ['By time+hash', r.matched_time_hash],
              ['By hash', r.matched_hash],
              ['Unmatched', r.unmatched],
            ] as const).map(([k, v]) => (
              <div key={k}>
                <dt className="text-xs text-gray-400">{k}</dt>
                <dd className="text-lg">{v}</dd>
              </div>
            ))}
          </dl>
          {(r.unmatched_items?.length ?? 0) > 0 && (
            <div className="mt-4">
              <h3 className="text-sm text-gray-400 mb-2">
                Unmatched items{r.unmatched > (r.unmatched_items?.length ?? 0) ? ` (first ${r.unmatched_items?.length})` : ''}
              </h3>
              <div className="flex flex-wrap gap-2">
                {r.unmatched_items?.map((u) => (
                  <img
                    key={u.album + u.media_key}
                    loading="lazy"
                    src={`${u.url}=w200`}
                    title={u.album}
                    alt=""
                    className="h-24 rounded border border-gray-800"
                  />
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </section>
  );
}

const methodColor: Record<string, string> = {
  time: 'bg-green-700',
  time_hash: 'bg-yellow-700',
  hash: 'bg-orange-700',
};

function PhotoGrid({ slug }: { slug: string }) {
  const qc = useQueryClient();
  const key = ['admin-photos', slug];
  const photos = useInfiniteQuery({
    queryKey: key,
    initialPageParam: 0,
    queryFn: ({ pageParam }) => adminApi.photos(slug, pageParam, PAGE),
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((n, p) => n + p.items.length, 0);
      return last.items.length > 0 && loaded < last.total ? loaded : undefined;
    },
  });
  const hide = useMutation({
    mutationFn: (p: AdminPhoto) => adminApi.setHidden(p.id, !p.hidden),
    onSuccess: (_d, p) => {
      qc.setQueryData<InfiniteData<AdminPhotoPage, number>>(key, (old) =>
        old && {
          ...old,
          pages: old.pages.map((pg) => ({
            ...pg,
            items: pg.items.map((x) => (x.id === p.id ? { ...x, hidden: !p.hidden } : x)),
          })),
        },
      );
    },
  });
  const items = photos.data?.pages.flatMap((pg) => pg.items) ?? [];
  const total = photos.data?.pages[0]?.total ?? 0;

  return (
    <section className={panel}>
      <h2 className="font-semibold mb-3">Photos <span className="text-sm text-gray-400 font-normal">({items.length} of {total})</span></h2>
      {photos.error && <p className="text-sm text-red-400">{photos.error.message}</p>}
      {hide.error && <p className="text-sm text-red-400">{hide.error.message}</p>}
      <div className="grid gap-3" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))' }}>
        {items.map((p) => (
          <figure key={p.id} className="relative" title={p.rel_path ?? 'unmatched'}>
            <img
              loading="lazy"
              src={`${p.base_url}=w400`}
              alt=""
              style={{ aspectRatio: `${p.width} / ${p.height}` }}
              className={`w-full rounded border border-gray-800 object-cover ${p.hidden ? 'opacity-30' : ''}`}
            />
            <span
              className={`absolute top-1 left-1 text-xs rounded px-1.5 py-0.5 ${
                p.match_method ? methodColor[p.match_method] : 'bg-red-800'
              }`}
            >
              {p.match_method ?? 'unmatched'}
            </span>
            <figcaption className="flex items-center justify-between mt-1 text-xs text-gray-400">
              <span className="truncate">{p.album}</span>
              <button className="text-blue-400 hover:underline" disabled={hide.isPending} onClick={() => hide.mutate(p)}>
                {p.hidden ? 'Unhide' : 'Hide'}
              </button>
            </figcaption>
          </figure>
        ))}
      </div>
      {photos.isLoading && <p className="text-sm text-gray-400">Loading…</p>}
      {photos.hasNextPage && (
        <button
          className={`${btnGhost} mt-4`}
          disabled={photos.isFetchingNextPage}
          onClick={() => photos.fetchNextPage()}
        >
          {photos.isFetchingNextPage ? 'Loading…' : 'Load more'}
        </button>
      )}
    </section>
  );
}

function DeletePanel({ ev }: { ev: AdminEventT }) {
  const nav = useNavigate();
  const qc = useQueryClient();
  const del = useMutation({
    mutationFn: () => adminApi.deleteEvent(ev.slug),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-events'] });
      nav('/admin');
    },
  });
  return (
    <section className={panel}>
      <button
        className={btnDanger}
        disabled={del.isPending}
        onClick={() => {
          if (confirm(`Delete event "${ev.title}" and all its synced photos?`)) del.mutate();
        }}
      >
        Delete event
      </button>
      {del.error && <p className="text-sm text-red-400 mt-2">{del.error.message}</p>}
    </section>
  );
}
