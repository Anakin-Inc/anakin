// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"log/slog"
	"sync"
	"time"
)

// maxTrackedNonces caps the replay cache so a burst of paid traffic cannot grow
// it without bound. When the cap is reached the cache fails closed: new payments
// are refused rather than silently accepted without replay protection.
const maxTrackedNonces = 100_000

// nonceCache tracks spent payment-authorization nonces so a payment can be used
// for exactly one scrape.
//
// The facilitator will keep reporting a captured authorization as valid until it
// expires, so validity alone does not stop a replay — something has to remember
// that this particular nonce was already spent. Entries are kept until the
// longest window an authorization could still be replayed in has passed, then
// pruned, which bounds the cache to the number of payments inside that window.
//
// It is safe for concurrent use.
type nonceCache struct {
	mu        sync.Mutex
	seen      map[string]time.Time // nonce -> time it may be forgotten
	ttl       time.Duration
	now       func() time.Time // overridable in tests
	lastPrune time.Time
}

func newNonceCache(ttl time.Duration) *nonceCache {
	if ttl <= 0 {
		ttl = defaultXNOMaxPaymentAge
	}
	return &nonceCache{
		seen: make(map[string]time.Time),
		ttl:  ttl,
		now:  time.Now,
	}
}

// claim records nonce as spent and reports whether it was previously unused.
//
// An empty nonce means the payment scheme carries no nonce to deduplicate on.
// That is accepted — the facilitator still has to verify the payload — but the
// caller gets no replay protection from this cache, so the limitation is logged.
func (c *nonceCache) claim(nonce string) bool {
	if nonce == "" {
		slog.Warn("payment carries no authorization nonce — this payment cannot be replay-protected by the server")
		return true
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.pruneLocked(now)

	if _, spent := c.seen[nonce]; spent {
		return false
	}
	if len(c.seen) >= maxTrackedNonces {
		slog.Error("payment replay cache is full — refusing new payments",
			"limit", maxTrackedNonces,
			"ttl", c.ttl)
		return false
	}

	c.seen[nonce] = now.Add(c.ttl)
	return true
}

// release returns a nonce to the pool. It is used when a claimed payment could
// not be checked because the facilitator was unreachable: the nonce was never
// accepted by anyone, so holding it would force the caller to re-sign for a
// reason that has nothing to do with their payment.
func (c *nonceCache) release(nonce string) {
	if nonce == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.seen, nonce)
}

// pruneLocked drops entries that can no longer be replayed.
//
// It runs at most once per TTL rather than on every claim. Sweeping the whole map
// per claim is O(live) each time, which turns filling the cache into O(n²) and
// stalls request handling well before the cap is reached. Pruning once per window
// costs the same total work amortized and lets an entry outlive its TTL by up to
// one window — the conservative direction, since it only ever refuses a payment
// slightly longer than necessary.
func (c *nonceCache) pruneLocked(now time.Time) {
	if !c.lastPrune.IsZero() && now.Sub(c.lastPrune) < c.ttl {
		return
	}
	c.lastPrune = now
	for nonce, expiry := range c.seen {
		if !expiry.After(now) {
			delete(c.seen, nonce)
		}
	}
}

// len reports how many nonces are currently tracked. Used by tests.
func (c *nonceCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}
