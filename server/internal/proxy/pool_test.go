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
