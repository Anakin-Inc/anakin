// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "testing"

// PROXY_URLS has to enable the proxy pool with or without DATABASE_URL. The database
// only persists scores between restarts; gating the pool on it left the zero-config
// mode the README leads with sending every request direct, with no warning.
func TestNewProxyPoolWithoutDatabase(t *testing.T) {
	pool := newProxyPool(nil, []string{"http://proxy1:8080", "http://proxy2:8080"})
	if pool == nil {
		t.Fatal("newProxyPool returned nil without a database; PROXY_URLS would be silently ignored")
	}

	if got := pool.SelectProxy("example.com"); got != "http://proxy1:8080" && got != "http://proxy2:8080" {
		t.Errorf("SelectProxy returned %q, want one of the configured proxies", got)
	}
}

func TestNewProxyPoolWithoutProxies(t *testing.T) {
	if pool := newProxyPool(nil, nil); pool != nil {
		t.Error("newProxyPool returned a pool for an empty PROXY_URLS")
	}
	if pool := newProxyPool(nil, []string{}); pool != nil {
		t.Error("newProxyPool returned a pool for an empty PROXY_URLS")
	}
}

func TestStorageMode(t *testing.T) {
	if got := storageMode(nil); got != "memory" {
		t.Errorf("storageMode(nil) = %q, want %q", got, "memory")
	}
}
