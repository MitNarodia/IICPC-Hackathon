// Mirror of the Go models the backend serializes (pkg/models). Keep field names
// in sync with the JSON tags — these are the contract between the two halves.

export interface RunSummary {
  run_id: string;
  contestants: number;
  last_update: string;
}

export interface LeaderboardEntry {
  rank: number;
  prev_rank: number;
  run_id: string;
  submission_id: string;
  display_name: string;
  composite: number;
  latency_score: number;
  throughput_score: number;
  correctness_score: number;
  stability_score: number;
  tps: number;
  p50_us: number;
  p99_us: number;
  error_rate: number;
  health: string;
  updated_at: string;
}

export interface BoardMessage {
  type: 'leaderboard';
  run_id: string;
  entries: LeaderboardEntry[];
}

export interface WindowAggregate {
  run_id: string;
  submission_id: string;
  window_start: string;
  window_end: string;
  window_kind: string;
  transactions: number;
  errors: number;
  timeouts: number;
  tps: number;
  error_rate: number;
  p50_us: number;
  p90_us: number;
  p99_us: number;
  max_us: number;
  mean_us: number;
  sample_count: number;
}

export interface RollingStats {
  run_id: string;
  submission_id: string;
  updated_at: string;
  total_transactions: number;
  total_errors: number;
  peak_tps: number;
  current_tps: number;
  p50_us: number;
  p90_us: number;
  p99_us: number;
  error_rate: number;
  tps_stddev: number;
  tps_cov: number;
}

export interface Finding {
  rule: string;
  message: string;
  order_id: number;
  at: string;
}

export interface ValidationResult {
  run_id: string;
  submission_id: string;
  updated_at: string;
  orders_checked: number;
  trades_checked: number;
  violations: number;
  violations_by_rule: Record<string, number>;
  correctness_score: number;
  recent_findings: Finding[];
}

export interface ContestantDetail {
  entry: LeaderboardEntry;
  rolling?: RollingStats;
  validation?: ValidationResult;
  history: WindowAggregate[];
}

export interface Filters {
  minCorrectness: number;
  health: string;
  search: string;
}
