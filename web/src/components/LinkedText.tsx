import type { ReactNode } from 'react';

// Event descriptions come from the old gallery's markdown and may hold `[label](url)` links and bare URLs.
// Render those as links and everything else as plain text (no HTML, no other markdown).
const LINK = /\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)|(https?:\/\/[^\s)]+)/g;

export function LinkedText({ text }: { text: string }) {
  const parts: ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(LINK)) {
    const start = m.index ?? 0;
    if (start > last) parts.push(text.slice(last, start));
    const href = m[2] ?? m[3];
    parts.push(
      <a key={start} href={href} target="_blank" rel="noreferrer" className="text-blue-400 hover:underline">
        {m[1] ?? href}
      </a>,
    );
    last = start + m[0].length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return <>{parts}</>;
}
