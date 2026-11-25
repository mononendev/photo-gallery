import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { publicApi } from '../api/client';
import { ErrorBox, Loading } from '../components/Status';
import { fmtDate } from '../lib/rating';

export default function Events() {
  const { data, error, isLoading } = useQuery({ queryKey: ['events'], queryFn: publicApi.events });
  const [series, setSeries] = useState<string | null>(null);
  if (isLoading) return <Loading />;
  if (error) return <ErrorBox error={error} />;
  const events = [...(data ?? [])].sort((a, b) => b.event_date.localeCompare(a.event_date));
  const allSeries = [...new Set(events.map((e) => e.series).filter((s): s is string => !!s))].sort();
  const shown = series ? events.filter((e) => e.series === series) : events;
  const chip = (active: boolean) =>
    `rounded-full border px-3 py-1 text-sm ${active ? 'border-gray-500 bg-gray-800 text-white' : 'border-gray-800 text-gray-400 hover:text-gray-200'}`;

  return (
    <div>
      <h1 className="mb-4 text-2xl font-semibold">Events</h1>
      {allSeries.length > 0 && (
        <div className="mb-4 flex flex-wrap gap-2">
          <button className={chip(series === null)} onClick={() => setSeries(null)}>All</button>
          {allSeries.map((s) => (
            <button key={s} className={chip(series === s)} onClick={() => setSeries(s)}>{s}</button>
          ))}
        </div>
      )}
      {shown.length === 0 && <p className="text-gray-400">No events.</p>}
      <div className="grid gap-4" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))' }}>
        {shown.map((e) => (
          <Link key={e.slug} to={`/events/${e.slug}`}
            className="overflow-hidden rounded-lg border border-gray-800 bg-gray-900 hover:border-gray-600">
            <div className="aspect-[3/2] bg-gray-800">
              {e.cover_url && (
                <img loading="lazy" alt="" src={`${e.cover_url}=w800`} className="h-full w-full object-cover" />
              )}
            </div>
            <div className="p-3">
              <div className="font-medium">{e.title}</div>
              <div className="mt-1 flex flex-wrap items-center gap-2 text-sm text-gray-400">
                <span>{fmtDate(e.event_date)}</span>
                {e.series && <span className="rounded-full bg-gray-800 px-2 py-0.5 text-xs">{e.series}</span>}
                <span className="ml-auto">{e.photo_count} photos</span>
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  );
}
