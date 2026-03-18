package proxy

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

const (
	blockedTTL       = 5 * time.Minute
	severePenalty    = 10
	latencyEMA       = 0.2
	persistInterval  = 60 * time.Second
	scoreEvictionTTL = 24 * time.Hour
)

// Pool manages proxy selection using Thompson Sampling.
// Scores are kept in memory and periodically flushed to PostgreSQL.
type Pool struct {
	mu      sync.RWMutex
	proxies []string
	scores  map[string]map[string]*Score    // targetHost -> proxyURL -> score
	blocked map[string]map[string]time.Time // targetHost -> proxyURL -> expiry
	sampler *Sampler
	db      *sql.DB
	stop    chan struct{}
}

// NewPool creates a proxy pool with the given proxy URLs.
func NewPool(db *sql.DB, proxies []string) *Pool {
	return &Pool{
		proxies: proxies,
		scores:  make(map[string]map[string]*Score),
		blocked: make(map[string]map[string]time.Time),
		sampler: NewSampler(),
		db:      db,
		stop:    make(chan struct{}),
	}
}

// Start loads scores from DB and begins background persistence.
func (p *Pool) Start(ctx context.Context) {
	p.loadScores()
	go func() {
		ticker := time.NewTicker(persistInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.persistScores()
				p.cleanBlocked()
			case <-ctx.Done():
				p.persistScores()
				return
			case <-p.stop:
				p.persistScores()
				return
			}
		}
	}()
	slog.Info("proxy pool started", "proxies", len(p.proxies))
}

func (p *Pool) Stop() {
	close(p.stop)
}

// SelectProxy picks the best proxy for the target host using Thompson Sampling.
func (p *Pool) SelectProxy(targetHost string) string {
	if len(p.proxies) == 0 {
		return ""
	}

	p.mu.RLock()
	hostScores := p.scores[targetHost]
	hostBlocked := p.blocked[targetHost]
	p.mu.RUnlock()

	now := time.Now()
	candidates := make([]*Score, 0, len(p.proxies))

	for _, proxyURL := range p.proxies {
		// Skip blocked proxies
		if hostBlocked != nil {
			if expiry, ok := hostBlocked[proxyURL]; ok && now.Before(expiry) {
				continue
			}
		}

		if hostScores != nil {
			if sc, ok := hostScores[proxyURL]; ok {
				candidates = append(candidates, sc)
				continue
			}
		}

		// New proxy: use default priors (uniform belief)
		candidates = append(candidates, &Score{
			ProxyURL:   proxyURL,
			TargetHost: targetHost,
			Alpha:      1,
			Beta:       1,
			Score:      0.5,
		})
	}

	if len(candidates) == 0 {
		// All blocked — return a random proxy as fallback
		return p.proxies[int(time.Now().UnixNano())%len(p.proxies)]
	}

	best := p.sampler.Select(candidates)
	if best == nil {
		return p.proxies[0]
	}
	return best.ProxyURL
}

// RecordSuccess updates the proxy score after a successful scrape.
func (p *Pool) RecordSuccess(proxyURL, targetHost string, latencyMs int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sc := p.getOrCreateScore(proxyURL, targetHost)
	sc.Alpha++
	sc.TotalRequests++
	sc.Score = float64(sc.Alpha) / float64(sc.Alpha+sc.Beta)
	if latencyMs > 0 {
		sc.AvgLatencyMs = int(float64(sc.AvgLatencyMs)*(1-latencyEMA) + float64(latencyMs)*latencyEMA)
	}
	sc.LastUpdated = time.Now()
}

// RecordFailure updates the proxy score after a failed scrape.
// If isBlocked is true, a severe penalty is applied and the proxy is temporarily blocked.
func (p *Pool) RecordFailure(proxyURL, targetHost string, isBlocked bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	sc := p.getOrCreateScore(proxyURL, targetHost)
	if isBlocked {
		sc.Beta += severePenalty
		if p.blocked[targetHost] == nil {
			p.blocked[targetHost] = make(map[string]time.Time)
		}
		p.blocked[targetHost][proxyURL] = time.Now().Add(blockedTTL)
	} else {
		sc.Beta++
	}
	sc.TotalRequests++
	sc.Score = float64(sc.Alpha) / float64(sc.Alpha+sc.Beta)
	sc.LastUpdated = time.Now()
}

