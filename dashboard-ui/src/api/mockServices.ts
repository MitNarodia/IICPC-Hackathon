import axios from 'axios';

// Create a configured Axios instance
export const apiClient = axios.create({
  // BaseURL is not needed if we rely on Vite proxy mapping absolute paths directly
  // but we can set it to root to use the proxy
  baseURL: '',
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add interceptor
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    console.error('API Error:', error);
    return Promise.reject(error);
  }
);

export interface CreateSubmissionRequest {
  contestant_id: string;
  language: string;
  submission_type: string;
  entrypoint: string;
  declared_port: number;
}

// --- Submission API ---
export const SubmissionService = {
  // We'll mock the list of submissions for now since the backend might not have a GET all endpoint
  // Wait, let's check if the backend has a GET all endpoint.
  // We'll keep a local array in memory just for the UI.
  getSubmissions: async () => {
    return [];
  },
  
  createSubmission: async (data: CreateSubmissionRequest) => {
    const res = await apiClient.post('/v1/submissions', data);
    return res.data; // Should contain { id, upload_url, etc }
  },
  
  uploadArtifact: async (uploadUrl: string, file: File) => {
    // The backend returns a URL pointing to 'minio:9000' which the browser cannot resolve.
    // It also might block CORS. We'll rewrite it to use our Vite proxy.
    // We also must replace any literal `\u0026` with `&` as Go's JSON encoder escapes them.
    const proxyUrl = uploadUrl.replace('http://minio:9000', '/api/minio').replace(/\\u0026/g, '&');
    
    await axios.put(proxyUrl, file, {
      headers: {
        'Content-Type': file.type || 'application/octet-stream'
      }
    });
  },

  getDeploymentStatus: async (id: string) => {
    const res = await apiClient.get(`/v1/submissions/${id}/deployment`);
    return res.data;
  },
  
  triggerBotFleet: async (submissionId: string, uuid: string) => {
    const res = await axios.post('/api/trigger-fleet', { submissionId, uuid });
    return res.data;
  }
};

// --- Telemetry API ---
export const TelemetryService = {
  getMetrics: async (timeRange: string) => {
    return { latency: 2.4, throughput: 15200 };
  }
};

// --- Leaderboard API ---
export const LeaderboardService = {
  getRankings: async () => {
    // The leaderboard API in Go requires a '?run=' parameter
    try {
      const runsRes = await apiClient.get('/v1/runs');
      const runs = runsRes.data.runs;
      if (!runs || runs.length === 0) return [];

      // Fetch leaderboard for the most recent run (index 0 is the newest)
      const latestRunId = runs[0].run_id;
      
      const res = await apiClient.get(`/v1/leaderboard?run=${latestRunId}`);
      // The Go backend returns an array of entries with composite scores
      return res.data.entries || [];
    } catch (e) {
      console.error("Failed to fetch real leaderboard:", e);
      return [];
    }
  }
};
