import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'
import { exec } from 'child_process'

const botFleetPlugin = () => ({
  name: 'bot-fleet-trigger',
  configureServer(server: any) {
    server.middlewares.use('/api/trigger-fleet', (req: any, res: any) => {
      let body = '';
      req.on('data', (chunk: any) => { body += chunk.toString(); });
      req.on('end', () => {
        try {
          const { submissionId, uuid } = JSON.parse(body);
          if (!submissionId) throw new Error("Missing submissionId");
          
          const cmd = `cd ../bot_fleet && docker compose run --rm --no-deps -e TRACK3_INGEST_URL=http://track3-ingestion-service-1:8080 -e TRACK3_SUBMISSION_ID=${submissionId} -e TRACK3_RUN_ID=demo-run-${uuid} bot_fleet --track1-api http://track1-submission-api-1:8080 --submission-id ${submissionId} --bots 50 --orders 20`;
          
          // Trigger in background
          exec(cmd, (err, stdout, stderr) => {
            if (err) console.error("Bot Fleet Error:", err);
            else console.log("Bot Fleet Completed:", stdout);
          });
          
          res.setHeader('Content-Type', 'application/json');
          res.end(JSON.stringify({ status: 'started' }));
        } catch (e: any) {
          res.statusCode = 400;
          res.end(JSON.stringify({ error: e.message }));
        }
      });
    });
  }
});

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), botFleetPlugin()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/v1/submissions': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/v1/leaderboard': {
        target: 'http://localhost:8082',
        changeOrigin: true,
      },
      '/v1/runs': {
        target: 'http://localhost:8082',
        changeOrigin: true,
      },
      '/api/minio': {
        target: 'http://localhost:9000',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api\/minio/, ''),
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq) => {
            // S3 presigned URLs strictly validate the Host header
            proxyReq.setHeader('Host', 'minio:9000');
          });
        }
      }
    }
  }
})
