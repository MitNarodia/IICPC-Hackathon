import React from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import { Download, Filter, Cpu } from 'lucide-react';
import { useDashboardData } from '@/hooks/useDashboardData';
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';

export function Telemetry() {
  const { series } = useDashboardData();

  return (
    <div className="space-y-6 pb-12">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight text-gray-100 flex items-center gap-2">
          <Cpu className="h-6 w-6 text-primary" />
          Telemetry Engine
        </h1>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" className="gap-2"><Filter className="h-4 w-4" /> Filter</Button>
          <Button variant="primary" size="sm" className="gap-2"><Download className="h-4 w-4" /> Export CSV</Button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <LargeChartCard title="P99 Latency (ms)" data={series} dataKey="latency" color="#3b82f6" />
        <LargeChartCard title="Bot Throughput (req/sec)" data={series} dataKey="throughput" color="#f59e0b" />
        <LargeChartCard title="Sandbox CPU (%)" data={series} dataKey="cpu" color="#8b5cf6" />
        <LargeChartCard title="Sandbox Memory (MB)" data={series} dataKey="memory" color="#ec4899" />
        <LargeChartCard title="Kafka Ingestion (events/sec)" data={series} dataKey="events" color="#22c55e" />
        <LargeChartCard title="Validation Rate (%)" data={series} dataKey="validation" color="#14b8a6" />
      </div>
    </div>
  );
}

function LargeChartCard({ title, data, dataKey, color }: { title: string, data: any[], dataKey: string, color: string }) {
  return (
    <Card>
      <CardHeader className="pb-2 border-b border-gray-800">
        <CardTitle className="text-sm font-bold text-gray-200">{title}</CardTitle>
      </CardHeader>
      <CardContent className="pt-4">
        <div className="h-[300px] w-full">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
              <defs>
                <linearGradient id={`color-${dataKey}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor={color} stopOpacity={0.3}/>
                  <stop offset="95%" stopColor={color} stopOpacity={0}/>
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="#27272a" vertical={false} />
              <XAxis dataKey="time" stroke="#71717a" fontSize={12} tickLine={false} axisLine={false} />
              <YAxis stroke="#71717a" fontSize={12} tickLine={false} axisLine={false} />
              <Tooltip 
                contentStyle={{ backgroundColor: '#18181b', borderColor: '#27272a', color: '#e4e4e7', borderRadius: '8px' }}
                itemStyle={{ color: color, fontWeight: 'bold' }}
              />
              <Area 
                type="monotone" 
                dataKey={dataKey} 
                stroke={color} 
                strokeWidth={2} 
                fillOpacity={1} 
                fill={`url(#color-${dataKey})`} 
                isAnimationActive={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}
