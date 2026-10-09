// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/netguard"
	"github.com/playwright-community/playwright-go"
)

// BrowserHandler implements ScrapingHandler using Playwright browser automation.
type BrowserHandler struct {
	wsURL    string
	timeout  time.Duration
	loadWait time.Duration
	pw       *playwright.Playwright
	once     sync.Once
	initErr  error

	allowPrivateTargets bool
	// guard decides whether the browser may reach a URL. Tests swap it to mark a
	// loopback server as internal.
	guard func(ctx context.Context, u *url.URL) error
}

// NewBrowserHandler creates a browser handler. Unless allowPrivateTargets is set,
// every request the page makes is checked against netguard, since the browser's
// dialer lives in another process and the Go-side dial guard can't see it.
func NewBrowserHandler(wsURL string, timeout, loadWait time.Duration, allowPrivateTargets bool) *BrowserHandler {
	return &BrowserHandler{
		wsURL:               wsURL,
		timeout:             timeout,
		loadWait:            loadWait,
		allowPrivateTargets: allowPrivateTargets,
		guard:               hostGuard,
	}
}

func hostGuard(ctx context.Context, u *url.URL) error {
	return netguard.ValidateHost(ctx, u.Hostname())
}

func (h *BrowserHandler) Name() string                                               { return "browser" }
func (h *BrowserHandler) CanHandle(_ context.Context, _ *models.HandlerRequest) bool { return true }

func (h *BrowserHandler) IsHealthy() bool {
	h.ensurePlaywright()
	return h.initErr == nil
}

func (h *BrowserHandler) ensurePlaywright() error {
	h.once.Do(func() {
		pw, err := playwright.Run()
		if err != nil {
			h.initErr = fmt.Errorf("failed to start playwright: %w", err)
			return
		}
		h.pw = pw
	})
	return h.initErr
}

func (h *BrowserHandler) Scrape(ctx context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error) {
	if err := h.ensurePlaywright(); err != nil {
		return nil, err
	}

	browser, err := h.pw.Chromium.Connect(h.wsURL, playwright.BrowserTypeConnectOptions{
		Timeout: playwright.Float(float64(h.timeout.Milliseconds())),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to browser at %s: %w", h.wsURL, err)
	}
	defer func() {
		if disconnectErr := browser.Close(); disconnectErr != nil {
			slog.Warn("failed to close browser connection", "error", disconnectErr)
		}
	}()

	var ctxOpts playwright.BrowserNewContextOptions
	if !h.allowPrivateTargets {
		// Service worker fetches bypass request routing.
		ctxOpts.ServiceWorkers = playwright.ServiceWorkerPolicyBlock
	}
	browserCtx, err := browser.NewContext(ctxOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create browser context: %w", err)
	}
	defer func() {
		if closeErr := browserCtx.Close(); closeErr != nil {
			slog.Warn("failed to close browser context", "error", closeErr)
		}
	}()

	var guard *requestGuard
	if !h.allowPrivateTargets {
		guard = &requestGuard{ctx: ctx, check: h.guard, seen: map[string]error{}}
		if err := guard.install(browserCtx); err != nil {
			return nil, err
		}
	}

	page, err := browserCtx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}
	if guard != nil {
		guard.watch(page)
	}

	timeout := h.timeout.Milliseconds()
	_, err = page.Goto(req.URL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(float64(timeout)),
	})
	if guard != nil {
		if blockedErr := guard.blocked(); blockedErr != nil {
			return nil, blockedErr
		}
	}
	if err != nil {
		return nil, fmt.Errorf("navigation failed: %w", err)
	}

	if h.loadWait > 0 {
		select {
		case <-time.After(h.loadWait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// The page may have navigated during the load wait; never read back a
	// document that came from a blocked address.
	if guard != nil {
		if blockedErr := guard.blocked(); blockedErr != nil {
			return nil, blockedErr
		}
		if err := guard.allowed(page.URL()); err != nil {
			return nil, err
		}
	}

	html, err := page.Content()
	if err != nil {
		return nil, fmt.Errorf("failed to get page content: %w", err)
	}

	return &models.ScrapeResult{
		HTML:       html,
		StatusCode: 200,
	}, nil
}

// requestGuard applies netguard to what one browser context fetches. It is the
// in-process half of the defence; the egress proxy in front of the browser
// (smokescreen in docker-compose.yml) is the other and enforces it at connect time.
//
// Request routing sees the first hop of every request (navigations, sub-resources,
// fetch/XHR) and aborts blocked ones before they leave the browser. Redirect hops are
// followed inside the browser and bypass routing, so main-frame navigations are also
// watched: if one lands on a blocked address the scrape fails and its content is
// never returned. WebSockets, redirect hops and DNS rebinding are only stopped by the
// egress proxy — Playwright's WebSocket routing is a JS shim that Camoufox runs in
// an isolated world, out of reach of page scripts.
type requestGuard struct {
	ctx   context.Context
	check func(ctx context.Context, u *url.URL) error

	mu   sync.Mutex
	seen map[string]error // per-host verdicts, so sub-resources don't re-resolve
	err  error            // first blocked main-frame navigation
}

func (g *requestGuard) install(browserCtx playwright.BrowserContext) error {
	err := browserCtx.Route("**/*", func(route playwright.Route) {
		if err := g.allowed(route.Request().URL()); err != nil {
			slog.Debug("browser request blocked", "url", route.Request().URL(), "error", err)
			route.Abort("blockedbyclient")
			return
		}
		route.Continue()
	})
	if err != nil {
		return fmt.Errorf("failed to install request guard: %w", err)
	}
	return nil
}

// watch records a blocked main-frame navigation, including redirect hops that
// routing never sees.
func (g *requestGuard) watch(page playwright.Page) {
	page.OnRequest(func(req playwright.Request) {
		if !req.IsNavigationRequest() || req.Frame() != page.MainFrame() {
			return
		}
		if err := g.allowed(req.URL()); err != nil {
			g.mu.Lock()
			if g.err == nil {
				g.err = err
			}
			g.mu.Unlock()
		}
	})
}

func (g *requestGuard) blocked() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.err
}

// allowed reports whether the browser may fetch rawURL.
func (g *requestGuard) allowed(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("unparseable browser URL %q: %w", rawURL, err)
	}
	switch u.Scheme {
	case "http", "https", "ws", "wss":
	case "data", "blob", "about":
		return nil // no network access
	default:
		return fmt.Errorf("blocked %s: scheme %q is not allowed", rawURL, u.Scheme)
	}

	host := u.Host
	g.mu.Lock()
	verdict, ok := g.seen[host]
	g.mu.Unlock()
	if !ok {
		verdict = g.check(g.ctx, u)
		if verdict != nil {
			verdict = fmt.Errorf("blocked browser request to %s: %w", u.Redacted(), verdict)
		}
		g.mu.Lock()
		g.seen[host] = verdict
		g.mu.Unlock()
	}
	return verdict
}

// Stop cleans up the Playwright driver.
func (h *BrowserHandler) Stop() {
	if h.pw != nil {
		if err := h.pw.Stop(); err != nil {
			slog.Warn("failed to stop playwright", "error", err)
		}
	}
}
