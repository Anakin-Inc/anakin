import { Component, type ReactNode } from 'react';

interface ErrorBoundaryProps {
  children: ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

/**
 * Catches render errors anywhere in the app and shows a recoverable error card
 * instead of unmounting the whole dashboard as a blank screen.
 */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  private handleReload = () => {
    this.setState({ error: null });
  };

  render() {
    if (this.state.error) {
      return (
        <div className="min-h-screen flex items-center justify-center bg-zinc-950 p-4">
          <div className="card p-8 max-w-md w-full text-center">
            <h1 className="text-xl font-bold text-zinc-100 mb-2">Something went wrong</h1>
            <p className="text-sm text-zinc-400 mb-4">
              The dashboard hit an unexpected error. Reload to continue.
            </p>
            <p className="text-xs text-zinc-600 font-mono bg-zinc-950 border border-zinc-800 rounded-lg p-3 mb-5 break-words">
              {this.state.error.message || String(this.state.error)}
            </p>
            <div className="flex justify-center gap-3">
              <button onClick={this.handleReload} className="btn-primary">
                Try again
              </button>
              <button onClick={() => window.location.reload()} className="btn-secondary">
                Reload page
              </button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
