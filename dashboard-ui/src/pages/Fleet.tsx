import React from 'react';
import { motion } from 'framer-motion';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card';
import { Network, Server, Plug, Cpu, ArrowRightLeft } from 'lucide-react';
import { useDashboardData } from '@/hooks/useDashboardData';

export function Fleet() {
  const { metrics } = useDashboardData();

  return (
    <div className="space-y-6 pb-12">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight text-gray-100 flex items-center gap-2">
          <Network className="h-6 w-6 text-secondary" />
          Bot Fleet Topology
        </h1>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card>
          <CardContent className="p-4 flex items-center gap-4">
            <div className="p-3 rounded-xl bg-primary/10 text-primary"><Cpu className="h-6 w-6" /></div>
            <div>
              <p className="text-sm font-medium text-gray-400">Workers</p>
              <p className="text-2xl font-bold text-gray-100">8</p>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4 flex items-center gap-4">
            <div className="p-3 rounded-xl bg-secondary/10 text-secondary"><Server className="h-6 w-6" /></div>
            <div>
              <p className="text-sm font-medium text-gray-400">Total Bots</p>
              <p className="text-2xl font-bold text-gray-100">10,000</p>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4 flex items-center gap-4">
            <div className="p-3 rounded-xl bg-success/10 text-success"><Plug className="h-6 w-6" /></div>
            <div>
              <p className="text-sm font-medium text-gray-400">Connections</p>
              <p className="text-2xl font-bold text-gray-100">10,000</p>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4 flex items-center gap-4">
            <div className="p-3 rounded-xl bg-yellow-500/10 text-yellow-500"><ArrowRightLeft className="h-6 w-6" /></div>
            <div>
              <p className="text-sm font-medium text-gray-400">Orders / Sec</p>
              <p className="text-2xl font-bold text-gray-100">{metrics.requestsPerSec.toLocaleString()}</p>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card className="h-[500px] relative overflow-hidden bg-[#0a0a0c]">
        <CardHeader className="absolute top-0 left-0 z-10 w-full bg-gradient-to-b from-card to-transparent border-none">
          <CardTitle>Realtime Network Topology</CardTitle>
        </CardHeader>
        <CardContent className="p-0 h-full w-full flex items-center justify-center">
          {/* Animated Network Visualization */}
          <div className="relative w-full max-w-3xl aspect-video flex items-center justify-between px-12">
            
            {/* Coordinator */}
            <div className="relative z-10 flex flex-col items-center">
              <div className="h-16 w-16 bg-gray-900 border-2 border-primary rounded-full flex items-center justify-center shadow-[0_0_15px_rgba(59,130,246,0.5)] z-20">
                <Server className="h-8 w-8 text-primary" />
              </div>
              <span className="mt-2 text-sm font-mono text-gray-400">Coordinator</span>
            </div>

            {/* Pulses */}
            <div className="absolute left-[15%] right-[15%] h-full pointer-events-none flex items-center">
               <svg className="w-full h-64 overflow-visible">
                 <path d="M 0 128 C 100 128, 200 30, 300 30" fill="none" stroke="#27272a" strokeWidth="2" />
                 <path d="M 0 128 C 100 128, 200 90, 300 90" fill="none" stroke="#27272a" strokeWidth="2" />
                 <path d="M 0 128 C 100 128, 200 160, 300 160" fill="none" stroke="#27272a" strokeWidth="2" />
                 <path d="M 0 128 C 100 128, 200 220, 300 220" fill="none" stroke="#27272a" strokeWidth="2" />
                 
                 {/* Animated Pulses */}
                 <motion.circle r="3" fill="#3b82f6"
                   animate={{ offsetDistance: ["0%", "100%"] }}
                   style={{ offsetPath: 'path("M 0 128 C 100 128, 200 30, 300 30")' }}
                   transition={{ duration: 1.5, repeat: Infinity, ease: "linear" }}
                 />
                 <motion.circle r="3" fill="#8b5cf6"
                   animate={{ offsetDistance: ["0%", "100%"] }}
                   style={{ offsetPath: 'path("M 0 128 C 100 128, 200 90, 300 90")' }}
                   transition={{ duration: 1.2, repeat: Infinity, ease: "linear", delay: 0.5 }}
                 />
                 <motion.circle r="3" fill="#22c55e"
                   animate={{ offsetDistance: ["0%", "100%"] }}
                   style={{ offsetPath: 'path("M 0 128 C 100 128, 200 160, 300 160")' }}
                   transition={{ duration: 1.8, repeat: Infinity, ease: "linear", delay: 0.2 }}
                 />
                 <motion.circle r="3" fill="#f59e0b"
                   animate={{ offsetDistance: ["0%", "100%"] }}
                   style={{ offsetPath: 'path("M 0 128 C 100 128, 200 220, 300 220")' }}
                   transition={{ duration: 1.4, repeat: Infinity, ease: "linear", delay: 0.8 }}
                 />
               </svg>
            </div>

            {/* Workers */}
            <div className="relative z-10 flex flex-col gap-6">
              {[1, 2, 3, 4].map((i) => (
                <div key={i} className="flex items-center gap-4">
                  <div className="h-10 w-10 bg-gray-900 border border-gray-700 rounded-lg flex items-center justify-center shadow-lg">
                    <Cpu className="h-5 w-5 text-gray-400" />
                  </div>
                  <div className="flex flex-col gap-1">
                    <div className="h-1 w-16 bg-gray-800 rounded-full overflow-hidden">
                      <motion.div 
                        className="h-full bg-secondary" 
                        animate={{ width: [`${30 + Math.random() * 20}%`, `${70 + Math.random() * 30}%`] }}
                        transition={{ duration: 2, repeat: Infinity, repeatType: 'reverse' }}
                      />
                    </div>
                  </div>
                </div>
              ))}
            </div>

            {/* Target Sandbox */}
            <div className="relative z-10 flex flex-col items-center ml-12">
              <div className="h-20 w-20 bg-gray-900 border-2 border-success rounded-xl flex items-center justify-center shadow-[0_0_20px_rgba(34,197,94,0.3)]">
                <Server className="h-10 w-10 text-success" />
              </div>
              <span className="mt-3 text-sm font-bold text-gray-200">Sandbox</span>
              <span className="text-xs font-mono text-gray-500">Target Engine</span>
            </div>
            
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
