// useRuns polls the run list so newly-started benchmark runs appear in the
// selector without a page reload. Runs change rarely, so a slow poll is plenty.

import { useEffect, useState } from 'react';
import { fetchRuns } from '../api/client';
import type { RunSummary } from '../types';

export function useRuns(intervalMs = 5000) {
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    const load = () => {
      fetchRuns()
        .then((r) => {
          if (active) {
            setRuns(r);
            setError(null);
          }
        })
        .catch((e) => active && setError(String(e)));
    };
    load();
    const id = window.setInterval(load, intervalMs);
    return () => {
      active = false;
      window.clearInterval(id);
    };
  }, [intervalMs]);

  return { runs, error };
}
