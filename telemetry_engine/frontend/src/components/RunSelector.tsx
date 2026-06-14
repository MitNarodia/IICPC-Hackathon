// RunSelector lets the judge switch between simultaneously-running benchmarks.
// Each run is an independent leaderboard.

import type { RunSummary } from '../types';

interface Props {
  runs: RunSummary[];
  selected: string | null;
  onSelect: (runID: string) => void;
}

export function RunSelector({ runs, selected, onSelect }: Props) {
  return (
    <div className="run-selector">
      <label htmlFor="run">Run</label>
      <select
        id="run"
        value={selected ?? ''}
        onChange={(e) => onSelect(e.target.value)}
        disabled={runs.length === 0}
      >
        {runs.length === 0 && <option value="">no active runs</option>}
        {runs.map((r) => (
          <option key={r.run_id} value={r.run_id}>
            {r.run_id} · {r.contestants} contestants
          </option>
        ))}
      </select>
    </div>
  );
}
