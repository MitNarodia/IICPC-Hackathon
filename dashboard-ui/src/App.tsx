import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { MainLayout } from './layouts/MainLayout';
import { Landing } from './pages/Landing';
import { Dashboard } from './pages/Dashboard';
import { Submissions } from './pages/Submissions';
import { Sandbox } from './pages/Sandbox';
import { Fleet } from './pages/Fleet';
import { Telemetry } from './pages/Telemetry';
import { Leaderboard } from './pages/Leaderboard';
import { Architecture } from './pages/Architecture';
import { Docs } from './pages/Docs';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<MainLayout />}>
          <Route path="/" element={<Landing />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/submissions" element={<Submissions />} />
          <Route path="/sandbox" element={<Sandbox />} />
          <Route path="/fleet" element={<Fleet />} />
          <Route path="/telemetry" element={<Telemetry />} />
          <Route path="/leaderboard" element={<Leaderboard />} />
          <Route path="/architecture" element={<Architecture />} />
          <Route path="/docs" element={<Docs />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
