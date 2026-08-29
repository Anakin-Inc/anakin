import { BrowserRouter, Routes, Route, Link } from 'react-router-dom';
import { Layout } from './components/Layout';
import { ErrorBoundary } from './components/ErrorBoundary';
import { Dashboard } from './pages/Dashboard';
import { Scrape } from './pages/Scrape';
import { Jobs } from './pages/Jobs';
import { JobDetail } from './pages/JobDetail';
import { DomainConfigs } from './pages/DomainConfigs';
import { ProxyScores } from './pages/ProxyScores';

function NotFound() {
  return (
    <div className="py-16 text-center">
      <h1 className="text-6xl font-bold text-zinc-800 mb-4">404</h1>
      <h2 className="text-xl font-semibold text-zinc-100 mb-2">Page not found</h2>
      <p className="text-zinc-500 mb-6">
        The page you're looking for doesn't exist or has been moved.
      </p>
      <Link to="/" className="btn-primary inline-block">
        Back to Dashboard
      </Link>
    </div>
  );
}

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
