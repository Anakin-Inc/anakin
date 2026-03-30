import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Layout } from './components/Layout';
import { Dashboard } from './pages/Dashboard';
import { Scrape } from './pages/Scrape';
import { Jobs } from './pages/Jobs';
import { JobDetail } from './pages/JobDetail';
import { DomainConfigs } from './pages/DomainConfigs';
import { ProxyScores } from './pages/ProxyScores';

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/scrape" element={<Scrape />} />
          <Route path="/jobs" element={<Jobs />} />
          <Route path="/jobs/:id" element={<JobDetail />} />
          <Route path="/domains" element={<DomainConfigs />} />
          <Route path="/proxies" element={<ProxyScores />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
