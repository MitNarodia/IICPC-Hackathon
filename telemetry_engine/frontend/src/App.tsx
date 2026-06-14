// App is the dashboard shell. It owns the selected run + contestant, wires the
// live WebSocket board (useLeaderboard) and the run list (useRuns), and lays out
// the header, filters, table, and detail panel.

import { useEffect, useMemo, useState } from 'react';
import { useRuns } from './hooks/useRuns';
import { useLeaderboard, type ConnState } from './hooks/useLeaderboard';
import { RunSelector } from './components/RunSelector';
import { Filters } from './components/Filters';
import { Leaderboard } from './components/Leaderboard';
import { ContestantDetail } from './components/ContestantDetail';
import type { Filters as FilterState } from './types';

const CONN_LABEL: Record<ConnState, string> = {
  connecting: 'connecting',
  live: 'live',
  reconnecting: 'reconnecting',
  idle: 'idle',
};

export default function App() {
  const { runs } = useRuns();
  const [runID, setRunID] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const [filters, setFilters] = useState<FilterState>({ minCorrectness: 0, health: '', search: '' });

  // Auto-select the first run once they load.
  useEffect(() => {
    if (!runID && runs.length > 0) setRunID(runs[0].run_id);
  }, [runs, runID]);

  // Reset selection when switching runs.
  useEffect(() => setSelected(null), [runID]);

  const { entries, conn } = useLeaderboard(runID);

  const selectedEntry = useMemo(
    () => entries.find((e) => e.submission_id === selected) ?? null,
    [entries, selected],
  );

  return (
    <div className="app">
      <header className="app-head">
        <div className="brand">
          <h1>Track 3 · Live Leaderboard</h1>
          <span className="tagline">Telemetry · Validation · Analytics</span>
        </div>
        <div className="head-controls">
          <RunSelector runs={runs} selected={runID} onSelect={setRunID} />
          <span className={`conn conn-${conn}`}>
            <span className="conn-dot" /> {CONN_LABEL[conn]}
          </span>
        </div>
      </header>

      <Filters value={filters} onChange={setFilters} />

      <main className={`content ${selectedEntry ? 'with-detail' : ''}`}>
        <section className="board-wrap">
          <Leaderboard entries={entries} filters={filters} selected={selected} onSelect={setSelected} />
        </section>
        {selectedEntry && runID && (
          <ContestantDetail runID={runID} entry={selectedEntry} onClose={() => setSelected(null)} />
        )}
      </main>

      <footer className="app-foot">
        <span>{entries.length} contestants</span>
        <span>{runID ?? 'no run'}</span>
        <span>IICPC Summer Hackathon 2026</span>
      </footer>
    </div>
  );
}
