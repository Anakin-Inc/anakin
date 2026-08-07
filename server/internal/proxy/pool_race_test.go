// SPDX-License-Identifier: AGPL-3.0-or-later

package proxy

import (
	"sync"
	"testing"
)

const raceHost = "example.com"

var raceProxies = []string{"http://a:8080", "http://b:8080", "http://c:8080"}

// The pool is driven concurrently in normal operation: WORKER_POOL_SIZE workers
// (5 by default) call SelectProxy and RecordSuccess/RecordFailure for the same
// host while GET /v1/proxy/scores can call Scores() at any moment.
//
// Before the fix, SelectProxy released the read lock and then handed live
// *Score pointers to the sampler, which read Alpha/Beta unsynchronised against
// the writes in RecordSuccess/RecordFailure. It also iterated the live inner
// map, which getOrCreateScore can insert into concurrently — a hard runtime
// throw, not merely a benign race.
//
// Fails under -race on the previous implementation.
func TestPool_ConcurrentSelectRecordAndScores(t *testing.T) {
	p := NewPool(nil, raceProxies)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				p.SelectProxy(raceHost)
				p.RecordSuccess(raceProxies[0], raceHost, 100)
				p.RecordFailure(raceProxies[1], raceHost, false)
				p.RecordFailure(raceProxies[2], raceHost, true)
				for _, scores := range p.Scores() {
					for _, sc := range scores {
						// Touch every field, including the multi-word time.Time,
						// the way the JSON marshaller in the scores handler does.
						_ = sc.ProxyURL
						_ = sc.Alpha
						_ = sc.Beta
						_ = sc.Score
						_ = sc.TotalRequests
						_ = sc.AvgLatencyMs
						_ = sc.LastUpdated
					}
				}
			}
		}(i)
	}
	wg.Wait()
}

// persistScores runs on a 60s ticker in production and reads every Score field
// to write it to Postgres. It previously collected live pointers under the read
// lock and then read them after releasing it, so a torn read could be persisted.
// Exercised here with a nil DB: the collection and copying still run, which is
// where the race was.
func TestPool_ConcurrentPersistAndRecord(t *testing.T) {
	p := NewPool(nil, raceProxies)
	p.RecordSuccess(raceProxies[0], raceHost, 100)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			p.persistScores()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			p.RecordSuccess(raceProxies[0], raceHost, i)
			p.RecordFailure(raceProxies[0], raceHost, false)
		}
	}()

	wg.Wait()
}

// Copying scores out for selection must not change which proxies are
// considered: a blocked proxy stays excluded, and an unknown proxy still gets
// the uniform Beta(1,1) prior.
func TestPool_SelectProxyStillHonoursBlockedAndPriors(t *testing.T) {
	p := NewPool(nil, []string{"http://a:8080", "http://b:8080"})

	p.RecordFailure("http://a:8080", raceHost, true) // blocks a for blockedTTL

	for i := 0; i < 100; i++ {
		if got := p.SelectProxy(raceHost); got == "http://a:8080" {
			t.Fatalf("SelectProxy returned blocked proxy on attempt %d", i)
		}
	}

	// b has no recorded score yet, so it is selected from the default prior
	// rather than being skipped for having no entry.
	if got := p.SelectProxy(raceHost); got != "http://b:8080" {
		t.Errorf("SelectProxy = %q, want the unscored-but-eligible proxy", got)
	}
}

// Scores() must hand back a snapshot: mutating the result cannot corrupt pool
// state, and later writes are not visible through an already-returned value.
func TestPool_ScoresReturnsSnapshot(t *testing.T) {
	p := NewPool(nil, raceProxies)
	p.RecordSuccess(raceProxies[0], raceHost, 100)

	snapshot := p.Scores()[raceHost]
	if len(snapshot) != 1 {
		t.Fatalf("got %d scores, want 1", len(snapshot))
	}
	alphaBefore := snapshot[0].Alpha

	// Mutating the returned value must not reach into the pool.
	snapshot[0].Alpha = 9999
	p.RecordSuccess(raceProxies[0], raceHost, 100)

	fresh := p.Scores()[raceHost][0]
	if fresh.Alpha != alphaBefore+1 {
		t.Errorf("Alpha = %d, want %d — the returned snapshot aliased pool state",
			fresh.Alpha, alphaBefore+1)
	}

	// And the snapshot must not observe the later write.
	if snapshot[0].Alpha != 9999 {
		t.Errorf("snapshot Alpha = %d, want 9999 — the snapshot is not independent", snapshot[0].Alpha)
	}
}
