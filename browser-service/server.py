"""
AnakinScraper Browser Service

Launches a Playwright Chromium browser in server mode, exposing a WebSocket
endpoint for remote browser automation. Includes a health-check HTTP server
and a watchdog that restarts the browser on crash with exponential backoff.
"""

import logging
import os
import signal
import subprocess
import sys
import threading
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
import json

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger("browser-service")

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

PORT = int(os.environ.get("PORT", "9222"))
WS_PATH = os.environ.get("WS_PATH", "playwright")
HEALTH_CHECK_PORT = int(os.environ.get("HEALTH_CHECK_PORT", "8080"))
HEADLESS = os.environ.get("HEADLESS", "true").lower() == "true"

PROXY_SERVER = os.environ.get("PROXY_SERVER", "")
PROXY_USERNAME = os.environ.get("PROXY_USERNAME", "")
PROXY_PASSWORD = os.environ.get("PROXY_PASSWORD", "")

# Watchdog backoff
INITIAL_BACKOFF = 1
MAX_BACKOFF = 30

# ---------------------------------------------------------------------------
# Global state
# ---------------------------------------------------------------------------

browser_process: subprocess.Popen | None = None
browser_running = threading.Event()
shutdown_event = threading.Event()
lock = threading.Lock()

# ---------------------------------------------------------------------------
# Health-check HTTP server
# ---------------------------------------------------------------------------


class HealthHandler(BaseHTTPRequestHandler):
    """Minimal HTTP handler that serves /health."""

    def do_GET(self):  # noqa: N802
        if self.path == "/health":
            is_running = browser_running.is_set()
            body = json.dumps({
                "status": "ok" if is_running else "starting",
                "browser": "running" if is_running else "stopped",
            })
            self.send_response(200 if is_running else 503)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.write_body(body)
        else:
            self.send_response(404)
            self.end_headers()

    def write_body(self, body: str):
        self.wfile.write(body.encode())

    def log_message(self, format, *args):  # noqa: A002
        # Silence default access logs to reduce noise
        pass


def start_health_server():
    """Run the health-check HTTP server in a daemon thread."""
    server = HTTPServer(("0.0.0.0", HEALTH_CHECK_PORT), HealthHandler)
    server.timeout = 1
    logger.info("Health-check server listening on :%d", HEALTH_CHECK_PORT)
    while not shutdown_event.is_set():
        server.handle_request()
    server.server_close()

# ---------------------------------------------------------------------------
# Browser lifecycle
# ---------------------------------------------------------------------------


def build_launch_command() -> list[str]:
    """Build the command to start Playwright's Chromium server."""
    cmd = [
        sys.executable, "-m", "playwright", "run-server",
        "--port", str(PORT),
        "--path", f"/{WS_PATH}",
        "--host", "0.0.0.0",
    ]

    return cmd


def start_browser() -> subprocess.Popen:
    """Start the Playwright browser server subprocess."""
    cmd = build_launch_command()
    logger.info("Starting browser: %s", " ".join(cmd))
    proc = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    return proc


def stream_output(proc: subprocess.Popen):
    """Stream subprocess stdout to our logger."""
    if proc.stdout is None:
        return
    for line in iter(proc.stdout.readline, b""):
        decoded = line.decode("utf-8", errors="replace").rstrip()
        if decoded:
            logger.info("[browser] %s", decoded)


def watchdog():
    """
    Main watchdog loop.
    Starts the browser and restarts it on crash with exponential backoff.
    """
    global browser_process
    backoff = INITIAL_BACKOFF

    while not shutdown_event.is_set():
        proc = start_browser()
        with lock:
            browser_process = proc

        # Give the browser a moment to start, then mark as running
        time.sleep(1)
        if proc.poll() is None:
            browser_running.set()
            logger.info(
                "Browser is running on ws://0.0.0.0:%d/%s", PORT, WS_PATH
            )
            backoff = INITIAL_BACKOFF  # reset on successful start

        # Stream output (blocks until process exits)
        stream_output(proc)
        proc.wait()

        browser_running.clear()

        if shutdown_event.is_set():
            logger.info("Browser stopped (shutdown requested)")
            break

        exit_code = proc.returncode
        logger.warning(
            "Browser exited with code %d. Restarting in %ds...",
            exit_code,
            backoff,
        )

        # Wait with backoff, but remain responsive to shutdown
        for _ in range(int(backoff * 10)):
            if shutdown_event.is_set():
                return
            time.sleep(0.1)

        backoff = min(backoff * 2, MAX_BACKOFF)

# ---------------------------------------------------------------------------
# Graceful shutdown
# ---------------------------------------------------------------------------


def handle_shutdown(signum, frame):
    """Handle SIGTERM / SIGINT for graceful shutdown."""
    sig_name = signal.Signals(signum).name
    logger.info("Received %s, shutting down...", sig_name)
    shutdown_event.set()

    with lock:
        if browser_process and browser_process.poll() is None:
            logger.info("Terminating browser process (pid=%d)", browser_process.pid)
            browser_process.terminate()
            try:
                browser_process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                logger.warning("Browser did not exit in time, killing...")
                browser_process.kill()
                browser_process.wait(timeout=5)

    logger.info("Shutdown complete.")

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main():
    signal.signal(signal.SIGTERM, handle_shutdown)
    signal.signal(signal.SIGINT, handle_shutdown)

    logger.info("AnakinScraper Browser Service starting")
    logger.info("  WebSocket : ws://0.0.0.0:%d/%s", PORT, WS_PATH)
    logger.info("  Health    : http://0.0.0.0:%d/health", HEALTH_CHECK_PORT)
    logger.info("  Headless  : %s", HEADLESS)
    logger.info("  Proxy     : %s", PROXY_SERVER or "(none)")

    # Start health-check server in background thread
    health_thread = threading.Thread(target=start_health_server, daemon=True)
    health_thread.start()

    # Run watchdog in main thread
    watchdog()


if __name__ == "__main__":
    main()
