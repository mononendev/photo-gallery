import type { ReactNode } from 'react';

// Event descriptions come from the old gallery's markdown and may hold `[label](url)` links and bare URLs.
// Render those as links and everything else as plain text (no HTML, no other markdown).
// A URL may hold one level of balanced parentheses (Wikipedia style).
const URL_PATTERN = String.raw`https?:\/\/(?:[^\s()]|\([^\s()]*\))+`;
const LINK = new RegExp(String.raw`\[([^\]]+)\]\((${URL_PATTERN})\)|(${URL_PATTERN})`, 'g');

export function LinkedText({ text }: { text: string }) {
  const parts: ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(LINK)) {
    const start = m.index ?? 0;
    if (start > last) parts.push(text.slice(last, start));
    // A bare URL does not take sentence punctuation after it ("see https://x.org."); that stays text.
    const bare = m[3]?.replace(/[.,;:!?'"\]]+$/, '');
    const href = m[2] ?? bare ?? '';
    parts.push(
      <a key={start} href={href} target="_blank" rel="noreferrer" className="text-blue-400 hover:underline">
        {m[1] ?? href}
      </a>,
    );
    last = start + (bare === undefined ? m[0].length : bare.length);
  }
  if (last < text.length) parts.push(text.slice(last));
  return <>{parts}</>;
}
