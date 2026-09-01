// SPDX-License-Identifier: AGPL-3.0-or-later

package domain

import (
	"reflect"
	"regexp"
	"testing"
)

// roundTrip mimics what the repository does to a list column: encode it on write,
// decode it on the next read.
func roundTrip(t *testing.T, in []string) []string {
	t.Helper()
	return decodeList(encodeList(in))
}

func TestRoundTripPreservesPatternsContainingCommas(t *testing.T) {
	// Regex quantifiers are the common case: a comma inside {m,n} is part of the
	// pattern, not a separator between two patterns.
	tests := []struct {
		name string
		in   []string
	}{
		{
			name: "bounded quantifier",
			in:   []string{`captcha.{0,50}verify`},
		},
		{
			name: "open-ended quantifier",
			in:   []string{`robot[ -]?check{1,}`},
		},
		{
			name: "several patterns, some with commas",
			in: []string{
				`captcha.{0,50}verify`,
				`Access Denied`,
				`(sign in|log in){1,2} to continue`,
			},
		},
		{
			name: "comma as a literal in a character class",
			in:   []string{`price: \d[\d,]{2,9}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTrip(t, tt.in)
			if !reflect.DeepEqual(got, tt.in) {
				t.Errorf("round trip changed the patterns:\n  in:  %q\n  out: %q", tt.in, got)
			}
		})
	}
}

// TestSplitPatternHalvesStillCompile documents why the corruption produced no
// diagnostic at all. Go's regexp treats a stray "{" as a literal, so both halves of
// a split quantifier compile cleanly — Detector.Check's skip-on-compile-error path
// never fires, nothing is logged, and the pattern simply stops matching.
func TestSplitPatternHalvesStillCompile(t *testing.T) {
	const original = `captcha.{0,50}verify`
	const page = `please solve this captcha to verify you are human`

	re, err := regexp.Compile(original)
	if err != nil {
		t.Fatalf("fixture does not compile: %v", err)
	}
	if !re.MatchString(page) {
		t.Fatal("fixture pattern should match the page")
	}

	for _, half := range []string{`captcha.{0`, `50}verify`} {
		re, err := regexp.Compile(half)
		if err != nil {
			t.Fatalf("expected %q to compile without error, got %v", half, err)
		}
		if re.MatchString(page) {
			t.Errorf("half-pattern %q unexpectedly matched; the split would not be silent", half)
		}
	}
}

func TestRoundTripKeepsPatternsCompilable(t *testing.T) {
	patterns := []string{`captcha.{0,50}verify`, `too many requests.{1,20}`}

	for _, p := range roundTrip(t, patterns) {
		if _, err := regexp.Compile(p); err != nil {
			t.Errorf("pattern %q no longer compiles after a round trip: %v", p, err)
		}
	}
}

func TestRoundTripPreservesCharactersNeedingJSONEscapes(t *testing.T) {
	patterns := []string{
		`"Access Denied"`,
		`\d+\s+results`,
		`\\backslash`,
		`unicode ✓ ok`,
		`tab\there`,
	}

	got := roundTrip(t, patterns)
	if !reflect.DeepEqual(got, patterns) {
		t.Errorf("round trip changed the patterns:\n  in:  %q\n  out: %q", patterns, got)
	}
}

func TestRoundTripPreservesOrderAndDuplicates(t *testing.T) {
	patterns := []string{"c", "a", "b", "a"}

	got := roundTrip(t, patterns)
	if !reflect.DeepEqual(got, patterns) {
		t.Errorf("round trip changed the list:\n  in:  %q\n  out: %q", patterns, got)
	}
}

func TestRoundTripHandlerChain(t *testing.T) {
	chain := []string{"http", "browser", "anakin"}

	got := roundTrip(t, chain)
	if !reflect.DeepEqual(got, chain) {
		t.Errorf("round trip changed the handler chain:\n  in:  %q\n  out: %q", chain, got)
	}
}

func TestEncodeList(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		// The list columns are NOT NULL DEFAULT '', so an empty list stays "".
		{name: "nil", in: nil, want: ""},
		{name: "empty", in: []string{}, want: ""},
		{name: "single", in: []string{"captcha"}, want: `["captcha"]`},
		{name: "comma inside a pattern", in: []string{`a{1,2}`}, want: `["a{1,2}"]`},
		{name: "multiple", in: []string{"a", "b"}, want: `["a","b"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeList(tt.in); got != tt.want {
				t.Errorf("encodeList(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDecodeListReadsLegacyCommaJoinedRows(t *testing.T) {
	// Rows written before the JSON encoding must keep working, including the
	// handler_chain column's DEFAULT 'http,browser' from scripts/init-db.sql.
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "schema default handler chain",
			in:   "http,browser",
			want: []string{"http", "browser"},
		},
		{
			name: "legacy patterns without commas",
			in:   "captcha,authwall",
			want: []string{"captcha", "authwall"},
		},
		{
			name: "legacy value with surrounding spaces",
			in:   " captcha , authwall ",
			want: []string{"captcha", "authwall"},
		},
		{
			name: "legacy value with empty entries",
			in:   "captcha,,authwall,",
			want: []string{"captcha", "authwall"},
		},
		{
			name: "single legacy pattern containing a comma stays as stored",
			in:   "a{1,2}",
			want: []string{"a{1", "2}"}, // already corrupted on write; nothing to recover
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeList(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("decodeList(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDecodeListFallsBackWhenLeadingBracketIsNotJSON(t *testing.T) {
	// A character class at the start of a lone legacy pattern looks like the start of
	// a JSON array. It must fall through to the legacy reader rather than vanish.
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "character class", in: `[a-z]+`, want: []string{`[a-z]+`}},
		{name: "negated class", in: `[^0-9]{3}`, want: []string{`[^0-9]{3}`}},
		{name: "bracket then text", in: `[0-9] results`, want: []string{`[0-9] results`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeList(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("decodeList(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDecodeListEmptyValues(t *testing.T) {
	for _, in := range []string{"", "   ", ",", " , , "} {
		if got := decodeList(in); got != nil {
			t.Errorf("decodeList(%q) = %q, want nil", in, got)
		}
	}
}

func TestDecodeListTrimsJSONEntries(t *testing.T) {
	got := decodeList(`["  captcha  ","","authwall"]`)
	want := []string{"captcha", "authwall"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decodeList = %q, want %q", got, want)
	}
}

// TestFailureDetectionSurvivesPersistence is the end-to-end reason this matters: a
// stored failure pattern has to still fire once it is read back.
func TestFailureDetectionSurvivesPersistence(t *testing.T) {
	cfg := &DomainConfig{
		Domain:          "example.com",
		FailurePatterns: []string{`captcha.{0,50}verify`},
	}

	// Persist and reload the way the repository does.
	cfg.FailurePatterns = decodeList(encodeList(cfg.FailurePatterns))

	html := `<html><body>Please solve this captcha to verify you are human</body></html>`
	got := NewDetector().Check(cfg, html)
	if !got.Failed {
		t.Fatalf("stored failure pattern did not match after a round trip: %+v", got)
	}
}

// TestRequiredPatternSurvivesPersistence covers the other list of regexes.
func TestRequiredPatternSurvivesPersistence(t *testing.T) {
	cfg := &DomainConfig{
		Domain:           "example.com",
		RequiredPatterns: []string{`<article[^>]{0,200}>`},
	}
	cfg.RequiredPatterns = decodeList(encodeList(cfg.RequiredPatterns))

	if got := NewDetector().Check(cfg, `<html><article class="post">hi</article></html>`); got.Failed {
		t.Errorf("required pattern should have matched valid content: %+v", got)
	}
	if got := NewDetector().Check(cfg, `<html><body>nothing here</body></html>`); !got.Failed {
		t.Errorf("required pattern should have failed content that lacks it")
	}
}
