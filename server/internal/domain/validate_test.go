// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import (
	"strings"
	"testing"
)

func TestValidateAcceptsUsableConfigs(t *testing.T) {
	tests := []struct {
		name string
		cfg  DomainConfig
	}{
		{
			name: "minimal",
			cfg:  DomainConfig{Domain: "example.com"},
		},
		{
			name: "realistic patterns",
			cfg: DomainConfig{
				Domain:           "linkedin.com",
				FailurePatterns:  []string{"captcha", `authwall.{0,40}`, `(?i)sign in to continue`},
				RequiredPatterns: []string{`<main`, `experience`},
			},
		},
		{
			name: "empty entries are tolerated, as Detector.Check skips them",
			cfg: DomainConfig{
				Domain:          "example.com",
				FailurePatterns: []string{"", "captcha", "   "},
			},
		},
		{
			name: "no patterns at all",
			cfg: DomainConfig{
				Domain:           "example.com",
				FailurePatterns:  nil,
				RequiredPatterns: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestValidateRejectsPatternsThatDoNotCompile(t *testing.T) {
	tests := []struct {
		name      string
		cfg       DomainConfig
		wantField string
	}{
		{
			name:      "unclosed group in failurePatterns",
			cfg:       DomainConfig{Domain: "example.com", FailurePatterns: []string{"(captcha"}},
			wantField: "failurePatterns[0]",
		},
		{
			name:      "unclosed character class",
			cfg:       DomainConfig{Domain: "example.com", FailurePatterns: []string{"ok", "[a-z"}},
			wantField: "failurePatterns[1]",
		},
		{
			name:      "dangling quantifier in requiredPatterns",
			cfg:       DomainConfig{Domain: "example.com", RequiredPatterns: []string{"*oops"}},
			wantField: "requiredPatterns[0]",
		},
		{
			name:      "unmatched closing paren",
			cfg:       DomainConfig{Domain: "example.com", RequiredPatterns: []string{"captcha)"}},
			wantField: "requiredPatterns[0]",
		},
		{
			name:      "invalid repetition bounds",
			cfg:       DomainConfig{Domain: "example.com", FailurePatterns: []string{"a{5,2}"}},
			wantField: "failurePatterns[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantField) {
				t.Errorf("error should point at %s, got: %v", tt.wantField, err)
			}
		})
	}
}

func TestValidateRequiresADomain(t *testing.T) {
	for _, d := range []string{"", "   "} {
		cfg := DomainConfig{Domain: d}
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate() with domain %q = nil, want an error", d)
		}
	}
}

// TestValidateAgreesWithDetector is the property that matters: anything Validate
// accepts must actually be usable by the detector, and anything it rejects is a
// pattern the detector would have silently dropped.
func TestValidateAgreesWithDetector(t *testing.T) {
	good := DomainConfig{Domain: "example.com", FailurePatterns: []string{`captcha.{0,50}`}}
	if err := good.Validate(); err != nil {
		t.Fatalf("Validate rejected a usable config: %v", err)
	}
	if got := NewDetector().Check(&good, "please solve this captcha now"); !got.Failed {
		t.Error("an accepted pattern should still fire in the detector")
	}

	bad := DomainConfig{Domain: "example.com", FailurePatterns: []string{`(captcha`}}
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate accepted a pattern the detector cannot compile")
	}
	// Confirm the silent no-op this prevents: the detector reports success.
	if got := NewDetector().Check(&bad, "please solve this captcha now"); got.Failed {
		t.Error("expected the uncompilable pattern to be silently skipped by the detector")
	}
}
