package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// DetectionResult holds the outcome of content validation.
type DetectionResult struct {
	Failed      bool
	Reason      string
	ShouldRetry bool
}

// Detector validates scrape results against domain-specific rules.
type Detector struct{}

func NewDetector() *Detector {
	return &Detector{}
}

// Check validates HTML content against the domain config rules.
// Returns a failed result if content doesn't meet the domain's requirements.
func (d *Detector) Check(cfg *DomainConfig, html string) *DetectionResult {
	// Check minimum content length
	if cfg.MinContentLength > 0 && len(html) < cfg.MinContentLength {
		return &DetectionResult{
			Failed:      true,
			Reason:      fmt.Sprintf("content too short: %d < %d bytes", len(html), cfg.MinContentLength),
			ShouldRetry: true,
		}
	}

	// Check failure patterns (any match = failure)
	for _, pattern := range cfg.FailurePatterns {
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(html) {
			return &DetectionResult{
				Failed:      true,
				Reason:      fmt.Sprintf("failure pattern matched: %s", pattern),
				ShouldRetry: true,
			}
		}
	}

	// Check required patterns (at least one must match for success)
	if hasNonEmpty(cfg.RequiredPatterns) {
		matched := false
		for _, pattern := range cfg.RequiredPatterns {
			if pattern == "" {
				continue
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			if re.MatchString(html) {
				matched = true
				break
			}
		}
		if !matched {
			return &DetectionResult{
				Failed:      true,
				Reason:      fmt.Sprintf("no required pattern matched: %s", strings.Join(cfg.RequiredPatterns, ", ")),
				ShouldRetry: true,
			}
		}
	}

	return &DetectionResult{Failed: false}
}

func hasNonEmpty(ss []string) bool {
	for _, s := range ss {
		if s != "" {
			return true
		}
	}
	return false
}
