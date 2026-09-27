import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Layout } from './components/Layout';
import { ErrorBoundary } from './components/ErrorBoundary';
import { Dashboard } from './pages/Dashboard';
import { Scrape } from './pages/Scrape';
import { Jobs } from './pages/Jobs';
import { JobDetail } from './pages/JobDetail';
import { DomainConfigs } from './pages/DomainConfigs';
import { ProxyScores } from './pages/ProxyScores';
import { NotFound } from './pages/NotFound';

export default function App() {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<Dashboard />} />
            <Route path="/scrape" element={<Scrape />} />
            <Route path="/jobs" element={<Jobs />} />
            <Route path="/jobs/:id" element={<JobDetail />} />
            <Route path="/domains" element={<DomainConfigs />} />
            <Route path="/proxies" element={<ProxyScores />} />
            <Route path="*" element={<NotFound />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
