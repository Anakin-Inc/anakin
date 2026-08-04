// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import "testing"

// newTestCache builds a Cache around a fixed config set, bypassing the DB
// refresh path but reusing the same indexing that refresh() applies.
func newTestCache(configs ...*DomainConfig) *Cache {
	m, _ := activeConfigs(configs)
	return &Cache{configs: m}
}

func TestCache_GetConfig_Matching(t *testing.T) {
	tests := []struct {
		name    string
		configs []*DomainConfig
		url     string
		want    string // matched domain, "" for no match
	}{
		{
			name:    "exact match",
			configs: []*DomainConfig{{Domain: "example.com", IsEnabled: true}},
			url:     "https://example.com/page",
			want:    "example.com",
		},
		{
			name:    "subdomain matches parent when matchSubdomains is set",
			configs: []*DomainConfig{{Domain: "example.com", IsEnabled: true, MatchSubdomains: true}},
			url:     "https://www.example.com/page",
			want:    "example.com",
		},
		{
			name:    "subdomain does not match parent without matchSubdomains",
			configs: []*DomainConfig{{Domain: "example.com", IsEnabled: true}},
			url:     "https://www.example.com/page",
			want:    "",
		},
		{
			name: "deep subdomain walks up to the nearest matching ancestor",
			configs: []*DomainConfig{
				{Domain: "example.com", IsEnabled: true, MatchSubdomains: true},
				{Domain: "jobs.example.com", IsEnabled: true, MatchSubdomains: true},
			},
			url:  "https://www.jobs.example.com/posting/1",
			want: "jobs.example.com",
		},
		{
			name:    "exact match wins over ancestor at equal priority",
			configs: []*DomainConfig{{Domain: "example.com", IsEnabled: true, MatchSubdomains: true}, {Domain: "www.example.com", IsEnabled: true}},
			url:     "https://www.example.com/page",
			want:    "www.example.com",
		},
		{
			name:    "no configs",
			configs: nil,
			url:     "https://example.com",
			want:    "",
		},
		{
			name:    "unparseable url",
			configs: []*DomainConfig{{Domain: "example.com", IsEnabled: true}},
			url:     "::not a url::",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newTestCache(tt.configs...).GetConfig(tt.url)
			assertMatched(t, got, tt.want)
		})
	}
}

// docs/domain-configs.md promises "if multiple configs could match, higher
// priority wins". Priority was loaded from the DB and then discarded, so the
// nearest ancestor always won regardless of how it was set.
func TestCache_GetConfig_Priority(t *testing.T) {
	tests := []struct {
		name    string
		configs []*DomainConfig
		url     string
		want    string
	}{
		{
			name: "higher-priority ancestor beats nearer ancestor",
			configs: []*DomainConfig{
				{Domain: "jobs.example.com", IsEnabled: true, MatchSubdomains: true, Priority: 0},
				{Domain: "example.com", IsEnabled: true, MatchSubdomains: true, Priority: 10},
			},
			url:  "https://www.jobs.example.com/posting/1",
			want: "example.com",
		},
		{
			name: "nearer ancestor wins at equal priority",
			configs: []*DomainConfig{
				{Domain: "jobs.example.com", IsEnabled: true, MatchSubdomains: true, Priority: 5},
				{Domain: "example.com", IsEnabled: true, MatchSubdomains: true, Priority: 5},
			},
			url:  "https://www.jobs.example.com/posting/1",
			want: "jobs.example.com",
		},
		{
			name: "higher-priority ancestor beats the exact host",
			configs: []*DomainConfig{
				{Domain: "www.example.com", IsEnabled: true, Priority: 1},
				{Domain: "example.com", IsEnabled: true, MatchSubdomains: true, Priority: 99},
			},
			url:  "https://www.example.com/page",
			want: "example.com",
		},
		{
			name: "exact host wins at equal priority",
			configs: []*DomainConfig{
				{Domain: "www.example.com", IsEnabled: true, Priority: 7},
				{Domain: "example.com", IsEnabled: true, MatchSubdomains: true, Priority: 7},
			},
			url:  "https://www.example.com/page",
			want: "www.example.com",
		},
		{
			name: "a higher-priority ancestor without matchSubdomains is not a candidate",
			configs: []*DomainConfig{
				{Domain: "jobs.example.com", IsEnabled: true, MatchSubdomains: true, Priority: 0},
				{Domain: "example.com", IsEnabled: true, MatchSubdomains: false, Priority: 99},
			},
			url:  "https://www.jobs.example.com/posting/1",
			want: "jobs.example.com",
		},
		{
			name: "a higher-priority ancestor that is disabled is not a candidate",
			configs: []*DomainConfig{
				{Domain: "jobs.example.com", IsEnabled: true, MatchSubdomains: true, Priority: 0},
				{Domain: "example.com", IsEnabled: false, MatchSubdomains: true, Priority: 99},
			},
			url:  "https://www.jobs.example.com/posting/1",
			want: "jobs.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMatched(t, newTestCache(tt.configs...).GetConfig(tt.url), tt.want)
		})
	}
}

// A disabled config must be completely inert. It previously still blocked the
// domain and still imposed content validation, because those branches in the
// processor did not consult IsEnabled.
func TestCache_GetConfig_IgnoresDisabledConfigs(t *testing.T) {
	c := newTestCache(
		&DomainConfig{Domain: "example.com", IsEnabled: false, Blocked: true, BlockedReason: "parked"},
	)

	if got := c.GetConfig("https://example.com/page"); got != nil {
		t.Errorf("GetConfig returned %+v for a disabled config; want nil", got)
	}
}

// A disabled config on a subdomain must not shadow an enabled parent.
func TestCache_GetConfig_DisabledConfigDoesNotShadowParent(t *testing.T) {
	c := newTestCache(
		&DomainConfig{Domain: "www.example.com", IsEnabled: false},
		&DomainConfig{Domain: "example.com", IsEnabled: true, MatchSubdomains: true},
	)

	assertMatched(t, c.GetConfig("https://www.example.com/page"), "example.com")
}

func TestExtractHost(t *testing.T) {
	tests := []struct{ url, want string }{
		{"https://example.com/page", "example.com"},
		{"https://WWW.Example.COM/page", "www.example.com"},
		{"http://example.com:8080/page", "example.com"},
		{"not a url", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := ExtractHost(tt.url); got != tt.want {
			t.Errorf("ExtractHost(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func assertMatched(t *testing.T, got *DomainConfig, wantDomain string) {
	t.Helper()
	if wantDomain == "" {
		if got != nil {
			t.Errorf("GetConfig matched %q, want no match", got.Domain)
		}
		return
	}
	if got == nil {
		t.Fatalf("GetConfig returned nil, want a match on %q", wantDomain)
	}
	if got.Domain != wantDomain {
		t.Errorf("GetConfig matched %q, want %q", got.Domain, wantDomain)
	}
}
