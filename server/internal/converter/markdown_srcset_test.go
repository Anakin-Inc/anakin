// SPDX-License-Identifier: AGPL-3.0-or-later

package converter

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// srcsetPadding keeps extractMainContent above its 100-character threshold so these
// tests exercise the <main> path rather than the body fallback.
const srcsetPadding = "<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor.</p>"

func mainPage(body string) string {
	return fmt.Sprintf("<html><body><main>%s%s</main></body></html>", body, srcsetPadding)
}

func convertOrFail(t *testing.T, html, pageURL string) *ConvertResult {
	t.Helper()
	result, err := HTMLToMarkdown(html, pageURL)
	if err != nil {
		t.Fatalf("HTMLToMarkdown returned error: %v", err)
	}
	return result
}

func TestResolveURLsRewritesImgSrcset(t *testing.T) {
	html := mainPage(`<img src="hero.jpg" srcset="hero-480.jpg 480w, hero-800.jpg 800w" alt="Hero">`)

	result := convertOrFail(t, html, "https://example.com/blog/post")

	want := `srcset="https://example.com/blog/hero-480.jpg 480w, https://example.com/blog/hero-800.jpg 800w"`
	if !strings.Contains(result.CleanedHTML, want) {
		t.Errorf("cleaned HTML missing %s:\n%s", want, result.CleanedHTML)
	}
}

func TestResolveURLsRewritesSourceSrcsetAndSrc(t *testing.T) {
	html := mainPage(
		`<picture><source srcset="wide.jpg 800w" media="(min-width:600px)"><img src="hero.jpg"></picture>` +
			`<video><source src="clip.mp4" type="video/mp4"></video>`,
	)

	result := convertOrFail(t, html, "https://example.com/blog/post")

	for _, want := range []string{
		`srcset="https://example.com/blog/wide.jpg 800w"`,
		`src="https://example.com/blog/clip.mp4"`,
	} {
		if !strings.Contains(result.CleanedHTML, want) {
			t.Errorf("cleaned HTML missing %s:\n%s", want, result.CleanedHTML)
		}
	}
}

func TestResolveURLsRewritesSrcsetAgainstBaseHref(t *testing.T) {
	// srcset must resolve against the same base as src, including a <base href>.
	html := `<html><head><base href="https://cdn.example.com/assets/"></head>` +
		`<body><main><img src="hero.jpg" srcset="hero-2x.jpg 2x">` + srcsetPadding + `</main></body></html>`

	result := convertOrFail(t, html, "https://example.com/blog/post")

	for _, want := range []string{
		`src="https://cdn.example.com/assets/hero.jpg"`,
		`srcset="https://cdn.example.com/assets/hero-2x.jpg 2x"`,
	} {
		if !strings.Contains(result.CleanedHTML, want) {
			t.Errorf("cleaned HTML missing %s:\n%s", want, result.CleanedHTML)
		}
	}
}

func TestResolveURLsLeavesAbsoluteSrcsetCandidatesAlone(t *testing.T) {
	html := mainPage(`<img srcset="https://cdn.example.org/a.jpg 1x, https://cdn.example.org/b.jpg 2x">`)

	result := convertOrFail(t, html, "https://example.com/blog/post")

	want := `srcset="https://cdn.example.org/a.jpg 1x, https://cdn.example.org/b.jpg 2x"`
	if !strings.Contains(result.CleanedHTML, want) {
		t.Errorf("absolute candidates should be unchanged, got:\n%s", result.CleanedHTML)
	}
}

func TestResolveURLsIgnoresBlankSrcset(t *testing.T) {
	html := mainPage(`<img src="hero.jpg" srcset="   ">`)

	result := convertOrFail(t, html, "https://example.com/blog/post")

	if !strings.Contains(result.CleanedHTML, `src="https://example.com/blog/hero.jpg"`) {
		t.Errorf("src should still resolve:\n%s", result.CleanedHTML)
	}
	if !strings.Contains(result.CleanedHTML, `srcset="   "`) {
		t.Errorf("a blank srcset should be left untouched:\n%s", result.CleanedHTML)
	}
}

// rewriteSrcset is unit-tested directly for the parsing edge cases, which are hard
// to reach through the full conversion pipeline.
func TestRewriteSrcset(t *testing.T) {
	base, err := url.Parse("https://example.com/blog/post")
	if err != nil {
		t.Fatalf("failed to parse base: %v", err)
	}

	tests := []struct {
		name   string
		srcset string
		want   string
	}{
		{
			name:   "width descriptors",
			srcset: "a.jpg 480w, b.jpg 800w",
			want:   "https://example.com/blog/a.jpg 480w, https://example.com/blog/b.jpg 800w",
		},
		{
			name:   "density descriptors",
			srcset: "a.jpg 1x, b.jpg 2x",
			want:   "https://example.com/blog/a.jpg 1x, https://example.com/blog/b.jpg 2x",
		},
		{
			name:   "single candidate without descriptor",
			srcset: "a.jpg",
			want:   "https://example.com/blog/a.jpg",
		},
		{
			name:   "candidate without descriptor followed by another",
			srcset: "a.jpg, b.jpg 2x",
			want:   "https://example.com/blog/a.jpg, https://example.com/blog/b.jpg 2x",
		},
		{
			name:   "irregular whitespace and repeated commas",
			srcset: "  a.jpg   1x ,,  b.jpg\t2x  ",
			want:   "https://example.com/blog/a.jpg 1x, https://example.com/blog/b.jpg 2x",
		},
		{
			name:   "newline separated candidates",
			srcset: "a.jpg 1x,\n    b.jpg 2x",
			want:   "https://example.com/blog/a.jpg 1x, https://example.com/blog/b.jpg 2x",
		},
		{
			// A data: URI contains a comma, so splitting the value on commas would
			// tear it in half. The URL runs to the next whitespace instead.
			name:   "data URI candidate keeps its comma",
			srcset: "data:image/gif;base64,R0lGODlhAQABAAA 1x, b.jpg 2x",
			want:   "data:image/gif;base64,R0lGODlhAQABAAA 1x, https://example.com/blog/b.jpg 2x",
		},
		{
			name:   "root relative and parent relative candidates",
			srcset: "/img/a.jpg 1x, ../c.jpg 2x",
			want:   "https://example.com/img/a.jpg 1x, https://example.com/c.jpg 2x",
		},
		{
			name:   "empty value",
			srcset: "",
			want:   "",
		},
		{
			name:   "only separators",
			srcset: " , , ",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rewriteSrcset(base, tt.srcset); got != tt.want {
				t.Errorf("rewriteSrcset(%q)\n got: %q\nwant: %q", tt.srcset, got, tt.want)
			}
		})
	}
}
