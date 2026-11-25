import { useState } from 'react';
import type { AdminAlbum, AdminEventInput } from '../../api/client';
import { btn, btnGhost, input, label } from './ui';

interface Props {
  initial: AdminEventInput;
  submitLabel: string;
  pending: boolean;
  error?: string;
  onSubmit: (body: AdminEventInput) => void;
}

export default function EventForm({ initial, submitLabel, pending, error, onSubmit }: Props) {
  const [title, setTitle] = useState(initial.title);
  const [series, setSeries] = useState(initial.series ?? '');
  const [date, setDate] = useState(initial.event_date);
  const [description, setDescription] = useState(initial.description);
  const [published, setPublished] = useState(initial.published);
  const [albums, setAlbums] = useState<AdminAlbum[]>(initial.albums);

  const setAlbum = (i: number, patch: Partial<AdminAlbum>) =>
    setAlbums(albums.map((a, j) => (j === i ? { ...a, ...patch } : a)));

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit({
          title: title.trim(),
          series: series.trim() || null,
          event_date: date,
          description,
          published,
          albums: albums.map((a) => ({ name: a.name.trim(), share_url: a.share_url.trim() })),
        });
      }}
    >
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="sm:col-span-2">
          <label className={label}>Title</label>
          <input className={input} required value={title} onChange={(e) => setTitle(e.target.value)} />
        </div>
        <div>
          <label className={label}>Date</label>
          <input className={input} type="date" required value={date} onChange={(e) => setDate(e.target.value)} />
        </div>
        <div>
          <label className={label}>Series</label>
          <input className={input} value={series} onChange={(e) => setSeries(e.target.value)} />
        </div>
        <label className="flex items-center gap-2 text-sm sm:col-span-2 self-end pb-1">
          <input type="checkbox" checked={published} onChange={(e) => setPublished(e.target.checked)} />
          Published
        </label>
      </div>
      <div>
        <label className={label}>Description</label>
        <textarea
          className={input}
          rows={4}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </div>
      <div>
        <label className={label}>Albums</label>
        <div className="space-y-2">
          {albums.map((a, i) => (
            <div key={i} className="flex gap-2">
              <input
                className={`${input} sm:w-56`}
                placeholder="Name"
                required
                value={a.name}
                onChange={(e) => setAlbum(i, { name: e.target.value })}
              />
              <input
                className={input}
                placeholder="https://photos.app.goo.gl/..."
                required
                value={a.share_url}
                onChange={(e) => setAlbum(i, { share_url: e.target.value })}
              />
              <button type="button" className={btnGhost} onClick={() => setAlbums(albums.filter((_, j) => j !== i))}>
                Remove
              </button>
            </div>
          ))}
          <button type="button" className={btnGhost} onClick={() => setAlbums([...albums, { name: '', share_url: '' }])}>
            Add album
          </button>
        </div>
      </div>
      {error && <p className="text-sm text-red-400">{error}</p>}
      <button className={btn} disabled={pending}>{pending ? 'Saving…' : submitLabel}</button>
    </form>
  );
}
