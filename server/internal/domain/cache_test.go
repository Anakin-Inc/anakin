// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import "testing"

func TestCacheSetConfigTakesEffectImmediately(t *testing.T) {
	cache := NewCache(nil)
	cache.SetConfig(&DomainConfig{
		Domain:  "example.com",
		Blocked: true,
	})

	got := cache.GetConfig("https://example.com/page")
	if got == nil || !got.Blocked {
		t.Fatalf("GetConfig() = %#v, want blocked example.com config", got)
	}
}

func TestCacheDeleteConfigTakesEffectImmediately(t *testing.T) {
	cache := NewCache(nil)
	cache.SetConfig(&DomainConfig{Domain: "example.com"})
	cache.DeleteConfig("example.com")

	if got := cache.GetConfig("https://example.com/page"); got != nil {
		t.Fatalf("GetConfig() = %#v after delete, want nil", got)
	}
}
