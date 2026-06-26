import React, { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Card, CardContent } from '@/components/ui/Card';
import { Box, Network, Cpu, ArrowDown, Database, Server, Info } from 'lucide-react';

const archDetails = {
  track1: { title: "Track 1: Submission Engine", desc: "Handles container orchestration. Contestants upload their bots, which are built into Docker images and deployed onto isolated Kubernetes pods. Features health monitoring and strict resource quotas." },
  track2: { title: "Track 2: Bot Fleet", desc: "A highly-concurrent C++ application using Boost.Asio. Spawns thousands of concurrent WebSockets to simulate intense market traffic against the contestant's sandbox. Measures P99 latency." },
  track3: { title: "Track 3: Telemetry Engine", desc: "A Go-based stream processor. Consumes high-volume metrics from Redpanda (Kafka), aggregates data, validates constraints, and updates the real-time Postgres/Redis leaderboard." }
};

export function Architecture() {
  const [activeNode, setActiveNode] = useState<keyof typeof archDetails | null>(null);

  return (
    <div className="space-y-6 pb-12 h-full flex flex-col">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight text-gray-100 flex items-center gap-2">
          <Network className="h-6 w-6 text-primary" />
          System Architecture
        </h1>
      </div>

      <div className="flex flex-col lg:flex-row gap-8 flex-1">
        {/* Diagram Area */}
        <div className="flex-1 flex flex-col items-center justify-center py-12 px-4 gap-8">
          
          <motion.div whileHover={{ scale: 1.05 }} onClick={() => setActiveNode('track1')} className="cursor-pointer w-full max-w-sm">
            <Card className={`border-2 transition-colors ${activeNode === 'track1' ? 'border-primary' : 'border-gray-800'}`}>
              <CardContent className="p-6 flex items-center gap-4">
                <div className="p-3 bg-primary/10 text-primary rounded-xl"><Box className="h-8 w-8" /></div>
                <div>
                  <h3 className="font-bold text-gray-100">Track 1</h3>
                  <p className="text-sm text-gray-400">Submission Engine</p>
                </div>
              </CardContent>
            </Card>
          </motion.div>

          <div className="flex flex-col items-center justify-center relative h-16 w-full">
            <motion.div 
              animate={{ y: [0, 10, 0] }} 
              transition={{ repeat: Infinity, duration: 2 }}
              className="absolute text-primary"
            >
              <ArrowDown className="h-8 w-8" />
            </motion.div>
            <span className="absolute right-1/2 translate-x-24 text-xs font-mono text-gray-500">gRPC / K8s API</span>
          </div>

          <motion.div whileHover={{ scale: 1.05 }} onClick={() => setActiveNode('track2')} className="cursor-pointer w-full max-w-sm">
            <Card className={`border-2 transition-colors ${activeNode === 'track2' ? 'border-secondary' : 'border-gray-800'}`}>
              <CardContent className="p-6 flex items-center gap-4">
                <div className="p-3 bg-secondary/10 text-secondary rounded-xl"><Server className="h-8 w-8" /></div>
                <div>
                  <h3 className="font-bold text-gray-100">Track 2</h3>
                  <p className="text-sm text-gray-400">Bot Fleet Load Generator</p>
                </div>
              </CardContent>
            </Card>
          </motion.div>

          <div className="flex flex-col items-center justify-center relative h-16 w-full">
            <motion.div 
              animate={{ y: [0, 10, 0] }} 
              transition={{ repeat: Infinity, duration: 2, delay: 0.5 }}
              className="absolute text-secondary"
            >
              <ArrowDown className="h-8 w-8" />
            </motion.div>
            <span className="absolute left-1/2 -translate-x-24 text-xs font-mono text-gray-500">UDP / Kafka</span>
          </div>

          <motion.div whileHover={{ scale: 1.05 }} onClick={() => setActiveNode('track3')} className="cursor-pointer w-full max-w-sm">
            <Card className={`border-2 transition-colors ${activeNode === 'track3' ? 'border-success' : 'border-gray-800'}`}>
              <CardContent className="p-6 flex items-center gap-4">
                <div className="p-3 bg-success/10 text-success rounded-xl"><Database className="h-8 w-8" /></div>
                <div>
                  <h3 className="font-bold text-gray-100">Track 3</h3>
                  <p className="text-sm text-gray-400">Telemetry Engine</p>
                </div>
              </CardContent>
            </Card>
          </motion.div>
        </div>

        {/* Details Area */}
        <div className="w-full lg:w-96 shrink-0">
          <Card className="h-full bg-gray-900/50 border-dashed">
            <CardContent className="p-6 flex flex-col h-full items-center justify-center text-center">
              <AnimatePresence mode="wait">
                {activeNode ? (
                  <motion.div
                    key={activeNode}
                    initial={{ opacity: 0, y: 10 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -10 }}
                    className="flex flex-col items-center gap-4"
                  >
                    <Info className="h-12 w-12 text-primary opacity-80" />
                    <h3 className="text-xl font-bold text-gray-100">{archDetails[activeNode].title}</h3>
                    <p className="text-gray-400 leading-relaxed text-sm">
                      {archDetails[activeNode].desc}
                    </p>
                  </motion.div>
                ) : (
                  <motion.div
                    key="empty"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    className="text-gray-500 flex flex-col items-center gap-4"
                  >
                    <Cpu className="h-12 w-12 opacity-50" />
                    <p>Click a component on the left to view detailed architectural specifications.</p>
                  </motion.div>
                )}
              </AnimatePresence>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
