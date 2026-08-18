// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
