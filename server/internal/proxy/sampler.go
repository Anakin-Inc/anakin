// SPDX-License-Identifier: AGPL-3.0-or-later

package proxy

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// Score represents a proxy's performance for a specific target host.
type Score struct {
	ProxyURL      string    `json:"proxyUrl"`
	TargetHost    string    `json:"targetHost"`
	Alpha         int       `json:"alpha"`
	Beta          int       `json:"beta"`
	Score         float64   `json:"score"`
	TotalRequests int       `json:"totalRequests"`
	AvgLatencyMs  int       `json:"avgLatencyMs"`
	LastUpdated   time.Time `json:"lastUpdated"`
}

// Sampler implements Thompson Sampling for proxy selection.
type Sampler struct {
	mu  sync.Mutex
	rng *rand.Rand
}

func NewSampler() *Sampler {
	return &Sampler{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Select picks the best proxy using Thompson Sampling from Beta distributions.
// Each proxy's score is sampled from Beta(alpha, beta), and the proxy with the
// highest sample is selected. This naturally balances exploration vs exploitation.
func (s *Sampler) Select(scores []*Score) *Score {
	if len(scores) == 0 {
		return nil
	}
	if len(scores) == 1 {
		return scores[0]
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var best *Score
	bestSample := -1.0

	for _, sc := range scores {
		sample := betaSample(s.rng, sc.Alpha, sc.Beta)
		if sample > bestSample {
			bestSample = sample
			best = sc
		}
	}
	return best
}

// betaSample draws from Beta(alpha, beta) using Gamma sampling.
func betaSample(rng *rand.Rand, alpha, beta int) float64 {
	a := float64(alpha)
	b := float64(beta)
	if a <= 0 {
		a = 1
	}
	if b <= 0 {
		b = 1
	}
	x := gammaSample(rng, a)
	y := gammaSample(rng, b)
	if x+y == 0 {
		return 0.5
	}
	return x / (x + y)
}

// gammaSample draws from Gamma(shape, 1) using the Marsaglia-Tsang method.
func gammaSample(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		return gammaSample(rng, shape+1) * math.Pow(rng.Float64(), 1.0/shape)
	}

	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9.0*d)

	for {
		var x, v float64
		for {
			x = rng.NormFloat64()
			v = 1.0 + c*x
			if v > 0 {
				break
			}
		}
		v = v * v * v
		u := rng.Float64()

		if u < 1.0-0.0331*(x*x)*(x*x) {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1.0-v+math.Log(v)) {
			return d * v
		}
	}
}
