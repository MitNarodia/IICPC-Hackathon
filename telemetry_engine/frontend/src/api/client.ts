// REST client for the leaderboard-service. All calls are same-origin in
// production (nginx proxies /v1) and dev (Vite proxies /v1), so the base is
// usually empty. VITE_API_BASE overrides it for split deployments.

import type { ContestantDetail, LeaderboardEntry, RunSummary, Filters } from '../types';

const API_BASE: string = (import.meta.env.VITE_API_BASE as string) || '';

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, { headers: { Accept: 'application/json' } });
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

export async function fetchRuns(): Promise<RunSummary[]> {
  const data = await getJSON<{ runs: RunSummary[] }>(`/v1/runs`);
  return data.runs ?? [];
}

export async function fetchLeaderboard(runID: string, f?: Partial<Filters>): Promise<LeaderboardEntry[]> {
  const q = new URLSearchParams({ run: runID });
  if (f?.minCorrectness) q.set('min_correctness', String(f.minCorrectness));
  if (f?.health) q.set('health', f.health);
  if (f?.search) q.set('search', f.search);
  const data = await getJSON<{ entries: LeaderboardEntry[] }>(`/v1/leaderboard?${q.toString()}`);
  return data.entries ?? [];
}

export async function fetchContestant(runID: string, submissionID: string): Promise<ContestantDetail> {
  const q = new URLSearchParams({ run: runID, submission: submissionID });
  return getJSON<ContestantDetail>(`/v1/contestant?${q.toString()}`);
}

// wsURL builds the WebSocket URL for a run, honoring VITE_WS_BASE or deriving
// ws(s):// from the current page origin.
export function wsURL(runID: string): string {
  const base = (import.meta.env.VITE_WS_BASE as string) || '';
  if (base) return `${base}/v1/ws/leaderboard?run=${encodeURIComponent(runID)}`;
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${proto}://${window.location.host}/v1/ws/leaderboard?run=${encodeURIComponent(runID)}`;
}
