package domain

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Cache provides fast in-memory domain config lookups with periodic DB refresh.
type Cache struct {
	repo    *Repository
	mu      sync.RWMutex
	configs map[string]*DomainConfig
	stop    chan struct{}
}

func NewCache(repo *Repository) *Cache {
	return &Cache{
		repo:    repo,
		configs: make(map[string]*DomainConfig),
		stop:    make(chan struct{}),
	}
}

// Start loads configs and begins background refresh every 60s.
func (c *Cache) Start(ctx context.Context) {
	c.refresh()
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.refresh()
			case <-c.stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (c *Cache) Stop() {
	close(c.stop)
}

func (c *Cache) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	configs, err := c.repo.GetAll(ctx)
	if err != nil {
		slog.Error("failed to refresh domain configs", "error", err)
		return
	}

	m := make(map[string]*DomainConfig, len(configs))
	for _, cfg := range configs {
		m[cfg.Domain] = cfg
	}

	c.mu.Lock()
	c.configs = m
	c.mu.Unlock()
	slog.Debug("domain configs refreshed", "count", len(configs))
}

// GetConfig returns the domain config for the given URL, or nil if none matches.
// Tries exact domain match first, then parent domains (if matchSubdomains is enabled).
func (c *Cache) GetConfig(rawURL string) *DomainConfig {
	host := ExtractHost(rawURL)
	if host == "" {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Exact match
	if cfg, ok := c.configs[host]; ok {
		return cfg
	}

	// Subdomain matching: for "www.shoes.amazon.com" try "shoes.amazon.com", "amazon.com"
	parts := strings.Split(host, ".")
	for i := 1; i < len(parts)-1; i++ {
		parent := strings.Join(parts[i:], ".")
		if cfg, ok := c.configs[parent]; ok && cfg.MatchSubdomains {
			return cfg
		}
	}

	return nil
}

// ExtractHost returns the lowercase hostname from a URL string.
func ExtractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
