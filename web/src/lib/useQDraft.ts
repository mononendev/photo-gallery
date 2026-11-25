import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';

// The search box types into a draft; the URL (one request) follows once typing pauses.
export function useQDraft(urlQ: string): [string, (v: string) => void] {
  const [, setSp] = useSearchParams();
  const [draft, setDraft] = useState<string | null>(null);
  useEffect(() => {
    if (draft === null) return;
    const t = setTimeout(() => {
      setSp((prev) => {
        const n = new URLSearchParams(prev);
        if (draft) n.set('q', draft); else n.delete('q');
        return n;
      }, { replace: true });
      setDraft(null);
    }, 300);
    return () => clearTimeout(t);
  }, [draft, setSp]);
  return [draft ?? urlQ, setDraft];
}
