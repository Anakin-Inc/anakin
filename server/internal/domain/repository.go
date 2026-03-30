// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// Repository handles domain config persistence.
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetAll(ctx context.Context) ([]*DomainConfig, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, domain, is_enabled, match_subdomains, priority,
		        handler_chain, request_timeout_ms, max_retries,
		        min_content_length, failure_patterns, required_patterns,
		        custom_headers, custom_user_agent, proxy_url,
		        blocked, blocked_reason, notes, created_at, updated_at
		 FROM domain_configs ORDER BY priority DESC, domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*DomainConfig
	for rows.Next() {
		cfg, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

func (r *Repository) GetByDomain(ctx context.Context, domain string) (*DomainConfig, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, domain, is_enabled, match_subdomains, priority,
		        handler_chain, request_timeout_ms, max_retries,
		        min_content_length, failure_patterns, required_patterns,
		        custom_headers, custom_user_agent, proxy_url,
		        blocked, blocked_reason, notes, created_at, updated_at
		 FROM domain_configs WHERE domain = $1`, domain)
	return scanConfigRow(row)
}

func (r *Repository) Create(ctx context.Context, cfg *DomainConfig) error {
	now := time.Now().UTC()
	cfg.CreatedAt = now
	cfg.UpdatedAt = now
	return r.db.QueryRowContext(ctx,
		`INSERT INTO domain_configs (domain, is_enabled, match_subdomains, priority,
		    handler_chain, request_timeout_ms, max_retries,
		    min_content_length, failure_patterns, required_patterns,
		    custom_headers, custom_user_agent, proxy_url,
		    blocked, blocked_reason, notes, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		 RETURNING id`,
		cfg.Domain, cfg.IsEnabled, cfg.MatchSubdomains, cfg.Priority,
		joinStrings(cfg.HandlerChain), cfg.RequestTimeoutMs, cfg.MaxRetries,
		cfg.MinContentLength, joinStrings(cfg.FailurePatterns), joinStrings(cfg.RequiredPatterns),
		marshalHeaders(cfg.CustomHeaders), nullString(cfg.CustomUserAgent), nullString(cfg.ProxyURL),
		cfg.Blocked, nullString(cfg.BlockedReason), nullString(cfg.Notes),
		now, now,
	).Scan(&cfg.ID)
}

func (r *Repository) Update(ctx context.Context, cfg *DomainConfig) error {
	now := time.Now().UTC()
	cfg.UpdatedAt = now
	_, err := r.db.ExecContext(ctx,
		`UPDATE domain_configs SET
		    is_enabled=$1, match_subdomains=$2, priority=$3,
		    handler_chain=$4, request_timeout_ms=$5, max_retries=$6,
		    min_content_length=$7, failure_patterns=$8, required_patterns=$9,
		    custom_headers=$10, custom_user_agent=$11, proxy_url=$12,
		    blocked=$13, blocked_reason=$14, notes=$15, updated_at=$16
		 WHERE domain=$17`,
		cfg.IsEnabled, cfg.MatchSubdomains, cfg.Priority,
		joinStrings(cfg.HandlerChain), cfg.RequestTimeoutMs, cfg.MaxRetries,
		cfg.MinContentLength, joinStrings(cfg.FailurePatterns), joinStrings(cfg.RequiredPatterns),
		marshalHeaders(cfg.CustomHeaders), nullString(cfg.CustomUserAgent), nullString(cfg.ProxyURL),
		cfg.Blocked, nullString(cfg.BlockedReason), nullString(cfg.Notes),
		now, cfg.Domain,
	)
	return err
}

func (r *Repository) Delete(ctx context.Context, domain string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM domain_configs WHERE domain = $1`, domain)
	return err
}

// --- helpers ---

func scanConfig(rows *sql.Rows) (*DomainConfig, error) {
	var cfg DomainConfig
	var chain, failPat, reqPat, headers string
	var userAgent, proxyURL, blockedReason, notes sql.NullString
	err := rows.Scan(
		&cfg.ID, &cfg.Domain, &cfg.IsEnabled, &cfg.MatchSubdomains, &cfg.Priority,
		&chain, &cfg.RequestTimeoutMs, &cfg.MaxRetries,
		&cfg.MinContentLength, &failPat, &reqPat,
		&headers, &userAgent, &proxyURL,
		&cfg.Blocked, &blockedReason, &notes, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	cfg.HandlerChain = splitStrings(chain)
	cfg.FailurePatterns = splitStrings(failPat)
	cfg.RequiredPatterns = splitStrings(reqPat)
	cfg.CustomHeaders = unmarshalHeaders(headers)
	cfg.CustomUserAgent = userAgent.String
	cfg.ProxyURL = proxyURL.String
	cfg.BlockedReason = blockedReason.String
	cfg.Notes = notes.String
	return &cfg, nil
}

func scanConfigRow(row *sql.Row) (*DomainConfig, error) {
	var cfg DomainConfig
	var chain, failPat, reqPat, headers string
	var userAgent, proxyURL, blockedReason, notes sql.NullString
	err := row.Scan(
		&cfg.ID, &cfg.Domain, &cfg.IsEnabled, &cfg.MatchSubdomains, &cfg.Priority,
		&chain, &cfg.RequestTimeoutMs, &cfg.MaxRetries,
		&cfg.MinContentLength, &failPat, &reqPat,
		&headers, &userAgent, &proxyURL,
		&cfg.Blocked, &blockedReason, &notes, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	cfg.HandlerChain = splitStrings(chain)
	cfg.FailurePatterns = splitStrings(failPat)
	cfg.RequiredPatterns = splitStrings(reqPat)
	cfg.CustomHeaders = unmarshalHeaders(headers)
	cfg.CustomUserAgent = userAgent.String
	cfg.ProxyURL = proxyURL.String
	cfg.BlockedReason = blockedReason.String
	cfg.Notes = notes.String
	return &cfg, nil
}

func joinStrings(s []string) string {
	return strings.Join(s, ",")
}

func splitStrings(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func marshalHeaders(h map[string]string) string {
	if h == nil {
		return "{}"
	}
	data, _ := json.Marshal(h)
	return string(data)
}

func unmarshalHeaders(s string) map[string]string {
	if s == "" || s == "{}" {
		return nil
	}
	var h map[string]string
	if err := json.Unmarshal([]byte(s), &h); err != nil {
		return nil
	}
	return h
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
