// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// Validate reports the first problem that would stop this config from being applied.
//
// Detector.Check skips any pattern that fails to compile, so without this an
// operator can POST a broken regex, get a 201 back, see it listed by GET, and never
// learn that failure detection is not running for the domain. Catching it on the way
// in turns a silent no-op into a 400 that names the pattern.
func (c *DomainConfig) Validate() error {
	if strings.TrimSpace(c.Domain) == "" {
		return fmt.Errorf("domain is required")
	}
	if err := validatePatterns("failurePatterns", c.FailurePatterns); err != nil {
		return err
	}
	if err := validatePatterns("requiredPatterns", c.RequiredPatterns); err != nil {
		return err
	}
	return nil
}

// validatePatterns compiles every non-empty pattern in the list. Empty entries are
// tolerated because Detector.Check already skips them.
func validatePatterns(field string, patterns []string) error {
	for i, p := range patterns {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if _, err := regexp.Compile(p); err != nil {
			return fmt.Errorf("%s[%d] is not a valid regex: %w", field, i, err)
		}
	}
	return nil
}
