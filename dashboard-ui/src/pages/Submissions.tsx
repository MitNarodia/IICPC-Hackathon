import React, { useState, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Card, CardContent } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Search, Filter, MoreHorizontal, X, Clock, CheckCircle2, PlayCircle, Loader2, Plus } from 'lucide-react';
import { NewSubmissionModal } from '@/components/NewSubmissionModal';

export function Submissions() {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [selectedSub, setSelectedSub] = useState<any>(null);
  
  // Real submissions state with localStorage persistence
  const [submissions, setSubmissions] = useState<any[]>(() => {
    const saved = localStorage.getItem('iicpc_submissions');
    if (saved) {
      try {
        return JSON.parse(saved);
      } catch (e) {}
    }
    return [];
  });

  useEffect(() => {
    localStorage.setItem('iicpc_submissions', JSON.stringify(submissions));
  }, [submissions]);

  const handleRefresh = async () => {
    // Automatically transition "RUNNING" submissions to "COMPLETED" if they appear in the real backend's /v1/runs
    try {
      const runsRes = await apiClient.get('/v1/runs');
      const realRuns = runsRes.data.runs || [];
      const runIds = new Set(realRuns.map((r: any) => r.run_id));
      
      setSubmissions(prev => prev.map(sub => {
        if (sub.status === 'RUNNING') {
          const expectedRunId = `demo-run-${sub.contestant}`;
          if (runIds.has(expectedRunId)) {
            return { ...sub, status: 'COMPLETED' };
          }
        }
        return sub;
      }));
    } catch (e) {
      console.error("Failed to fetch runs for status sync", e);
    }
    
    // Animate a spinner or just provide visual feedback if needed
    setTimeout(() => {
      // Done refreshing
    }, 500);
  };

  const openDrawer = (sub: any) => {
    setSelectedSub(sub);
    setDrawerOpen(true);
  };

  const handleNewSubmissionSuccess = (newSub: any) => {
    setSubmissions(prev => [newSub, ...prev]);
  };

  return (
    <div className="space-y-6 pb-12 relative">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <h1 className="text-2xl font-bold tracking-tight text-gray-100">Submissions</h1>
        <div className="flex items-center gap-4">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-500" />
            <input
              type="text"
              placeholder="Search ID or Contestant..."
              className="pl-9 pr-4 py-2 bg-gray-900 border border-gray-800 rounded-md text-sm text-gray-200 focus:outline-none focus:ring-1 focus:ring-primary w-64"
            />
          </div>
          <Button variant="outline" className="gap-2">
            <Filter className="h-4 w-4" /> Filters
          </Button>
          <Button variant="primary" className="gap-2" onClick={() => setModalOpen(true)}>
            <Plus className="h-4 w-4" /> New Submission
          </Button>
        </div>
      </div>

      <Card>
        <div className="overflow-x-auto">
          <table className="w-full text-sm text-left text-gray-400">
            <thead className="text-xs uppercase bg-gray-900 border-b border-gray-800 text-gray-400">
              <tr>
                <th className="px-6 py-4 font-semibold">ID</th>
                <th className="px-6 py-4 font-semibold">Contestant</th>
                <th className="px-6 py-4 font-semibold">Language</th>
                <th className="px-6 py-4 font-semibold">Status</th>
                <th className="px-6 py-4 font-semibold">Created</th>
                <th className="px-6 py-4 font-semibold">Deployment</th>
                <th className="px-6 py-4 font-semibold">Health</th>
                <th className="px-6 py-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {submissions.map((row, idx) => (
                <motion.tr 
                  key={row.id}
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: idx * 0.05 }}
                  className="border-b border-gray-800 hover:bg-gray-800/50 transition-colors"
                >
                  <td className="px-6 py-4 font-mono text-gray-300">{row.id}</td>
                  <td className="px-6 py-4 font-medium text-gray-200">{row.contestant}</td>
                  <td className="px-6 py-4">{row.lang}</td>
                  <td className="px-6 py-4">
                    <Badge variant={row.status === 'COMPLETED' ? 'success' : row.status === 'RUNNING' ? 'default' : row.status === 'FAILED' ? 'danger' : 'warning'}>
                      {row.status}
                    </Badge>
                  </td>
                  <td className="px-6 py-4">{row.created}</td>
                  <td className="px-6 py-4">
                    <Badge variant={row.deployStatus === 'READY' ? 'success' : row.deployStatus === 'FAILED' ? 'danger' : 'outline'}>
                      {row.deployStatus}
                    </Badge>
                  </td>
                  <td className="px-6 py-4">
                    <div className="flex items-center gap-2">
                      <span className={`h-2 w-2 rounded-full ${row.health === 'HEALTHY' ? 'bg-success' : row.health === 'DEAD' ? 'bg-danger' : 'bg-gray-500'}`} />
                      {row.health}
                    </div>
                  </td>
                  <td className="px-6 py-4 text-right">
                    <Button variant="ghost" size="sm" onClick={() => openDrawer(row)}>
                      Details <MoreHorizontal className="ml-2 h-4 w-4" />
                    </Button>
                  </td>
                </motion.tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>

      <NewSubmissionModal 
        isOpen={modalOpen} 
        onClose={() => setModalOpen(false)} 
        onSuccess={handleNewSubmissionSuccess} 
      />

      {/* Detail Drawer overlay */}
      <AnimatePresence>
        {drawerOpen && selectedSub && (
          <>
            <motion.div 
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              className="fixed inset-0 bg-black/50 z-30 backdrop-blur-sm"
              onClick={() => setDrawerOpen(false)}
            />
            <motion.div
              initial={{ x: '100%' }}
              animate={{ x: 0 }}
              exit={{ x: '100%' }}
              transition={{ type: 'spring', damping: 25, stiffness: 200 }}
              className="fixed inset-y-0 right-0 w-full max-w-md bg-card border-l border-gray-800 z-40 shadow-2xl flex flex-col"
            >
              <div className="p-6 border-b border-gray-800 flex items-center justify-between">
                <div>
                  <h2 className="text-lg font-bold text-gray-100">Submission Details</h2>
                  <p className="text-sm text-gray-400 font-mono">{selectedSub.id}</p>
                </div>
                <Button variant="ghost" size="sm" onClick={() => setDrawerOpen(false)} className="px-2">
                  <X className="h-5 w-5" />
                </Button>
              </div>
              <div className="flex-1 overflow-y-auto p-6 space-y-8">
                <div>
                  <h3 className="text-sm font-semibold text-gray-400 mb-4 uppercase tracking-wider">Lifecycle Timeline</h3>
                  <div className="space-y-6">
                    <div className="flex gap-4">
                      <div className="mt-1"><CheckCircle2 className="h-5 w-5 text-success" /></div>
                      <div>
                        <p className="text-sm font-medium text-gray-200">Artifact Uploaded</p>
                        <p className="text-xs text-gray-500">S3 bucket received tar.gz</p>
                      </div>
                    </div>
                    <div className="flex gap-4">
                      <div className="mt-1"><CheckCircle2 className="h-5 w-5 text-success" /></div>
                      <div>
                        <p className="text-sm font-medium text-gray-200">Build Succeeded</p>
                        <p className="text-xs text-gray-500">Docker image compiled in 45s</p>
                      </div>
                    </div>
                    <div className="flex gap-4">
                      <div className="mt-1">
                        {selectedSub.status === 'RUNNING' ? <Loader2 className="h-5 w-5 text-primary animate-spin" /> : 
                         selectedSub.status === 'BUILDING' ? <Clock className="h-5 w-5 text-yellow-500" /> :
                         <PlayCircle className="h-5 w-5 text-success" />}
                      </div>
                      <div>
                        <p className="text-sm font-medium text-gray-200">Sandbox Deployment</p>
                        <p className="text-xs text-gray-500">{selectedSub.deployStatus}</p>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="space-y-4">
                  <h3 className="text-sm font-semibold text-gray-400 uppercase tracking-wider">Quick Actions</h3>
                  <Button className="w-full justify-start gap-2" variant="outline">
                    View Build Logs
                  </Button>
                  <Button className="w-full justify-start gap-2" variant="outline">
                    View Sandbox Logs
                  </Button>
                  <Button className="w-full justify-start gap-2" variant="danger">
                    Terminate Run
                  </Button>
                </div>
              </div>
            </motion.div>
          </>
        )}
      </AnimatePresence>
    </div>
  );
}
