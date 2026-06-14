// ContestantRow is one line in the leaderboard. It shows the rank with a
// movement arrow (computed from prev_rank), the composite score with a colored
// bar, the four sub-scores, and the headline metrics. Clicking opens the detail.

import type { LeaderboardEntry } from '../types';
import { fmtFloat, fmtMicros, fmtPct, fmtTPS, healthColor, rankDelta, scoreColor } from '../format';

interface Props {
  entry: LeaderboardEntry;
  selected: boolean;
  onSelect: (submissionID: string) => void;
}

function RankMove({ rank, prev }: { rank: number; prev: number }) {
  const delta = rankDelta(rank, prev);
  if (delta === 0) {
    return <span className="rank-move flat" title="no change">—</span>;
  }
  if (delta > 0) {
    return <span className="rank-move up" title={`up ${delta}`}>▲{delta}</span>;
  }
  return <span className="rank-move down" title={`down ${-delta}`}>▼{-delta}</span>;
}

export function ContestantRow({ entry, selected, onSelect }: Props) {
  return (
    <tr
      className={`row ${selected ? 'selected' : ''}`}
      onClick={() => onSelect(entry.submission_id)}
    >
      <td className="cell-rank">
        <span className="rank-num">{entry.rank}</span>
        <RankMove rank={entry.rank} prev={entry.prev_rank} />
      </td>
      <td className="cell-name">
        <span className="name">{entry.display_name || entry.submission_id.slice(0, 8)}</span>
        <span className="sub">{entry.submission_id.slice(0, 12)}</span>
      </td>
      <td className="cell-composite">
        <div className="composite-bar-wrap">
          <div
            className="composite-bar"
            style={{ width: `${Math.max(2, Math.min(100, entry.composite))}%`, background: scoreColor(entry.composite) }}
          />
          <span className="composite-val">{fmtFloat(entry.composite, 1)}</span>
        </div>
      </td>
      <td className="cell-sub" title="latency sub-score">{fmtFloat(entry.latency_score, 0)}</td>
      <td className="cell-sub" title="throughput sub-score">{fmtFloat(entry.throughput_score, 0)}</td>
      <td className="cell-sub" title="correctness sub-score">{fmtFloat(entry.correctness_score, 0)}</td>
      <td className="cell-sub" title="stability sub-score">{fmtFloat(entry.stability_score, 0)}</td>
      <td className="cell-metric">{fmtTPS(entry.tps)}</td>
      <td className="cell-metric">{fmtMicros(entry.p99_us)}</td>
      <td className="cell-metric">{fmtPct(entry.error_rate)}</td>
      <td className="cell-health">
        <span className="health-dot" style={{ background: healthColor(entry.health) }} />
        {entry.health || '—'}
      </td>
    </tr>
  );
}
