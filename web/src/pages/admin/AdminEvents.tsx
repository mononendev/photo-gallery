import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { adminApi } from '../../api/client';
import type { AdminEvent } from '../../api/client';
import EventForm from './EventForm';
import { SLUG_RE, input, label, panel } from './ui';

export default function AdminEvents() {
  const nav = useNavigate();
  const qc = useQueryClient();
  const events = useQuery({ queryKey: ['admin-events'], queryFn: adminApi.events });
  const [slug, setSlug] = useState('');
  const create = useMutation({
    mutationFn: (body: Parameters<typeof adminApi.putEvent>[1]) => adminApi.putEvent(slug, body),
    onSuccess: (ev) => {
      qc.setQueryData<AdminEvent[]>(['admin-events'], (old) => [...(old ?? []).filter((e) => e.slug !== ev.slug), ev]);
      qc.invalidateQueries({ queryKey: ['admin-events'] });
      nav(`/admin/events/${ev.slug}`);
    },
  });
  const [slugError, setSlugError] = useState('');

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold">Events</h1>
      <div className={`${panel} overflow-x-auto`}>
        {events.isLoading && <p className="text-gray-400 text-sm">Loading…</p>}
        {events.error && <p className="text-red-400 text-sm">{events.error.message}</p>}
        {events.data && (
          <table className="w-full text-sm">
            <thead className="text-left text-gray-400">
              <tr>
                <th className="py-1 pr-3">Title</th>
                <th className="pr-3">Slug</th>
                <th className="pr-3">Date</th>
                <th className="pr-3">Published</th>
                <th className="pr-3">Photos</th>
                <th className="pr-3">Matched</th>
                <th className="pr-3">Sources</th>
                <th>Sync</th>
              </tr>
            </thead>
            <tbody>
              {events.data.map((e) => (
                <tr key={e.slug} className="border-t border-gray-800">
                  <td className="py-1.5 pr-3">
                    <Link className="text-blue-400 hover:underline" to={`/admin/events/${e.slug}`}>{e.title}</Link>
                  </td>
                  <td className="pr-3 text-gray-400">{e.slug}</td>
                  <td className="pr-3">{e.event_date}</td>
                  <td className="pr-3">{e.published ? 'yes' : 'no'}</td>
                  <td className="pr-3">{e.photos}</td>
                  <td className="pr-3">{e.matched}</td>
                  <td className="pr-3">{e.sources}</td>
                  <td className={e.sync_state === 'failed' ? 'text-red-400' : ''}>{e.sync_state}</td>
                </tr>
              ))}
              {events.data.length === 0 && (
                <tr><td colSpan={8} className="py-2 text-gray-400">No events.</td></tr>
              )}
            </tbody>
          </table>
        )}
      </div>

      <div className={panel}>
        <h2 className="font-semibold mb-3">New event</h2>
        <div className="mb-3 max-w-xs">
          <label className={label}>Slug</label>
          <input
            className={input}
            placeholder="ocsf7"
            value={slug}
            onChange={(e) => { setSlug(e.target.value); setSlugError(''); }}
          />
          {(slugError || (slug !== '' && !SLUG_RE.test(slug))) && (
            <p className="text-xs text-red-400 mt-1">
              {slugError || 'Lowercase letters, digits and dashes only.'}
            </p>
          )}
        </div>
        <EventForm
          initial={{ title: '', series: null, event_date: '', description: '', published: false, albums: [] }}
          submitLabel="Create event"
          pending={create.isPending}
          error={create.error?.message}
          onSubmit={(body) => {
            if (slug === '') setSlugError('Slug required');
            else if (!SLUG_RE.test(slug)) setSlugError('Lowercase letters, digits and dashes only.');
            else if (events.data?.some((e) => e.slug === slug)) setSlugError('Slug already exists');
            else create.mutate(body);
          }}
        />
      </div>
    </div>
  );
}