// Scores returns all current proxy scores grouped by target host.
func (p *Pool) Scores() map[string][]*Score {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string][]*Score)
	for host, scores := range p.scores {
		for _, sc := range scores {
			result[host] = append(result[host], sc)
		}
	}
	return result
}

func (p *Pool) getOrCreateScore(proxyURL, targetHost string) *Score {
	if p.scores[targetHost] == nil {
		p.scores[targetHost] = make(map[string]*Score)
	}
	sc, ok := p.scores[targetHost][proxyURL]
	if !ok {
		sc = &Score{
			ProxyURL:    proxyURL,
			TargetHost:  targetHost,
			Alpha:       1,
			Beta:        1,
			Score:       0.5,
			LastUpdated: time.Now(),
		}
		p.scores[targetHost][proxyURL] = sc
	}
	return sc
}

func (p *Pool) cleanBlocked() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for host, proxies := range p.blocked {
		for px, expiry := range proxies {
			if now.After(expiry) {
				delete(proxies, px)
			}
		}
		if len(proxies) == 0 {
			delete(p.blocked, host)
		}
	}
}

func (p *Pool) loadScores() {
	if p.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := p.db.QueryContext(ctx,
		`SELECT proxy_url, target_host, alpha, beta, score, total_requests, avg_latency_ms, last_updated
		 FROM proxy_scores`)
	if err != nil {
		slog.Warn("failed to load proxy scores", "error", err)
		return
	}
	defer rows.Close()

	count := 0
	p.mu.Lock()
	defer p.mu.Unlock()

	for rows.Next() {
		var sc Score
		if err := rows.Scan(&sc.ProxyURL, &sc.TargetHost, &sc.Alpha, &sc.Beta,
			&sc.Score, &sc.TotalRequests, &sc.AvgLatencyMs, &sc.LastUpdated); err != nil {
			continue
		}
		if p.scores[sc.TargetHost] == nil {
			p.scores[sc.TargetHost] = make(map[string]*Score)
		}
		p.scores[sc.TargetHost][sc.ProxyURL] = &sc
		count++
	}
	slog.Info("loaded proxy scores", "count", count)
}

func (p *Pool) persistScores() {
	if p.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p.mu.RLock()
	var allScores []*Score
	for _, hostScores := range p.scores {
		for _, sc := range hostScores {
			allScores = append(allScores, sc)
		}
	}
	p.mu.RUnlock()

	for _, sc := range allScores {
		_, err := p.db.ExecContext(ctx,
			`INSERT INTO proxy_scores (proxy_url, target_host, alpha, beta, score, total_requests, avg_latency_ms, last_updated)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (proxy_url, target_host) DO UPDATE SET
			    alpha=$3, beta=$4, score=$5, total_requests=$6, avg_latency_ms=$7, last_updated=$8`,
			sc.ProxyURL, sc.TargetHost, sc.Alpha, sc.Beta,
			sc.Score, sc.TotalRequests, sc.AvgLatencyMs, sc.LastUpdated,
		)
		if err != nil {
			slog.Warn("failed to persist proxy score", "proxy", sc.ProxyURL, "host", sc.TargetHost, "error", err)
		}
	}
	slog.Debug("persisted proxy scores", "count", len(allScores))

	// Evict stale scores
	now := time.Now()
	p.mu.Lock()
	for host, proxies := range p.scores {
		for proxyURL, score := range proxies {
			if now.Sub(score.LastUpdated) > scoreEvictionTTL {
				delete(proxies, proxyURL)
			}
		}
		if len(proxies) == 0 {
			delete(p.scores, host)
		}
	}
	p.mu.Unlock()
}
