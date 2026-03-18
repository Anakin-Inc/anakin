# SPDX-License-Identifier: AGPL-3.0-or-later

"""
AnakinScraper Browser Service

Launches a Camoufox (anti-detect Firefox) browser in server mode, exposing a
Playwright-compatible WebSocket endpoint for remote browser automation.
Includes a health-check HTTP server and a watchdog that restarts the browser
on crash with exponential backoff.
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
WS_PATH = os.environ.get("WS_PATH", "camoufox")
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
            self.wfile.write(body.encode())
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):  # noqa: A002
        pass


def start_health_server():
    server = HTTPServer(("0.0.0.0", HEALTH_CHECK_PORT), HealthHandler)
    server.timeout = 1
    logger.info("Health-check server listening on :%d", HEALTH_CHECK_PORT)
    while not shutdown_event.is_set():
        server.handle_request()
    server.server_close()

# ---------------------------------------------------------------------------
# Browser lifecycle
# ---------------------------------------------------------------------------


def run_browser():
    """Run Camoufox browser server via its Python API.

    Uses the official launch_server() with port and ws_path parameters
    so the WebSocket endpoint is deterministic (ws://0.0.0.0:PORT/WS_PATH).

    Monkey-patches launch_options to strip None values from the config
    dict before it reaches the browser binary.  Camoufox's launch_options()
    always includes ``proxy: None`` which serialises to JSON ``null`` and
    the browser rejects with "proxy: expected object, got null".
    """
    import camoufox.server as _srv  # noqa: E402

    _orig_launch_options = _srv.launch_options

    def _patched_launch_options(**kwargs):
        config = _orig_launch_options(**kwargs)
        return {k: v for k, v in config.items() if v is not None}

    _srv.launch_options = _patched_launch_options

    kwargs: dict = {
        "headless": HEADLESS,
        "port": PORT,
        "ws_path": WS_PATH,
    }

    if PROXY_SERVER:
        kwargs["proxy"] = {
            "server": PROXY_SERVER,
            **({"username": PROXY_USERNAME} if PROXY_USERNAME else {}),
            **({"password": PROXY_PASSWORD} if PROXY_PASSWORD else {}),
        }

    logger.info(
        "Launching camoufox server (headless=%s, port=%d, ws_path=%s)",
        HEADLESS, PORT, WS_PATH,
    )
    _srv.launch_server(**kwargs)


def start_browser() -> subprocess.Popen:
    """Fork a child process that calls run_browser().

    We still need a subprocess so the watchdog can monitor / restart it.
    """
    cmd = [sys.executable, "-c",
           "from server import run_browser; run_browser()"]
    logger.info("Starting browser subprocess")
    proc = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        cwd="/app",
    )
    return proc


def stream_output(proc: subprocess.Popen):
    if proc.stdout is None:
        return
    for line in iter(proc.stdout.readline, b""):
        decoded = line.decode("utf-8", errors="replace").rstrip()
        if decoded:
            logger.info("[browser] %s", decoded)


def watchdog():
    global browser_process
    backoff = INITIAL_BACKOFF

    while not shutdown_event.is_set():
        proc = start_browser()
        with lock:
            browser_process = proc

        time.sleep(3)
        if proc.poll() is None:
            browser_running.set()
            logger.info(
                "Browser is running on ws://0.0.0.0:%d/%s", PORT, WS_PATH
            )
            backoff = INITIAL_BACKOFF

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

        for _ in range(int(backoff * 10)):
            if shutdown_event.is_set():
                return
            time.sleep(0.1)

        backoff = min(backoff * 2, MAX_BACKOFF)

# ---------------------------------------------------------------------------
# Graceful shutdown
# ---------------------------------------------------------------------------


def handle_shutdown(signum, frame):
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

    logger.info("AnakinScraper Browser Service (Camoufox) starting")
    logger.info("  WebSocket : ws://0.0.0.0:%d/%s", PORT, WS_PATH)
    logger.info("  Health    : http://0.0.0.0:%d/health", HEALTH_CHECK_PORT)
    logger.info("  Headless  : %s", HEADLESS)
    logger.info("  Proxy     : %s", PROXY_SERVER or "(none)")

    health_thread = threading.Thread(target=start_health_server, daemon=True)
    health_thread.start()

    watchdog()


if __name__ == "__main__":
    main()
