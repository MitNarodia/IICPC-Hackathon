import React, { useState, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Card, CardContent } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Trophy, ChevronDown, ChevronUp, TrendingUp, AlertCircle, RefreshCw } from 'lucide-react';
import { LineChart, Line, ResponsiveContainer, YAxis } from 'recharts';
import { LeaderboardService } from '@/api/mockServices';

export function Leaderboard() {
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [leaderboardData, setLeaderboardData] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let interval: ReturnType<typeof setInterval>;
    
    const fetchLeaderboard = async () => {
      try {
        const data = await LeaderboardService.getRankings();
        setLeaderboardData(data || []);
        setError(null);
      } catch (err: any) {
        console.error(err);
        setError("Failed to fetch live leaderboard data.");
      } finally {
        setLoading(false);
      }
    };
    
    fetchLeaderboard();
    interval = setInterval(fetchLeaderboard, 2000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="space-y-6 pb-12">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight text-gray-100 flex items-center gap-2">
          <Trophy className="h-6 w-6 text-yellow-500" />
          Global Leaderboard
        </h1>
        <Badge variant="success" className="animate-pulse">Live Updates Active</Badge>
      </div>

      {error && (
        <div className="p-4 bg-danger/10 border border-danger/20 rounded-md flex items-start gap-3">
          <AlertCircle className="h-5 w-5 text-danger shrink-0 mt-0.5" />
          <p className="text-sm text-danger">{error}</p>
        </div>
      )}

      <Card>
        <div className="overflow-x-auto">
          <table className="w-full text-sm text-left text-gray-400">
            <thead className="text-xs uppercase bg-gray-900 border-b border-gray-800 text-gray-400">
              <tr>
                <th className="px-6 py-4 font-semibold">Rank</th>
                <th className="px-6 py-4 font-semibold">Contestant / Run ID</th>
                <th className="px-6 py-4 font-semibold text-right">Composite Score</th>
                <th className="px-6 py-4 font-semibold text-right">P99 Latency (ms)</th>
                <th className="px-6 py-4 font-semibold text-right">Throughput (req/s)</th>
                <th className="px-6 py-4 font-semibold text-center">Correctness</th>
                <th className="px-6 py-4"></th>
              </tr>
            </thead>
            <tbody>
              {loading && leaderboardData.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-6 py-12 text-center text-gray-500">
                    <RefreshCw className="h-6 w-6 animate-spin mx-auto mb-2" />
                    Loading rankings...
                  </td>
                </tr>
              ) : leaderboardData.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-6 py-12 text-center text-gray-500">
                    No benchmarks have been run yet.
                  </td>
                </tr>
              ) : (
                leaderboardData.map((row, index) => (
                  <React.Fragment key={row.submission_id}>
                    <motion.tr 
                      initial={{ opacity: 0, x: -20 }}
                      animate={{ opacity: 1, x: 0 }}
                      transition={{ delay: index * 0.1 }}
                      className={`border-b border-gray-800 hover:bg-gray-800/50 transition-colors cursor-pointer ${expandedId === row.submission_id ? 'bg-gray-800/30' : ''}`}
                      onClick={() => setExpandedId(expandedId === row.submission_id ? null : row.submission_id)}
                    >
                      <td className="px-6 py-4 font-bold text-gray-100">
                        {index === 0 ? <span className="text-yellow-500 text-lg">#{index + 1}</span> : 
                         index === 1 ? <span className="text-gray-300 text-lg">#{index + 1}</span> :
                         index === 2 ? <span className="text-amber-600 text-lg">#{index + 1}</span> :
                         `#${index + 1}`}
                      </td>
                      <td className="px-6 py-4 font-medium text-gray-200">{row.submission_id || row.run_id}</td>
                      <td className="px-6 py-4 text-right font-bold text-primary">{row.composite?.toFixed(2)}</td>
                      <td className="px-6 py-4 text-right">{(row.p99_us / 1000)?.toFixed(2)}</td>
                      <td className="px-6 py-4 text-right">{row.tps?.toLocaleString()}</td>
                      <td className="px-6 py-4 text-center">
                        <Badge variant={row.correctness_score >= 99 ? 'success' : 'warning'}>{row.correctness_score?.toFixed(0)}%</Badge>
                      </td>
                      <td className="px-6 py-4 text-right">
                        {expandedId === row.submission_id ? <ChevronUp className="h-5 w-5 text-gray-500 inline" /> : <ChevronDown className="h-5 w-5 text-gray-500 inline" />}
                      </td>
                    </motion.tr>
                    <AnimatePresence>
                      {expandedId === row.submission_id && (
                        <motion.tr
                          initial={{ opacity: 0, height: 0 }}
                          animate={{ opacity: 1, height: 'auto' }}
                          exit={{ opacity: 0, height: 0 }}
                          className="bg-gray-900/50 border-b border-gray-800"
                        >
                          <td colSpan={7} className="px-6 py-6">
                            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                              <Card className="bg-card">
                                <CardContent className="p-4 flex flex-col justify-center h-full">
                                  <h4 className="text-xs font-semibold text-gray-400 mb-2">Detailed Metrics</h4>
                                  <div className="space-y-2 text-sm">
                                    <div className="flex justify-between"><span className="text-gray-500">Status:</span> <Badge>{row.health}</Badge></div>
                                    <div className="flex justify-between"><span className="text-gray-500">Run ID:</span> <span className="text-gray-200">{row.run_id}</span></div>
                                    <div className="flex justify-between"><span className="text-gray-500">Error Rate:</span> <span className="text-red-400">{(row.error_rate * 100)?.toFixed(2)}%</span></div>
                                    <div className="flex justify-between"><span className="text-gray-500">Stability:</span> <span className="text-success">{row.stability_score?.toFixed(2)}/100</span></div>
                                  </div>
                                </CardContent>
                              </Card>
                              <Card className="bg-card">
                                <CardContent className="p-4 flex flex-col justify-center h-full">
                                  <h4 className="text-xs font-semibold text-gray-400 mb-2">Latency Percentiles</h4>
                                  <div className="space-y-4">
                                    <div>
                                      <div className="flex justify-between text-xs mb-1"><span className="text-gray-400">P50 Latency</span> <span className="text-gray-200">{(row.p50_us / 1000)?.toFixed(2)}ms</span></div>
                                      <div className="w-full bg-gray-800 rounded-full h-1.5"><div className="bg-blue-500 h-1.5 rounded-full" style={{ width: '50%' }}></div></div>
                                    </div>
                                    <div>
                                      <div className="flex justify-between text-xs mb-1"><span className="text-gray-400">P99 Latency</span> <span className="text-gray-200">{(row.p99_us / 1000)?.toFixed(2)}ms</span></div>
                                      <div className="w-full bg-gray-800 rounded-full h-1.5"><div className="bg-purple-500 h-1.5 rounded-full" style={{ width: '99%' }}></div></div>
                                    </div>
                                  </div>
                                </CardContent>
                              </Card>
                            </div>
                          </td>
                        </motion.tr>
                      )}
                    </AnimatePresence>
                  </React.Fragment>
                ))
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
