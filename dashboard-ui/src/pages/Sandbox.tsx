import React, { useState } from 'react';
import { motion } from 'framer-motion';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Terminal, Activity, Server, Clock, Search, StopCircle, RefreshCw } from 'lucide-react';

const mockSandboxes = [
  { id: 'pod-abcd-1234', submissionId: 'sub-001', status: 'Running', cpu: '1.2m', memory: '450Mi', restarts: 0, uptime: '2h 15m' },
  { id: 'pod-efgh-5678', submissionId: 'sub-002', status: 'Running', cpu: '0.8m', memory: '210Mi', restarts: 1, uptime: '5h 10m' },
  { id: 'pod-ijkl-9012', submissionId: 'sub-003', status: 'CrashLoopBackOff', cpu: '0m', memory: '0Mi', restarts: 5, uptime: '10m' },
];

export function Sandbox() {
  const [selectedPod, setSelectedPod] = useState(mockSandboxes[0].id);

  return (
    <div className="space-y-6 pb-12 h-full flex flex-col">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight text-gray-100 flex items-center gap-2">
          <Server className="h-6 w-6 text-primary" />
          Sandbox Explorer
        </h1>
        <div className="flex gap-2">
          <Button variant="outline" size="sm"><RefreshCw className="h-4 w-4 mr-2" /> Refresh</Button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 flex-1 min-h-0">
        {/* Left Column: List of Sandboxes */}
        <Card className="col-span-1 flex flex-col min-h-0">
          <CardHeader className="pb-3 border-b border-gray-800">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-500" />
              <input
                type="text"
                placeholder="Filter pods..."
                className="w-full pl-9 pr-4 py-2 bg-gray-900 border border-gray-800 rounded-md text-sm text-gray-200 focus:outline-none focus:ring-1 focus:ring-primary"
              />
            </div>
          </CardHeader>
          <div className="flex-1 overflow-y-auto p-2 space-y-1">
            {mockSandboxes.map((pod) => (
              <button
                key={pod.id}
                onClick={() => setSelectedPod(pod.id)}
                className={`w-full text-left p-3 rounded-lg transition-colors border ${
                  selectedPod === pod.id 
                    ? 'bg-gray-800 border-gray-700' 
                    : 'bg-transparent border-transparent hover:bg-gray-800/50'
                }`}
              >
                <div className="flex justify-between items-start mb-2">
                  <div className="font-mono text-sm text-gray-200 truncate pr-2">{pod.id}</div>
                  <Badge variant={pod.status === 'Running' ? 'success' : 'danger'}>{pod.status}</Badge>
                </div>
                <div className="text-xs text-gray-500 flex justify-between">
                  <span>Sub: {pod.submissionId}</span>
                  <span>{pod.uptime}</span>
                </div>
              </button>
            ))}
          </div>
        </Card>

        {/* Right Column: Details and Logs */}
        <div className="col-span-1 lg:col-span-2 flex flex-col gap-6 min-h-0">
          <Card>
            <CardContent className="p-6">
              <div className="flex justify-between items-start mb-6">
                <div>
                  <h3 className="text-lg font-bold text-gray-100 font-mono mb-1">{selectedPod}</h3>
                  <p className="text-sm text-gray-400">Kubernetes Sandbox Container</p>
                </div>
                <Button variant="danger" size="sm" className="gap-2">
                  <StopCircle className="h-4 w-4" /> Terminate
                </Button>
              </div>
              
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div className="bg-gray-900 rounded-lg p-3 border border-gray-800">
                  <div className="text-xs text-gray-500 mb-1 flex items-center gap-1"><Activity className="h-3 w-3" /> CPU Usage</div>
                  <div className="text-lg font-semibold text-gray-200">1.2m</div>
                </div>
                <div className="bg-gray-900 rounded-lg p-3 border border-gray-800">
                  <div className="text-xs text-gray-500 mb-1 flex items-center gap-1"><Server className="h-3 w-3" /> Memory</div>
                  <div className="text-lg font-semibold text-gray-200">450 Mi</div>
                </div>
                <div className="bg-gray-900 rounded-lg p-3 border border-gray-800">
                  <div className="text-xs text-gray-500 mb-1 flex items-center gap-1"><RefreshCw className="h-3 w-3" /> Restarts</div>
                  <div className="text-lg font-semibold text-gray-200">0</div>
                </div>
                <div className="bg-gray-900 rounded-lg p-3 border border-gray-800">
                  <div className="text-xs text-gray-500 mb-1 flex items-center gap-1"><Clock className="h-3 w-3" /> Uptime</div>
                  <div className="text-lg font-semibold text-gray-200">2h 15m</div>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className="flex-1 flex flex-col min-h-0">
            <CardHeader className="py-3 border-b border-gray-800 flex flex-row items-center justify-between">
              <CardTitle className="text-sm flex items-center gap-2"><Terminal className="h-4 w-4" /> Live Logs</CardTitle>
              <Badge variant="outline">Follow Mode</Badge>
            </CardHeader>
            <div className="flex-1 bg-[#0d0d0f] p-4 overflow-y-auto font-mono text-xs text-gray-300 rounded-b-xl leading-relaxed">
              <div className="text-gray-500 mb-2"># Connected to log stream for {selectedPod}</div>
              <div className="text-green-400">[INFO] Server started on port 8081</div>
              <div>[DEBUG] Initializing WebSocket pool...</div>
              <div>[INFO] Waiting for incoming connections from Track 2...</div>
              <div>[INFO] Accepted connection from 10.244.1.5:43912</div>
              <div>[INFO] Order received: B 100 @ 50.00</div>
              <div>[INFO] Order received: S 50 @ 50.05</div>
              <div className="text-yellow-400">[WARN] High latency detected in matching engine (2.5ms)</div>
              <div>[INFO] Order received: B 200 @ 49.95</div>
              <motion.div 
                initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ repeat: Infinity, duration: 1, repeatType: 'reverse' }}
                className="w-2 h-4 bg-gray-400 mt-2 inline-block" 
              />
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
