// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// TestBrowserHandler_SSRFGuard drives a real browser at an "attacker" page that tries
// to reach an "internal" canary server through every vector a page controls. The
// browser lives in another process, so the Go dialer guard can't see its traffic;
// this covers the browser handler's request guard together with the egress proxy
// (WebSockets and redirect hops are only stopped by the latter).
//
// Needs a browser on an internal network whose only route out is smokescreen,
// allowing the attacker server and nothing else on this host. The test reaches the
// browser through smokescreen's network, since an internal-only container can't
// publish ports:
//
//	docker network create --internal ssrf-egress
//	docker run -d --name ssrf-smokescreen --network bridge <smokescreen image> \
//	    --listen-ip 0.0.0.0 --listen-port 4750 --allow-address 172.17.0.1:18081
//	docker network connect ssrf-egress ssrf-smokescreen
//	docker run -d --name ssrf-browser --network ssrf-egress \
//	    -e PROXY_SERVER=http://ssrf-smokescreen:4750 <browser-service image>
//	BROWSER_WS_URL=ws://<ssrf-browser IP on ssrf-egress>:9222/camoufox SSRF_TEST_HOST=172.17.0.1 \
//	    SSRF_TEST_ATTACKER_PORT=18081 go test -run SSRF ./internal/handler/
//
// SSRF_TEST_HOST is the address the browser uses to reach this test process: the
// docker bridge gateway when the browser runs in a container, loopback otherwise.
func TestBrowserHandler_SSRFGuard(t *testing.T) {
	wsURL := os.Getenv("BROWSER_WS_URL")
	if wsURL == "" {
		t.Skip("BROWSER_WS_URL not set; skipping browser integration test")
	}
	host := os.Getenv("SSRF_TEST_HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	secret := fmt.Sprintf("CANARY-SECRET-%d", time.Now().UnixNano())
	var hits atomic.Int64
	canary := listenAll(t, "0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(secret))
	}))
	canaryPort := portOf(t, canary)
	canaryURL := fmt.Sprintf("http://%s:%s", host, canaryPort)

	attacker := listenAll(t, os.Getenv("SSRF_TEST_ATTACKER_PORT"), attackerMux(canaryURL, fmt.Sprintf("ws://%s:%s/ws", host, canaryPort)))
	attackerURL := fmt.Sprintf("http://%s:%s", host, portOf(t, attacker))

	h := NewBrowserHandler(wsURL, 20*time.Second, 0, false)
	defer h.Stop()
	// Every server here is on the same host, so mark only the canary as internal.
	h.guard = func(_ context.Context, u *url.URL) error {
		if u.Port() == canaryPort {
			return errors.New("canary is internal")
		}
		return nil
	}

	scrape := func(t *testing.T, target string) (*models.ScrapeResult, error) {
		t.Helper()
		hits.Store(0)
		res, err := h.Scrape(context.Background(), &models.HandlerRequest{URL: target, UseBrowser: true})
		// Sub-resources may still be in flight when the page settles.
		time.Sleep(500 * time.Millisecond)
		return res, err
	}

	t.Run("benign page still scrapes", func(t *testing.T) {
		res, err := scrape(t, attackerURL+"/benign")
		if err != nil {
			t.Fatalf("benign scrape failed: %v", err)
		}
		if !strings.Contains(res.HTML, "BENIGN-OK") {
			t.Errorf("benign page content missing: %q", res.HTML)
		}
	})

	vectors := []struct{ name, path string }{
		{"direct navigation", ""}, // target is the canary itself
		{"302 redirect", "/redirect"},
		{"meta refresh", "/meta"},
		{"js location", "/location"},
		{"fetch no-cors", "/fetch"},
		{"img and script src", "/subresource"},
		{"websocket", "/websocket"},
		{"img redirect", "/img-redirect"},
	}
	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			target := attackerURL + v.path
			if v.path == "" {
				target = canaryURL + "/"
			}
			res, err := scrape(t, target)
			if n := hits.Load(); n != 0 {
				t.Errorf("browser reached the internal canary %d time(s)", n)
			}
			if err == nil && strings.Contains(res.HTML, secret) {
				t.Errorf("internal response body was returned to the caller")
			}
		})
	}
}

func attackerMux(canaryURL, canaryWS string) http.Handler {
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><head><title>t</title></head><body>%s</body></html>", body)
		}
	}
	mux := http.NewServeMux()
	mux.Handle("/benign", page("BENIGN-OK"))
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, canaryURL+"/", http.StatusFound)
	})
	mux.Handle("/meta", page(fmt.Sprintf(`<meta http-equiv="refresh" content="0;url=%s/">`, canaryURL)))
	mux.Handle("/location", page(fmt.Sprintf(`<script>window.addEventListener("load", function () { location.href = %q; });</script>`, canaryURL+"/")))
	mux.Handle("/fetch", page(fmt.Sprintf(`<script>fetch(%q, {mode: "no-cors"}).catch(function () {});</script>`, canaryURL+"/f")))
	mux.Handle("/subresource", page(fmt.Sprintf(`<img src="%s/i.png"><script src="%s/s.js"></script>`, canaryURL, canaryURL)))
	mux.Handle("/websocket", page(fmt.Sprintf(`<script>try { new WebSocket(%q); } catch (e) {}</script>`, canaryWS)))
	mux.Handle("/img-redirect", page(`<img src="/redirect">`))
	return mux
}

// listenAll serves h on every interface so a containerised browser can reach it.
// An empty port picks a free one.
func listenAll(t *testing.T, port string, h http.Handler) *httptest.Server {
	t.Helper()
	if port == "" {
		port = "0"
	}
	ln, err := net.Listen("tcp", "0.0.0.0:"+port)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func portOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split %q: %v", srv.Listener.Addr(), err)
	}
	return port
}
