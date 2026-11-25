import { useEffect } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { publicApi } from '../api/client';
import { ErrorBox, Loading, Panel } from '../components/Status';
import { Stars } from '../components/Stars';
import { RATING_COLORS, RATING_NAMES, ratingColor, ratingName, shutterText, unscore } from '../lib/rating';

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="flex justify-between gap-3 py-0.5 text-sm">
      <span className="text-gray-400">{k}</span><span className="text-right">{v}</span>
    </div>
  );
}

export default function PhotoPage() {
  const { slug = '', id = '' } = useParams();
  const nav = useNavigate();
  const { data: p, error, isLoading } = useQuery({ queryKey: ['photo', id], queryFn: () => publicApi.photo(id) });
  const prev = p?.prev_id ?? null;
  const next = p?.next_id ?? null;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLElement && /^(INPUT|SELECT|TEXTAREA)$/.test(e.target.tagName)) return;
      if (e.key === 'ArrowLeft' && prev != null) nav(`/events/${slug}/photos/${prev}`);
      if (e.key === 'ArrowRight' && next != null) nav(`/events/${slug}/photos/${next}`);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [prev, next, slug, nav]);

  if (isLoading) return <Loading />;
  if (error) return <ErrorBox error={error} />;
  if (!p) return null;

  const x = p.exif ?? {};
  const hasRatings = p.rating != null || p.focus_tier != null || p.quality_score != null || p.keeper != null;
  const hasDesc = !!p.description || p.keywords.length > 0 || p.adjectives.length > 0 || !!p.subject || !!p.composition || !!p.action;
  const hasCam = x.camera != null || x.lens != null || x.focal_mm != null || x.f_number != null || x.shutter_s != null || x.iso != null;
  const btn = 'rounded-lg border border-gray-800 bg-gray-900 px-3 py-1.5 text-sm hover:bg-gray-800';
  const dot = (r: number) => <span className="mr-1.5 inline-block h-2 w-2 rounded-full" style={{ background: ratingColor(r) }} />;

  return (
    <div>
      <div className="mb-4 flex items-center gap-2">
        <Link to={`/events/${slug}`} className={btn}>← Back to event</Link>
        <span className="flex-1" />
        {prev != null ? <Link to={`/events/${slug}/photos/${prev}`} className={btn}>← Prev</Link> : <span className={`${btn} opacity-40`}>← Prev</span>}
        {next != null ? <Link to={`/events/${slug}/photos/${next}`} className={btn}>Next →</Link> : <span className={`${btn} opacity-40`}>Next →</span>}
      </div>
      <div className="flex flex-col gap-6 min-[900px]:flex-row min-[900px]:items-start">
        <div className="min-w-0 flex-1">
          <img alt={p.description ?? ''} src={`${p.url}=w2048-h2048`} width={p.width} height={p.height}
            style={{ aspectRatio: `${p.width} / ${p.height}` }}
            className="mx-auto h-auto max-h-[85vh] w-auto max-w-full rounded-lg bg-gray-900 object-contain" />
        </div>
        <div className="w-full space-y-4 min-[900px]:w-[380px] min-[900px]:shrink-0">
          {hasRatings && (
            <Panel title="Ratings">
              {p.rating != null && <Row k="Your rating" v={<>{dot(p.rating)}{ratingName(p.rating)}</>} />}
              {p.focus_tier != null && (
                <Row k="Focus tier" v={<>{dot(p.focus_tier)}{RATING_NAMES[p.focus_tier]}</>} />
              )}
              {p.quality_score != null && (
                <Row k="Model score" v={
                  <><Stars n={p.quality_score} />
                    {p.keeper && <span className="ml-2 rounded-full px-2 py-0.5 text-xs text-white" style={{ background: RATING_COLORS[3] }}>Keeper</span>}
                  </>} />
              )}
              {p.quality_score == null && p.keeper && <Row k="Keeper" v="Yes" />}
            </Panel>
          )}
          {hasDesc && (
            <Panel title="Description">
              {p.description && <p className="mb-3 text-sm leading-relaxed text-gray-200">{p.description}</p>}
              {p.keywords.length > 0 && (
                <div className="mb-2 flex flex-wrap gap-1.5">
                  {p.keywords.map((k) => (
                    <Link key={k} to={`/search?keyword=${encodeURIComponent(k)}`}
                      className="rounded-full bg-gray-800 px-2.5 py-0.5 text-xs hover:bg-gray-700">{k}</Link>
                  ))}
                </div>
              )}
              {p.adjectives.length > 0 && (
                <div className="mb-2 flex flex-wrap gap-1.5">
                  {p.adjectives.map((a) => <span key={a} className="rounded-full border border-gray-800 px-2.5 py-0.5 text-xs text-gray-400">{a}</span>)}
                </div>
              )}
              {p.subject && <Row k="Subject" v={unscore(p.subject)} />}
              {p.composition && <Row k="Composition" v={unscore(p.composition)} />}
              {p.action && <Row k="Action" v={unscore(p.action)} />}
            </Panel>
          )}
          {hasCam && (
            <Panel title="Camera">
              {x.camera != null && <Row k="Camera" v={x.camera} />}
              {x.lens != null && <Row k="Lens" v={x.lens} />}
              {x.focal_mm != null && <Row k="Focal length" v={`${x.focal_mm} mm`} />}
              {x.f_number != null && <Row k="Aperture" v={`f/${x.f_number}`} />}
              {x.shutter_s != null && x.shutter_s > 0 && <Row k="Shutter" v={shutterText(x.shutter_s)} />}
              {x.iso != null && <Row k="ISO" v={x.iso} />}
            </Panel>
          )}
        </div>
      </div>
    </div>
  );
}
