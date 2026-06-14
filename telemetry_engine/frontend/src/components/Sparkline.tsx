// Sparkline is a dependency-free inline-SVG line chart for the contestant
// detail panel. We avoid a charting library to keep the bundle tiny and the
// render synchronous (these update on every WS tick).

interface Props {
  values: number[];
  width?: number;
  height?: number;
  color?: string;
  fill?: boolean;
}

export function Sparkline({ values, width = 280, height = 56, color = 'var(--accent)', fill = true }: Props) {
  if (values.length === 0) {
    return <div className="sparkline-empty">no data yet</div>;
  }
  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const span = max - min || 1;
  const stepX = values.length > 1 ? width / (values.length - 1) : width;

  const points = values.map((v, i) => {
    const x = i * stepX;
    const y = height - ((v - min) / span) * (height - 4) - 2;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  const line = points.join(' ');
  const area = `${line} ${width},${height} 0,${height}`;

  return (
    <svg className="sparkline" width={width} height={height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
      {fill && <polygon points={area} fill={color} opacity={0.12} />}
      <polyline points={line} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}
