import { Link } from 'react-router-dom';
import type { Photo } from '../api/client';
import { Stars, RatingPill } from './Stars';

export function PhotoCard({ p }: { p: Photo }) {
  const kw = p.keywords.slice(0, 4).join(' · ');
  return (
    <Link to={`/events/${p.event_slug}/photos/${p.id}`} className="block group">
      <div className="relative overflow-hidden rounded-lg bg-gray-900 border border-gray-800">
        <img
          loading="lazy"
          alt={p.description ?? ''}
          src={`${p.url}=w800`}
          srcSet={`${p.url}=w400 400w, ${p.url}=w800 800w, ${p.url}=w1200 1200w`}
          sizes="(min-width: 900px) 240px, 50vw"
          width={p.width}
          height={p.height}
          style={{ aspectRatio: `${p.width} / ${p.height}` }}
          className="w-full h-auto block transition group-hover:opacity-90"
        />
        {p.rating != null && <RatingPill rating={p.rating} className="absolute top-2 left-2" />}
        {p.quality_score != null && (
          <Stars n={p.quality_score} className="absolute top-2 right-2 rounded-full bg-gray-950/80 px-2 py-0.5 text-xs" />
        )}
      </div>
      {kw && <div className="mt-1 text-xs text-gray-400 truncate">{kw}</div>}
    </Link>
  );
}

export function PhotoGrid({ items }: { items: Photo[] }) {
  return (
    <div className="grid gap-3 items-start" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}>
      {items.map((p) => <PhotoCard key={p.id} p={p} />)}
    </div>
  );
}
