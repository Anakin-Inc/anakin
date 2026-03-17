package proxy

import (
	"math/rand"
	"testing"
)

func TestBetaSample_ReturnsBetweenZeroAndOne(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 1000; i++ {
		v := betaSample(rng, 5, 5)
		if v < 0 || v > 1 {
			t.Fatalf("betaSample(5,5) = %f, want in [0,1]", v)
		}
	}
}

func TestBetaSample_HighAlphaLowBeta_TendsTowardOne(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	sum := 0.0
	n := 5000
	for i := 0; i < n; i++ {
		sum += betaSample(rng, 100, 2)
	}
	mean := sum / float64(n)
	// Beta(100,2) has mean = 100/102 ~ 0.98
	if mean < 0.90 {
		t.Errorf("mean of Beta(100,2) samples = %f, expected > 0.90", mean)
	}
}

func TestBetaSample_LowAlphaHighBeta_TendsTowardZero(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	sum := 0.0
	n := 5000
	for i := 0; i < n; i++ {
		sum += betaSample(rng, 2, 100)
	}
	mean := sum / float64(n)
	// Beta(2,100) has mean = 2/102 ~ 0.02
	if mean > 0.10 {
		t.Errorf("mean of Beta(2,100) samples = %f, expected < 0.10", mean)
	}
}

func TestBetaSample_Uniform_AlphaOneBetaOne(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	sum := 0.0
	n := 5000
	for i := 0; i < n; i++ {
		sum += betaSample(rng, 1, 1)
	}
	mean := sum / float64(n)
	// Beta(1,1) is uniform on [0,1], mean = 0.5
	if mean < 0.40 || mean > 0.60 {
		t.Errorf("mean of Beta(1,1) samples = %f, expected near 0.5", mean)
	}
}

func TestBetaSample_ZeroAlpha_HandledGracefully(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	// alpha=0 should be clamped to 1 internally
	for i := 0; i < 100; i++ {
		v := betaSample(rng, 0, 5)
		if v < 0 || v > 1 {
			t.Fatalf("betaSample(0,5) = %f, want in [0,1]", v)
		}
	}
}

func TestBetaSample_ZeroBeta_HandledGracefully(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		v := betaSample(rng, 5, 0)
		if v < 0 || v > 1 {
			t.Fatalf("betaSample(5,0) = %f, want in [0,1]", v)
		}
	}
}

func TestSampler_SelectReturnsNilForEmpty(t *testing.T) {
	s := NewSampler()
	got := s.Select(nil)
	if got != nil {
		t.Errorf("Select(nil) = %v, want nil", got)
	}
}

func TestSampler_SelectReturnsSingleScore(t *testing.T) {
	s := NewSampler()
	score := &Score{ProxyURL: "http://proxy1", Alpha: 5, Beta: 5}
	got := s.Select([]*Score{score})
	if got != score {
		t.Errorf("Select with single score did not return that score")
	}
}

func TestSampler_SelectPrefersBetterProxy(t *testing.T) {
	s := NewSampler()
	good := &Score{ProxyURL: "http://good", Alpha: 200, Beta: 2}
	bad := &Score{ProxyURL: "http://bad", Alpha: 2, Beta: 200}

	goodCount := 0
	trials := 500
	for i := 0; i < trials; i++ {
		picked := s.Select([]*Score{good, bad})
		if picked == good {
			goodCount++
		}
	}

	// The good proxy should be selected the vast majority of the time
	if goodCount < trials*80/100 {
		t.Errorf("good proxy selected %d/%d times, expected > 80%%", goodCount, trials)
	}
}
