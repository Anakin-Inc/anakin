// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// newCountingProxy starts an HTTP server that answers proxied (absolute-URI)
// requests and reports how many TCP connections it has accepted.
func newCountingProxy(t *testing.T, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var conns atomic.Int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(body))
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	return srv, &conns
}

// A transport owns its connection pool, so building one per request means no scrape
// can ever reuse a connection: every one costs a fresh handshake, and the transport
// it belonged to is dropped without CloseIdleConnections, leaving the socket and its
// goroutines alive until IdleConnTimeout. Proxies are selected per job by the proxy
// pool, so that is the normal path whenever PROXY_URLS is configured.
func TestHTTPHandler_ProxyConnectionsAreReused(t *testing.T) {
	proxy, conns := newCountingProxy(t, "<html><body>proxied</body></html>")

	h := NewHTTPHandler(5*time.Second, "", false)
	req := &models.HandlerRequest{URL: "http://target.example/", ProxyURL: proxy.URL}

	const scrapes = 3
	for i := 0; i < scrapes; i++ {
		res, err := h.Scrape(context.Background(), req)
		if err != nil {
			t.Fatalf("scrape %d: %v", i+1, err)
		}
		if !strings.Contains(res.HTML, "proxied") {
			t.Fatalf("scrape %d: unexpected body %q", i+1, res.HTML)
		}
	}

	if got := conns.Load(); got != 1 {
		t.Errorf("proxy accepted %d connections across %d scrapes, want 1", got, scrapes)
	}
}

func TestHTTPHandler_ProxyClientsAreCachedPerURL(t *testing.T) {
	h := NewHTTPHandler(5*time.Second, "", false)

	first, err := h.proxyClient("http://proxy-a.example:8080")
	if err != nil {
		t.Fatalf("proxyClient: %v", err)
	}
	again, err := h.proxyClient("http://proxy-a.example:8080")
	if err != nil {
		t.Fatalf("proxyClient: %v", err)
	}
	if first != again {
		t.Error("the same proxy URL produced two clients, so its connection pool is not shared")
	}

	other, err := h.proxyClient("http://proxy-b.example:8080")
	if err != nil {
		t.Fatalf("proxyClient: %v", err)
	}
	if first == other {
		t.Error("two different proxy URLs shared one client")
	}
}

// An unparseable proxy URL used to leave transport.Proxy nil, which quietly sent the
// request from the server's own address instead of the proxy the caller asked for —
// and past the dialer guard, since the per-request transport carries none.
func TestHTTPHandler_InvalidProxyURLDoesNotFallBackToDirect(t *testing.T) {
	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Write([]byte("<html><body>direct</body></html>"))
	}))
	defer target.Close()

	// allowPrivateTargets, so a direct fetch of this loopback target would succeed.
	h := NewHTTPHandler(5*time.Second, "", true)
	_, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL:      target.URL,
		ProxyURL: "http://[::1",
	})

	if err == nil {
		t.Fatal("expected an error for an unparseable proxy URL")
	}
	if !strings.Contains(err.Error(), "invalid proxy URL") {
		t.Errorf("expected an invalid-proxy-URL error, got: %v", err)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("target was fetched %d times; the request bypassed the proxy", got)
	}
}
