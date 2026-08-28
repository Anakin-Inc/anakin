package proxy

import "testing"

func TestRecordSuccess_SeedsLatencyOnFirstSample(t *testing.T) {
	p := NewPool(nil, []string{"http://proxy1"})

	p.RecordSuccess("http://proxy1", "example.com", 1000)

	sc := p.getOrCreateScore("http://proxy1", "example.com")
	if sc.AvgLatencyMs != 1000 {
		t.Errorf("first sample: AvgLatencyMs = %d, want 1000 (seeded, not EMA-weighted)", sc.AvgLatencyMs)
	}
}

func TestRecordSuccess_AppliesEMAAfterSeeding(t *testing.T) {
	p := NewPool(nil, []string{"http://proxy1"})

	p.RecordSuccess("http://proxy1", "example.com", 1000) // seeds to 1000
	p.RecordSuccess("http://proxy1", "example.com", 2000) // EMA: 1000*0.8 + 2000*0.2 = 1200

	sc := p.getOrCreateScore("http://proxy1", "example.com")
	if sc.AvgLatencyMs != 1200 {
		t.Errorf("second sample: AvgLatencyMs = %d, want 1200", sc.AvgLatencyMs)
	}
}

func TestRecordSuccess_ZeroLatencyDoesNotSeed(t *testing.T) {
	p := NewPool(nil, []string{"http://proxy1"})

	p.RecordSuccess("http://proxy1", "example.com", 0)

	sc := p.getOrCreateScore("http://proxy1", "example.com")
	if sc.AvgLatencyMs != 0 {
		t.Errorf("zero latency: AvgLatencyMs = %d, want 0 (ignored)", sc.AvgLatencyMs)
	}
	// Alpha still increments even when latency is unknown.
	if sc.Alpha != 2 {
		t.Errorf("Alpha = %d, want 2 (started at 1, +1 success)", sc.Alpha)
	}
}

func TestSelectProxy_ConcurrentAccess(t *testing.T) {
	proxies := []string{"http://proxy1:8080", "http://proxy2:8080", "http://proxy3:8080"}
	p := NewPool(nil, proxies)

	done := make(chan bool)
	hosts := []string{"example.com", "api.github.com", "google.com"}

	// Spawn concurrent readers
	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				host := hosts[j%len(hosts)]
				_ = p.SelectProxy(host)
			}
			done <- true
		}(i)
	}

	// Spawn concurrent writers
	for i := 0; i < 5; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				host := hosts[j%len(hosts)]
				px := proxies[j%len(proxies)]
				if j%2 == 0 {
					p.RecordSuccess(px, host, 500)
				} else {
					p.RecordFailure(px, host, j%4 == 1)
				}
			}
			done <- true
		}(i)
	}

	for i := 0; i < 15; i++ {
		<-done
	}
}
