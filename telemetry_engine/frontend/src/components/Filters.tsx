// Filters narrows the visible board: by correctness floor, sandbox health, and
// a free-text search over name/submission id. Filtering is client-side so it is
// instant and does not perturb the live stream.

import type { Filters as FilterState } from '../types';

interface Props {
  value: FilterState;
  onChange: (next: FilterState) => void;
}

export function Filters({ value, onChange }: Props) {
  return (
    <div className="filters">
      <input
        className="filter-search"
        type="search"
        placeholder="Search name or submission…"
        value={value.search}
        onChange={(e) => onChange({ ...value, search: e.target.value })}
      />
      <label className="filter-field">
        Min correctness
        <input
          type="range"
          min={0}
          max={100}
          step={5}
          value={value.minCorrectness}
          onChange={(e) => onChange({ ...value, minCorrectness: Number(e.target.value) })}
        />
        <span className="filter-val">{value.minCorrectness}</span>
      </label>
      <label className="filter-field">
        Health
        <select value={value.health} onChange={(e) => onChange({ ...value, health: e.target.value })}>
          <option value="">any</option>
          <option value="READY">READY</option>
          <option value="DEGRADED">DEGRADED</option>
          <option value="UNHEALTHY">UNHEALTHY</option>
        </select>
      </label>
    </div>
  );
}
