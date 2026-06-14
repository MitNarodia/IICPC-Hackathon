// MetricBadge renders a labeled value with an optional accent color. Used for
// the component sub-scores and headline metrics.

interface Props {
  label: string;
  value: string;
  color?: string;
  title?: string;
}

export function MetricBadge({ label, value, color, title }: Props) {
  return (
    <div className="badge" title={title}>
      <span className="badge-label">{label}</span>
      <span className="badge-value" style={color ? { color } : undefined}>
        {value}
      </span>
    </div>
  );
}
