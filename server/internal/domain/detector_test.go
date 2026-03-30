package domain

import (
	"strings"
	"testing"
)

func TestDetector_Check(t *testing.T) {
	d := NewDetector()

	t.Run("content matching failure pattern returns failed", func(t *testing.T) {
		cfg := &DomainConfig{
			FailurePatterns: []string{"Access Denied", "captcha"},
		}
		result := d.Check(cfg, "<html><body>Access Denied - Please verify you are human</body></html>")
		if !result.Failed {
			t.Error("expected failure when content matches a failure pattern")
		}
		if !strings.Contains(result.Reason, "failure pattern matched") {
			t.Errorf("expected reason to mention failure pattern, got: %q", result.Reason)
		}
		if !result.ShouldRetry {
			t.Error("expected ShouldRetry to be true for failure pattern match")
		}
	})

	t.Run("content matching second failure pattern returns failed", func(t *testing.T) {
		cfg := &DomainConfig{
			FailurePatterns: []string{"Access Denied", "captcha"},
		}
		result := d.Check(cfg, "<html><body>Please solve the captcha below</body></html>")
		if !result.Failed {
			t.Error("expected failure when content matches second failure pattern")
		}
	})

	t.Run("content not matching failure patterns passes", func(t *testing.T) {
		cfg := &DomainConfig{
			FailurePatterns: []string{"Access Denied", "captcha"},
		}
		result := d.Check(cfg, "<html><body><h1>Welcome</h1><p>Page content here</p></body></html>")
		if result.Failed {
			t.Errorf("expected pass when content does not match failure patterns, got reason: %q", result.Reason)
		}
	})

	t.Run("content matching required pattern passes", func(t *testing.T) {
		cfg := &DomainConfig{
			RequiredPatterns: []string{"<article", "<div class=\"content\""},
		}
		result := d.Check(cfg, `<html><body><article>Some article content</article></body></html>`)
		if result.Failed {
			t.Errorf("expected pass when content matches required pattern, got reason: %q", result.Reason)
		}
	})

	t.Run("content not matching required patterns returns failed", func(t *testing.T) {
		cfg := &DomainConfig{
			RequiredPatterns: []string{"<article", `<div class="content"`},
		}
		result := d.Check(cfg, "<html><body><p>Just a simple paragraph</p></body></html>")
		if !result.Failed {
			t.Error("expected failure when content does not match any required pattern")
		}
		if !strings.Contains(result.Reason, "no required pattern matched") {
			t.Errorf("expected reason to mention no required pattern matched, got: %q", result.Reason)
		}
		if !result.ShouldRetry {
			t.Error("expected ShouldRetry to be true for missing required pattern")
		}
	})

	t.Run("empty failure patterns always passes", func(t *testing.T) {
		cfg := &DomainConfig{
			FailurePatterns: []string{},
		}
		result := d.Check(cfg, "<html><body>Any content</body></html>")
		if result.Failed {
			t.Errorf("expected pass with empty failure patterns, got reason: %q", result.Reason)
		}
	})

	t.Run("nil failure patterns always passes", func(t *testing.T) {
		cfg := &DomainConfig{
			FailurePatterns: nil,
		}
		result := d.Check(cfg, "<html><body>Any content</body></html>")
		if result.Failed {
			t.Errorf("expected pass with nil failure patterns, got reason: %q", result.Reason)
		}
	})

	t.Run("empty required patterns always passes", func(t *testing.T) {
		cfg := &DomainConfig{
			RequiredPatterns: []string{},
		}
		result := d.Check(cfg, "<html><body>Any content</body></html>")
		if result.Failed {
			t.Errorf("expected pass with empty required patterns, got reason: %q", result.Reason)
		}
	})

	t.Run("nil required patterns always passes", func(t *testing.T) {
		cfg := &DomainConfig{
			RequiredPatterns: nil,
		}
		result := d.Check(cfg, "<html><body>Any content</body></html>")
		if result.Failed {
			t.Errorf("expected pass with nil required patterns, got reason: %q", result.Reason)
		}
	})

	t.Run("required patterns with only empty strings passes", func(t *testing.T) {
		cfg := &DomainConfig{
			RequiredPatterns: []string{"", ""},
		}
		result := d.Check(cfg, "<html><body>Any content</body></html>")
		if result.Failed {
			t.Errorf("expected pass when required patterns are all empty strings, got reason: %q", result.Reason)
		}
	})

	t.Run("invalid regex in failure patterns does not panic", func(t *testing.T) {
		cfg := &DomainConfig{
			FailurePatterns: []string{"[invalid(regex"},
		}
		// Should not panic
		result := d.Check(cfg, "<html><body>Some content</body></html>")
		if result.Failed {
			t.Error("expected invalid regex to be skipped, not cause failure")
		}
	})

	t.Run("invalid regex in required patterns does not panic", func(t *testing.T) {
		cfg := &DomainConfig{
			RequiredPatterns: []string{"[invalid(regex"},
		}
		// Should not panic; the invalid pattern is skipped and since
		// no valid pattern can match, it reports failure
		result := d.Check(cfg, "<html><body>Some content</body></html>")
		if !result.Failed {
			t.Error("expected failure since the only required pattern is invalid and cannot match")
		}
	})

	t.Run("regex failure patterns work", func(t *testing.T) {
		cfg := &DomainConfig{
			FailurePatterns: []string{`\b403\b.*Forbidden`},
		}
		result := d.Check(cfg, "<html><body>Error 403 Forbidden</body></html>")
		if !result.Failed {
			t.Error("expected regex failure pattern to match")
		}
	})

	t.Run("regex failure pattern does not match unrelated content", func(t *testing.T) {
		cfg := &DomainConfig{
			FailurePatterns: []string{`\b403\b.*Forbidden`},
		}
		result := d.Check(cfg, "<html><body>Page loaded successfully</body></html>")
		if result.Failed {
			t.Errorf("expected regex failure pattern not to match, got reason: %q", result.Reason)
		}
	})

	t.Run("content below minimum length returns failed", func(t *testing.T) {
		cfg := &DomainConfig{
			MinContentLength: 100,
		}
		result := d.Check(cfg, "<p>Short</p>")
		if !result.Failed {
			t.Error("expected failure when content is below minimum length")
		}
		if !strings.Contains(result.Reason, "content too short") {
			t.Errorf("expected reason to mention content too short, got: %q", result.Reason)
		}
	})

	t.Run("content above minimum length passes", func(t *testing.T) {
		cfg := &DomainConfig{
			MinContentLength: 10,
		}
		result := d.Check(cfg, "<html><body><p>This is enough content to pass the minimum length check.</p></body></html>")
		if result.Failed {
			t.Errorf("expected pass when content meets minimum length, got reason: %q", result.Reason)
		}
	})

	t.Run("zero minimum content length skips length check", func(t *testing.T) {
		cfg := &DomainConfig{
			MinContentLength: 0,
		}
		result := d.Check(cfg, "x")
		if result.Failed {
			t.Error("expected pass when MinContentLength is 0")
		}
	})

	t.Run("empty config passes any content", func(t *testing.T) {
		cfg := &DomainConfig{}
		result := d.Check(cfg, "<html><body>Anything</body></html>")
		if result.Failed {
			t.Errorf("expected pass with empty config, got reason: %q", result.Reason)
		}
	})

	t.Run("failure patterns checked before required patterns", func(t *testing.T) {
		cfg := &DomainConfig{
			FailurePatterns:  []string{"blocked"},
			RequiredPatterns: []string{"<article"},
		}
		// Content matches both a failure pattern and a required pattern
		result := d.Check(cfg, "<article>You have been blocked</article>")
		if !result.Failed {
			t.Error("expected failure: failure patterns should be checked before required patterns")
		}
		if !strings.Contains(result.Reason, "failure pattern matched") {
			t.Errorf("expected failure pattern reason, got: %q", result.Reason)
		}
	})
}
