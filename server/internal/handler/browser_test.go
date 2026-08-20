// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"net"
	"testing"
	"time"
)

func TestWSDialAddr(t *testing.T) {
	tests := []struct {
		name    string
		wsURL   string
		want    string
		wantErr bool
	}{
		{"explicit port", "ws://localhost:9222/camoufox", "localhost:9222", false},
		{"ws defaults to 80", "ws://browser/camoufox", "browser:80", false},
		{"wss defaults to 443", "wss://browser.example.com/camoufox", "browser.example.com:443", false},
		{"ipv6 literal", "ws://[::1]:9222/camoufox", "[::1]:9222", false},
		{"no host", "ws:///camoufox", "", true},
		{"unparseable", "ws://a b c", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := wsDialAddr(tt.wsURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("wsDialAddr(%q) = %q, want error", tt.wsURL, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("wsDialAddr(%q) unexpected error: %v", tt.wsURL, err)
			}
			if got != tt.want {
				t.Errorf("wsDialAddr(%q) = %q, want %q", tt.wsURL, got, tt.want)
			}
		})
	}
}

// A handler pointing at a dead endpoint must report itself unhealthy so the
// chain skips it, rather than failing inside Scrape and masking the real error.
func TestBrowserHandler_UnreachableEndpointIsUnhealthy(t *testing.T) {
	// Port 1 on loopback: reserved, nothing listens there.
	h := NewBrowserHandler("ws://127.0.0.1:1/camoufox", time.Second, 0)
	if h.IsHealthy() {
		t.Error("IsHealthy() = true for an endpoint with nothing listening")
	}
}

func TestBrowserHandler_ReachableEndpointProbesOK(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	h := NewBrowserHandler("ws://"+ln.Addr().String()+"/camoufox", time.Second, 0)
	if !h.endpointReachable() {
		t.Error("endpointReachable() = false while a listener is accepting")
	}
}

// The probe is cached so a chain fallback does not dial on every request.
func TestBrowserHandler_ProbeIsCached(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()

	h := NewBrowserHandler("ws://"+addr+"/camoufox", time.Second, 0)
	if !h.endpointReachable() {
		t.Fatal("endpointReachable() = false while a listener is accepting")
	}

	// Endpoint goes away, but the cached result is still within probeTTL.
	ln.Close()
	if !h.endpointReachable() {
		t.Error("endpointReachable() re-dialled instead of using the cached result")
	}

	// Expire the cache: the next call must observe reality.
	h.probeMu.Lock()
	h.probedAt = time.Now().Add(-probeTTL - time.Second)
	h.probeMu.Unlock()
	if h.endpointReachable() {
		t.Error("endpointReachable() = true after the cache expired and the listener closed")
	}
}
