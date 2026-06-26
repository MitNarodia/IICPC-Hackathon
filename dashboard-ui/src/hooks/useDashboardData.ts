import { useState, useEffect } from 'react';

export interface DashboardMetrics {
  runningBenchmarks: number;
  healthySandboxes: number;
  requestsPerSec: number;
  eventsPerSec: number;
  avgLatency: number;
  p50: number;
  p90: number;
  p99: number;
  successRate: number;
}

export interface TimeSeriesPoint {
  time: string;
  latency: number;
  cpu: number;
  memory: number;
  throughput: number;
  events: number;
  validation: number;
}

export function useDashboardData() {
  const [metrics, setMetrics] = useState<DashboardMetrics>({
    runningBenchmarks: 3,
    healthySandboxes: 12,
    requestsPerSec: 15200,
    eventsPerSec: 45600,
    avgLatency: 2.4,
    p50: 1.8,
    p90: 3.2,
    p99: 4.8,
    successRate: 99.98,
  });

  const [series, setSeries] = useState<TimeSeriesPoint[]>([]);

  useEffect(() => {
    // Initialize some historical data
    const now = new Date();
    const initialSeries = Array.from({ length: 20 }).map((_, i) => {
      const t = new Date(now.getTime() - (20 - i) * 1000);
      return generatePoint(t);
    });
    setSeries(initialSeries);

    // Update every second
    const interval = setInterval(() => {
      setSeries((prev) => {
        const next = [...prev, generatePoint(new Date())];
        if (next.length > 20) next.shift();
        return next;
      });

      setMetrics((prev) => ({
        ...prev,
        requestsPerSec: Math.floor(15000 + Math.random() * 2000),
        eventsPerSec: Math.floor(45000 + Math.random() * 5000),
        avgLatency: Number((2.0 + Math.random()).toFixed(2)),
        successRate: Number((99.9 + Math.random() * 0.09).toFixed(2)),
      }));
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  return { metrics, series };
}

function generatePoint(t: Date): TimeSeriesPoint {
  return {
    time: t.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' }),
    latency: 2.0 + Math.random() * 1.5,
    cpu: 40 + Math.random() * 20,
    memory: 60 + Math.random() * 10,
    throughput: 15000 + Math.random() * 2000,
    events: 45000 + Math.random() * 5000,
    validation: 99.5 + Math.random() * 0.5,
  };
}
