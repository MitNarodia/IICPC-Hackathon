import React from 'react';
import { motion } from 'framer-motion';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { useDashboardData } from '@/hooks/useDashboardData';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { Activity, Cpu, Server, Zap, BarChart2, ShieldCheck, HeartPulse } from 'lucide-react';

export function Dashboard() {
  const { metrics, series } = useDashboardData();

  const topStats = [
    { name: 'Running Benchmarks', value: metrics.runningBenchmarks, icon: Activity, color: 'text-primary' },
    { name: 'Healthy Sandboxes', value: metrics.healthySandboxes, icon: Server, color: 'text-success' },
    { name: 'Requests/sec', value: metrics.requestsPerSec.toLocaleString(), icon: Zap, color: 'text-yellow-500' },
    { name: 'Events/sec', value: metrics.eventsPerSec.toLocaleString(), icon: BarChart2, color: 'text-secondary' },
    { name: 'Avg Latency', value: `${metrics.avgLatency}ms`, icon: Activity, color: 'text-primary' },
    { name: 'Success Rate', value: `${metrics.successRate}%`, icon: ShieldCheck, color: 'text-success' },
  ];

  const services = [
    { name: 'Submission API', track: 'Track 1', status: 'healthy' },
    { name: 'Build Service', track: 'Track 1', status: 'healthy' },
    { name: 'Sandbox Runner', track: 'Track 1', status: 'healthy' },
    { name: 'Coordinator', track: 'Track 2', status: 'healthy' },
    { name: 'Bot Workers', track: 'Track 2', status: 'warning' },
    { name: 'Ingestion', track: 'Track 3', status: 'healthy' },
    { name: 'Scoring', track: 'Track 3', status: 'healthy' },
    { name: 'Leaderboard', track: 'Track 3', status: 'healthy' },
  ];

  return (
    <div className="space-y-6 pb-12">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight text-gray-100">System Dashboard</h1>
        <div className="flex items-center gap-2 text-sm text-gray-400">
          <span className="relative flex h-3 w-3">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-success opacity-75"></span>
            <span className="relative inline-flex rounded-full h-3 w-3 bg-success"></span>
          </span>
          Live Updates
        </div>
      </div>

      {/* Top Metrics */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        {topStats.map((stat, i) => (
          <motion.div
            key={stat.name}
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: i * 0.05 }}
          >
            <Card>
              <CardContent className="p-4 flex flex-col gap-2">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-medium text-gray-400">{stat.name}</span>
                  <stat.icon className={`h-4 w-4 ${stat.color}`} />
                </div>
                <div className="text-2xl font-bold text-gray-100">{stat.value}</div>
              </CardContent>
            </Card>
          </motion.div>
        ))}
      </div>

      {/* Charts Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <ChartCard title="System Latency (ms)" data={series} dataKey="latency" color="#3b82f6" />
        <ChartCard title="Bot Throughput (req/sec)" data={series} dataKey="throughput" color="#f59e0b" />
        <ChartCard title="CPU Utilization (%)" data={series} dataKey="cpu" color="#8b5cf6" />
        <ChartCard title="Kafka Events (ev/sec)" data={series} dataKey="events" color="#22c55e" />
      </div>

      {/* Service Health Grid */}
      <div>
        <h2 className="text-xl font-bold tracking-tight text-gray-100 mb-4 mt-8">Service Health Grid</h2>
        <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-8 gap-4">
          {services.map((service, i) => (
            <motion.div
              key={service.name}
              initial={{ opacity: 0, scale: 0.9 }}
              animate={{ opacity: 1, scale: 1 }}
              transition={{ delay: i * 0.05 }}
            >
              <Card className="h-full">
                <CardContent className="p-4 flex flex-col items-center justify-center text-center gap-3">
                  <div className={`p-2 rounded-full ${
                    service.status === 'healthy' ? 'bg-success/10 text-success' : 
                    service.status === 'warning' ? 'bg-yellow-500/10 text-yellow-500' : 
                    'bg-danger/10 text-danger'
                  }`}>
                    <HeartPulse className="h-6 w-6 animate-pulse" />
                  </div>
                  <div>
                    <div className="text-xs font-semibold text-gray-100">{service.name}</div>
                    <div className="text-[10px] text-gray-500">{service.track}</div>
                  </div>
                </CardContent>
              </Card>
            </motion.div>
          ))}
        </div>
      </div>
    </div>
  );
}

function ChartCard({ title, data, dataKey, color }: { title: string, data: any[], dataKey: string, color: string }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-gray-400">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[250px] w-full">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data} margin={{ top: 5, right: 5, left: -20, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#27272a" vertical={false} />
              <XAxis dataKey="time" stroke="#71717a" fontSize={12} tickLine={false} axisLine={false} />
              <YAxis stroke="#71717a" fontSize={12} tickLine={false} axisLine={false} />
              <Tooltip 
                contentStyle={{ backgroundColor: '#18181b', borderColor: '#27272a', color: '#e4e4e7' }}
                itemStyle={{ color: color }}
              />
              <Line 
                type="monotone" 
                dataKey={dataKey} 
                stroke={color} 
                strokeWidth={2} 
                dot={false}
                isAnimationActive={false} // Disable to avoid stuttering on 1s updates
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}
