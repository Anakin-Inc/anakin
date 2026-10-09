// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// httptest servers listen on loopback, so they double as the "internal address" a
// scrape must not reach — this asserts the dialer guard is actually wired up, which
// is what covers redirects and DNS rebinding past the boundary check.
func TestHTTPHandler_DialGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<html><body>internal</body></html>"))
	}))
	defer srv.Close()

	req := &models.HandlerRequest{URL: srv.URL}

	t.Run("blocked by default", func(t *testing.T) {
		h := NewHTTPHandler(5*time.Second, "", false)
		_, err := h.Scrape(context.Background(), req)
		if err == nil {
			t.Fatal("expected the loopback fetch to be refused")
		}
		if !strings.Contains(err.Error(), "blocked connection") {
			t.Errorf("expected a blocked-connection error, got: %v", err)
		}
	})

	t.Run("allowed with ALLOW_PRIVATE_TARGETS", func(t *testing.T) {
		h := NewHTTPHandler(5*time.Second, "", true)
		res, err := h.Scrape(context.Background(), req)
		if err != nil {
			t.Fatalf("expected the fetch to succeed, got: %v", err)
		}
		if !strings.Contains(res.HTML, "internal") {
			t.Errorf("unexpected body: %q", res.HTML)
		}
	})
}

// With a proxy the dial target is the proxy, so the dialer guard is off. The
// redirect hops must still be refused: an origin that passed the boundary check
// 302s to an internal address.
func TestHTTPHandler_ProxyRedirectGuard(t *testing.T) {
	var hits atomic.Int64
	canary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Write([]byte("internal"))
	}))
	defer canary.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, canary.URL+"/", http.StatusFound)
	}))
	defer origin.Close()

	// Minimal forward proxy: replays the absolute-URI request upstream.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RequestURI = ""
		resp, err := http.DefaultTransport.RoundTrip(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	cases := []struct {
		name string
		h    *HTTPHandler
		req  *models.HandlerRequest
	}{
		{"handler proxy", NewHTTPHandler(5*time.Second, proxy.URL, false), &models.HandlerRequest{URL: origin.URL}},
		{"per-request proxy", NewHTTPHandler(5*time.Second, "", false), &models.HandlerRequest{URL: origin.URL, ProxyURL: proxy.URL}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits.Store(0)
			_, err := tc.h.Scrape(context.Background(), tc.req)
			if err == nil {
				t.Error("expected the redirect to the internal address to be refused")
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("internal canary was reached %d time(s) through the proxy", n)
			}
		})
	}
}
