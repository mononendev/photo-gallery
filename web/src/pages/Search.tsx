import { useSearchParams } from 'react-router-dom';
import { keepPreviousData, useInfiniteQuery } from '@tanstack/react-query';
import { publicApi, type PhotoQuery } from '../api/client';
import { ErrorBox, Loading } from '../components/Status';
import { PhotoGrid } from '../components/PhotoGrid';
import { PAGE_SIZE } from '../lib/filters';
import { useQDraft } from '../lib/useQDraft';

export default function Search() {
  const [sp, setSp] = useSearchParams();
  const q = sp.get('q')?.trim() ?? '';
  const [qText, setQText] = useQDraft(sp.get('q') ?? '');
  const keyword = sp.getAll('keyword');
  const params: PhotoQuery = { q: q || undefined, keyword };
  const enabled = q !== '' || keyword.length > 0;
  const photos = useInfiniteQuery({
    queryKey: ['photos', params],
    queryFn: ({ pageParam }) => publicApi.photos({ ...params, offset: pageParam, limit: PAGE_SIZE }),
    initialPageParam: 0,
    enabled,
    placeholderData: keepPreviousData,
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((n, p) => n + p.items.length, 0);
      return last.items.length > 0 && loaded < last.total ? loaded : undefined;
    },
  });
  const items = photos.data?.pages.flatMap((p) => p.items) ?? [];

  const dropKeyword = (k: string) =>
    setSp((prev) => {
      const n = new URLSearchParams(prev);
      n.delete('keyword');
      keyword.filter((x) => x !== k).forEach((x) => n.append('keyword', x));
      return n;
    }, { replace: true });

  return (
    <div>
      <h1 className="mb-4 text-2xl font-semibold">Search</h1>
      <input autoFocus value={qText} onChange={(e) => setQText(e.target.value)} placeholder="Search descriptions and keywords across all events"
        className="mb-3 w-full max-w-xl rounded-md border border-gray-800 bg-gray-900 px-3 py-2" />
      {keyword.length > 0 && (
        <div className="mb-3 flex flex-wrap gap-2">
          {keyword.map((k) => (
            <button key={k} onClick={() => dropKeyword(k)} className="rounded-full bg-gray-800 px-3 py-1 text-sm hover:bg-gray-700">
              {k} ✕
            </button>
          ))}
        </div>
      )}
      {!enabled && <p className="text-gray-400">Enter a search term.</p>}
      {enabled && photos.isLoading && <Loading />}
      {photos.error && <ErrorBox error={photos.error} />}
      {photos.data && (
        <p className="mb-3 text-sm text-gray-400">{photos.data.pages[0].total} photos</p>
      )}
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
  );
}
