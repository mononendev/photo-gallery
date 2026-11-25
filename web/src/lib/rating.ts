export const RATING_NAMES = ['Missed', 'Soft', 'Slightly soft', 'Sharp', 'Banger'];
export const RATING_COLORS = ['#e5484d', '#f76b15', '#ffc53d', '#46a758', '#3e63dd'];
export const TIER_NAMES = RATING_NAMES.slice(0, 4);

export const ratingName = (r: number) => RATING_NAMES[r] ?? String(r);
export const ratingColor = (r: number) => RATING_COLORS[r] ?? '#888';

export function shutterText(s: number): string {
  return s >= 1 ? `${s} s` : `1/${Math.round(1 / s)} s`;
}

export const unscore = (s: string | null) => (s ? s.replace(/_/g, ' ') : s);

export function fmtDate(d: string): string {
  const t = new Date(d.length === 10 ? `${d}T00:00:00` : d);
  return isNaN(t.getTime()) ? d : t.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}
