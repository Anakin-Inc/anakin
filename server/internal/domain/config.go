package domain

import "time"

// DomainConfig holds per-domain scraping configuration.
type DomainConfig struct {
	ID               int               `json:"id"`
	Domain           string            `json:"domain"`
	IsEnabled        bool              `json:"isEnabled"`
	MatchSubdomains  bool              `json:"matchSubdomains"`
	Priority         int               `json:"priority"`
	HandlerChain     []string          `json:"handlerChain"`
	RequestTimeoutMs int               `json:"requestTimeoutMs"`
	MaxRetries       int               `json:"maxRetries"`
	MinContentLength int               `json:"minContentLength"`
	FailurePatterns  []string          `json:"failurePatterns"`
	RequiredPatterns []string          `json:"requiredPatterns"`
	CustomHeaders    map[string]string `json:"customHeaders"`
	CustomUserAgent  string            `json:"customUserAgent,omitempty"`
	ProxyURL         string            `json:"proxyUrl,omitempty"`
	Blocked          bool              `json:"blocked"`
	BlockedReason    string            `json:"blockedReason,omitempty"`
	Notes            string            `json:"notes,omitempty"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}
