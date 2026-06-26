import React from 'react';
import { motion } from 'framer-motion';
import { Link } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/Card';
import { Box, Network, Cpu, ArrowRight, Activity, Zap, Shield } from 'lucide-react';

export function Landing() {
  const container = {
    hidden: { opacity: 0 },
    show: {
      opacity: 1,
      transition: { staggerChildren: 0.1 }
    }
  };

  const item = {
    hidden: { opacity: 0, y: 20 },
    show: { opacity: 1, y: 0 }
  };

  return (
    <div className="flex flex-col gap-16 pb-16">
      {/* Hero Section */}
      <section className="pt-20 text-center max-w-4xl mx-auto px-4">
        <motion.div initial={{ opacity: 0, y: -20 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.5 }}>
          <h1 className="text-5xl font-extrabold tracking-tight text-white sm:text-7xl mb-6">
            Distributed Systems <span className="text-primary">Benchmarking</span> Engine
          </h1>
          <p className="text-xl text-gray-400 mb-10 max-w-2xl mx-auto">
            A production-grade platform for evaluating high-frequency trading engines. Built with Kubernetes, Redpanda, and Go.
          </p>
          <div className="flex items-center justify-center gap-4">
            <Link to="/dashboard">
              <Button size="lg" className="gap-2">
                Open Dashboard <ArrowRight className="h-4 w-4" />
              </Button>
            </Link>
            <Link to="/docs">
              <Button size="lg" variant="outline">Documentation</Button>
            </Link>
            <a href="https://github.com" target="_blank" rel="noreferrer">
              <Button size="lg" variant="ghost">GitHub</Button>
            </a>
          </div>
        </motion.div>
      </section>

      {/* System Statistics */}
      <section className="max-w-7xl mx-auto w-full px-4">
        <motion.div 
          className="grid grid-cols-1 md:grid-cols-3 gap-6"
          variants={container}
          initial="hidden"
          animate="show"
        >
          {[
            { label: 'Max Throughput', value: '1.2M req/sec', icon: Zap, color: 'text-yellow-500' },
            { label: 'P99 Latency', value: '< 5ms', icon: Activity, color: 'text-primary' },
            { label: 'Reliability', value: '99.99%', icon: Shield, color: 'text-success' },
          ].map((stat, i) => (
            <motion.div key={i} variants={item}>
              <Card className="bg-card/50 backdrop-blur-sm border-gray-800">
                <CardContent className="p-6 flex items-center gap-4">
                  <div className={`p-3 rounded-xl bg-gray-900 ${stat.color}`}>
                    <stat.icon className="h-6 w-6" />
                  </div>
                  <div>
                    <p className="text-sm font-medium text-gray-400">{stat.label}</p>
                    <p className="text-3xl font-bold text-gray-100">{stat.value}</p>
                  </div>
                </CardContent>
              </Card>
            </motion.div>
          ))}
        </motion.div>
      </section>

      {/* Architecture Overview */}
      <section className="max-w-7xl mx-auto w-full px-4">
        <div className="text-center mb-12">
          <h2 className="text-3xl font-bold text-gray-100">Three-Track Architecture</h2>
          <p className="text-gray-400 mt-2">A decoupled approach to benchmarking</p>
        </div>
        
        <motion.div 
          className="grid grid-cols-1 lg:grid-cols-3 gap-8"
          variants={container}
          initial="hidden"
          whileInView="show"
          viewport={{ once: true }}
        >
          <motion.div variants={item}>
            <Card className="h-full hover:border-primary/50 transition-colors">
              <CardHeader>
                <div className="h-12 w-12 bg-primary/10 text-primary rounded-lg flex items-center justify-center mb-4">
                  <Box className="h-6 w-6" />
                </div>
                <CardTitle>Track 1: Submission Engine</CardTitle>
                <CardDescription>Container lifecycle management</CardDescription>
              </CardHeader>
              <CardContent className="text-gray-400 text-sm">
                Manages contestant uploads, Docker builds, and secure sandbox deployments on Kubernetes. Health is monitored in real-time.
              </CardContent>
            </Card>
          </motion.div>

          <motion.div variants={item}>
            <Card className="h-full hover:border-secondary/50 transition-colors">
              <CardHeader>
                <div className="h-12 w-12 bg-secondary/10 text-secondary rounded-lg flex items-center justify-center mb-4">
                  <Network className="h-6 w-6" />
                </div>
                <CardTitle>Track 2: Bot Fleet</CardTitle>
                <CardDescription>Distributed load generation</CardDescription>
              </CardHeader>
              <CardContent className="text-gray-400 text-sm">
                Spawns thousands of concurrent WebSockets to simulate high-frequency market participants. Measures raw latency and throughput.
              </CardContent>
            </Card>
          </motion.div>

          <motion.div variants={item}>
            <Card className="h-full hover:border-success/50 transition-colors">
              <CardHeader>
                <div className="h-12 w-12 bg-success/10 text-success rounded-lg flex items-center justify-center mb-4">
                  <Cpu className="h-6 w-6" />
                </div>
                <CardTitle>Track 3: Telemetry Engine</CardTitle>
                <CardDescription>Real-time analytics pipeline</CardDescription>
              </CardHeader>
              <CardContent className="text-gray-400 text-sm">
                Ingests high-volume metrics via Redpanda, aggregates streams with Go, and serves real-time leaderboards using Redis and Postgres.
              </CardContent>
            </Card>
          </motion.div>
        </motion.div>
      </section>

      {/* Footer */}
      <footer className="border-t border-gray-800 mt-20 py-8 text-center text-gray-500 text-sm">
        <p>IICPC Distributed Systems Benchmarking Platform &copy; 2026</p>
      </footer>
    </div>
  );
}
