// Leaderboard renders the ranked table. It applies the client-side filters and
// sorts by composite (the backend already ranks, but we re-sort defensively so
// the UI is correct even if a stale frame arrives out of order).

import { useMemo } from 'react';
import type { Filters, LeaderboardEntry } from '../types';
import { ContestantRow } from './ContestantRow';

interface Props {
  entries: LeaderboardEntry[];
  filters: Filters;
  selected: string | null;
  onSelect: (submissionID: string) => void;
}

export function Leaderboard({ entries, filters, selected, onSelect }: Props) {
  const visible = useMemo(() => {
    const term = filters.search.trim().toLowerCase();
    return entries
      .filter((e) => {
        if (filters.minCorrectness > 0 && e.correctness_score < filters.minCorrectness) return false;
        if (filters.health && e.health.toUpperCase() !== filters.health.toUpperCase()) return false;
        if (term && !e.display_name.toLowerCase().includes(term) && !e.submission_id.toLowerCase().includes(term)) {
          return false;
        }
        return true;
      })
      .slice()
      .sort((a, b) => b.composite - a.composite || a.p99_us - b.p99_us);
  }, [entries, filters]);

  if (entries.length === 0) {
    return <div className="empty">Waiting for the first scores…</div>;
  }

  return (
    <table className="leaderboard">
      <thead>
        <tr>
          <th>#</th>
          <th>Contestant</th>
          <th>Composite</th>
          <th title="latency">LAT</th>
          <th title="throughput">THR</th>
          <th title="correctness">COR</th>
          <th title="stability">STB</th>
          <th>TPS</th>
          <th>p99</th>
          <th>Err</th>
          <th>Health</th>
        </tr>
      </thead>
      <tbody>
        {visible.map((e) => (
          <ContestantRow key={e.submission_id} entry={e} selected={e.submission_id === selected} onSelect={onSelect} />
        ))}
      </tbody>
    </table>
  );
}
