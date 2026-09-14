// SPDX-License-Identifier: AGPL-3.0-or-later

package proxy

import (
	"context"
	"testing"
)

// The pool is documented as usable without PostgreSQL — scores simply start fresh on
// every boot. Its full lifecycle therefore has to work with a nil *sql.DB.
func TestPoolLifecycleWithoutDatabase(t *testing.T) {
	proxies := []string{"http://proxy1:8080", "http://proxy2:8080"}
	p := NewPool(nil, proxies)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Start(ctx) // loads scores; must not touch a nil DB

	selected := p.SelectProxy("example.com")
	if selected != proxies[0] && selected != proxies[1] {
		t.Fatalf("SelectProxy returned %q, want one of %v", selected, proxies)
	}

	p.RecordSuccess(proxies[0], "example.com", 250)
	p.RecordFailure(proxies[1], "example.com", true)

	scores := p.Scores()["example.com"]
	if len(scores) != 2 {
		t.Fatalf("got %d scores for example.com, want 2", len(scores))
	}

	for _, sc := range scores {
		switch sc.ProxyURL {
		case proxies[0]:
			if sc.Alpha != 2 {
				t.Errorf("alpha for the successful proxy = %d, want 2", sc.Alpha)
			}
			if sc.AvgLatencyMs != 250 {
				t.Errorf("avg latency = %d, want 250", sc.AvgLatencyMs)
			}
		case proxies[1]:
			if sc.Beta != 1+severePenalty {
				t.Errorf("beta for the blocked proxy = %d, want %d", sc.Beta, 1+severePenalty)
			}
		}
	}

	// A proxy blocked for this host is skipped, so learning still works with no DB.
	if got := p.SelectProxy("example.com"); got != proxies[0] {
		t.Errorf("SelectProxy returned %q, want the unblocked %q", got, proxies[0])
	}

	p.Stop() // flushes scores; must not touch a nil DB either
}

func TestPoolWithoutProxiesSelectsNothing(t *testing.T) {
	if got := NewPool(nil, nil).SelectProxy("example.com"); got != "" {
		t.Errorf("SelectProxy returned %q with no proxies configured, want an empty string", got)
	}
}
