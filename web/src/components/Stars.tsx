import { ratingColor, ratingName } from '../lib/rating';

export function Stars({ n, className = '' }: { n: number; className?: string }) {
  return (
    <span className={`text-yellow-400 tracking-tight ${className}`} title={`Model score ${n}/5`}>
      {'★'.repeat(n)}<span className="text-gray-600">{'★'.repeat(Math.max(0, 5 - n))}</span>
    </span>
  );
}

export function RatingPill({ rating, className = '' }: { rating: number; className?: string }) {
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full bg-gray-950/80 px-2 py-0.5 text-xs ${className}`}>
      <span className="h-2 w-2 rounded-full" style={{ background: ratingColor(rating) }} />
      {ratingName(rating)}
    </span>
  );
}
