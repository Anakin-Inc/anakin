import { Link } from 'react-router-dom';

export function NotFound() {
  return (
    <div className="p-6 max-w-xl mx-auto mt-12">
      <div className="card p-12 text-center space-y-4">
        <div className="text-4xl font-bold text-zinc-400 font-mono">404</div>
        <h1 className="text-xl font-semibold text-zinc-100">Page not found</h1>
        <p className="text-sm text-zinc-400">
          The page you are looking for does not exist or has been moved.
        </p>
        <div className="pt-2">
          <Link to="/" className="btn-primary inline-block">
            Back to Dashboard
          </Link>
        </div>
      </div>
    </div>
  );
}
