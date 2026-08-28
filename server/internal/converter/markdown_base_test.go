// SPDX-License-Identifier: AGPL-3.0-or-later

package converter

import (
	"fmt"
	"strings"
	"testing"
)

// padding keeps extractMainContent above its 100-character threshold so these
// tests exercise the <main> path rather than the body fallback.
const padding = "<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor.</p>"

func pageWithBase(base string, body string) string {
	baseTag := ""
	if base != "" {
		baseTag = fmt.Sprintf("<base href=%q>", base)
	}
	return fmt.Sprintf("<html><head>%s</head><body><main>%s%s</main></body></html>", baseTag, body, padding)
}

func TestResolveURLsHonoursAbsoluteBaseHref(t *testing.T) {
	html := pageWithBase(
		"https://cdn.example.com/docs/",
		`<a href="guide.html">Guide</a><img src="img/logo.png">`,
	)

	result, err := HTMLToMarkdown(html, "https://example.com/blog/post")
	if err != nil {
		t.Fatalf("HTMLToMarkdown returned error: %v", err)
	}

	for _, want := range []string{
		"https://cdn.example.com/docs/guide.html",
		"https://cdn.example.com/docs/img/logo.png",
	} {
		if !strings.Contains(result.CleanedHTML, want) {
			t.Errorf("cleaned HTML missing %q:\n%s", want, result.CleanedHTML)
		}
	}
	if strings.Contains(result.CleanedHTML, "example.com/blog/guide.html") {
		t.Errorf("relative link resolved against the page URL instead of <base href>:\n%s", result.CleanedHTML)
	}
}

func TestResolveURLsHonoursRelativeBaseHref(t *testing.T) {
	// A base href is itself allowed to be relative, and resolves against the page URL.
	html := pageWithBase("/assets/", `<a href="guide.html">Guide</a><img src="logo.png">`)

	result, err := HTMLToMarkdown(html, "https://example.com/blog/post")
	if err != nil {
		t.Fatalf("HTMLToMarkdown returned error: %v", err)
	}

	for _, want := range []string{
		"https://example.com/assets/guide.html",
		"https://example.com/assets/logo.png",
	} {
		if !strings.Contains(result.CleanedHTML, want) {
			t.Errorf("cleaned HTML missing %q:\n%s", want, result.CleanedHTML)
		}
	}
}

func TestResolveURLsUsesFirstBaseHref(t *testing.T) {
	// Browsers honour the first <base href>; later ones are ignored.
	html := `<html><head><base href="https://first.example.com/"><base href="https://second.example.com/"></head>` +
		`<body><main><a href="guide.html">Guide</a>` + padding + `</main></body></html>`

	result, err := HTMLToMarkdown(html, "https://example.com/blog/post")
	if err != nil {
		t.Fatalf("HTMLToMarkdown returned error: %v", err)
	}

	if !strings.Contains(result.CleanedHTML, "https://first.example.com/guide.html") {
		t.Errorf("expected the first <base href> to win:\n%s", result.CleanedHTML)
	}
	if strings.Contains(result.CleanedHTML, "second.example.com") {
		t.Errorf("later <base href> should be ignored:\n%s", result.CleanedHTML)
	}
}

func TestResolveURLsFallsBackToPageURLWithoutBase(t *testing.T) {
	html := pageWithBase("", `<a href="guide.html">Guide</a><img src="img/logo.png">`)

	result, err := HTMLToMarkdown(html, "https://example.com/blog/post")
	if err != nil {
		t.Fatalf("HTMLToMarkdown returned error: %v", err)
	}

	for _, want := range []string{
		"https://example.com/blog/guide.html",
		"https://example.com/blog/img/logo.png",
	} {
		if !strings.Contains(result.CleanedHTML, want) {
			t.Errorf("cleaned HTML missing %q:\n%s", want, result.CleanedHTML)
		}
	}
}

func TestResolveURLsIgnoresEmptyBaseHref(t *testing.T) {
	// An empty or whitespace-only href carries no information — keep the page URL.
	html := pageWithBase("   ", `<a href="guide.html">Guide</a>`)

	result, err := HTMLToMarkdown(html, "https://example.com/blog/post")
	if err != nil {
		t.Fatalf("HTMLToMarkdown returned error: %v", err)
	}

	if !strings.Contains(result.CleanedHTML, "https://example.com/blog/guide.html") {
		t.Errorf("expected fallback to the page URL:\n%s", result.CleanedHTML)
	}
}

func TestResolveURLsWithBaseLeavesNonRelativeRefsAlone(t *testing.T) {
	html := pageWithBase(
		"https://cdn.example.com/docs/",
		`<a href="https://other.example.org/x">Abs</a><a href="mailto:hi@example.com">Mail</a><a href="#top">Top</a>`,
	)

	result, err := HTMLToMarkdown(html, "https://example.com/blog/post")
	if err != nil {
		t.Fatalf("HTMLToMarkdown returned error: %v", err)
	}

	for _, want := range []string{"https://other.example.org/x", "mailto:hi@example.com", "#top"} {
		if !strings.Contains(result.CleanedHTML, want) {
			t.Errorf("cleaned HTML missing %q:\n%s", want, result.CleanedHTML)
		}
	}
}
