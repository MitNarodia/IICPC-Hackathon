// Small presentation helpers shared by the components. Pure functions, no React.

export function fmtInt(n: number): string {
  return Math.round(n).toLocaleString();
}

export function fmtFloat(n: number, digits = 1): string {
  return n.toLocaleString(undefined, { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

// Microseconds → human string (µs / ms).
export function fmtMicros(us: number): string {
  if (us < 1000) return `${fmtInt(us)} µs`;
  return `${fmtFloat(us / 1000, 2)} ms`;
}

export function fmtTPS(tps: number): string {
  if (tps >= 1_000_000) return `${fmtFloat(tps / 1_000_000, 2)}M`;
  if (tps >= 1_000) return `${fmtFloat(tps / 1_000, 1)}k`;
  return fmtInt(tps);
}

export function fmtPct(frac: number): string {
  return `${fmtFloat(frac * 100, 2)}%`;
}

// Score → color band for the composite cell (red→amber→green).
export function scoreColor(score: number): string {
  if (score >= 80) return 'var(--good)';
  if (score >= 55) return 'var(--ok)';
  if (score >= 30) return 'var(--warn)';
  return 'var(--bad)';
}

export function healthColor(health: string): string {
  switch (health.toUpperCase()) {
    case 'READY':
      return 'var(--good)';
    case 'DEGRADED':
      return 'var(--warn)';
    case 'UNHEALTHY':
      return 'var(--bad)';
    default:
      return 'var(--muted)';
  }
}

// Rank movement: negative delta means moved UP (smaller rank number is better).
export function rankDelta(rank: number, prev: number): number {
  if (prev === 0) return 0; // newly appeared
  return prev - rank;
}
