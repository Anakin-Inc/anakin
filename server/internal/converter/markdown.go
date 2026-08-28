// SPDX-License-Identifier: AGPL-3.0-or-later

package converter

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/PuerkitoBio/goquery"
)

// ConvertResult holds the cleaned HTML and generated markdown.
type ConvertResult struct {
	CleanedHTML string
	Markdown    string
}

// HTMLToMarkdown converts raw HTML to clean markdown.
func HTMLToMarkdown(rawHTML string, pageURL string) (*ConvertResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	doc.Find("script, style, noscript, iframe, svg, nav, footer, header").Remove()

	if pageURL != "" {
		resolveURLs(doc, pageURL)
	}

	mainContent := extractMainContent(doc)

	var cleanedHTML string
	if mainContent != "" && len(mainContent) >= 100 {
		cleanedHTML = mainContent
	} else {
		bodyHTML, err := doc.Find("body").Html()
		if err != nil {
			fullHTML, _ := doc.Html()
			cleanedHTML = fullHTML
		} else {
			cleanedHTML = bodyHTML
		}
	}

	markdown, err := htmltomarkdown.ConvertString(cleanedHTML)
	if err != nil {
		slog.Warn("markdown conversion failed, returning cleaned HTML only", "error", err)
		return &ConvertResult{
			CleanedHTML: cleanedHTML,
			Markdown:    cleanedHTML,
		}, nil
	}

	markdown = strings.TrimSpace(markdown)

	return &ConvertResult{
		CleanedHTML: cleanedHTML,
		Markdown:    markdown,
	}, nil
}

func extractMainContent(doc *goquery.Document) string {
	selectors := []string{
		"main", "article", "[role='main']",
		"#content", "#main-content", ".content", ".main-content",
	}
	for _, sel := range selectors {
		selection := doc.Find(sel)
		if selection.Length() > 0 {
			html, err := selection.First().Html()
			if err == nil && len(strings.TrimSpace(html)) > 0 {
				return html
			}
		}
	}
	return ""
}

func resolveURLs(doc *goquery.Document, pageURL string) {
	page, err := url.Parse(pageURL)
	if err != nil {
		return
	}

	base := documentBase(doc, page)

	resolveAttr(doc, "a[href]", "href", base)
	// <source> carries the media for <picture>, <video> and <audio>; a responsive
	// <img> keeps its candidates in srcset, so src alone misses most of them.
	resolveAttr(doc, "img[src], source[src]", "src", base)
	resolveSrcset(doc, base)
}

// resolveAttr rewrites attr on every element matching selector to an absolute URL.
func resolveAttr(doc *goquery.Document, selector, attr string, base *url.URL) {
	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		value, exists := s.Attr(attr)
		if !exists || value == "" {
			return
		}
		if resolved := resolveURL(base, value); resolved != "" {
			s.SetAttr(attr, resolved)
		}
	})
}

// resolveSrcset rewrites the candidate URLs in every srcset attribute.
func resolveSrcset(doc *goquery.Document, base *url.URL) {
	doc.Find("img[srcset], source[srcset]").Each(func(_ int, s *goquery.Selection) {
		srcset, exists := s.Attr("srcset")
		if !exists || strings.TrimSpace(srcset) == "" {
			return
		}
		if rewritten := rewriteSrcset(base, srcset); rewritten != "" {
			s.SetAttr("srcset", rewritten)
		}
	})
}

// rewriteSrcset resolves every candidate URL in a srcset attribute against base,
// preserving each candidate's descriptor.
//
// A srcset is a comma-separated list of candidates, each a URL optionally followed
// by a width or pixel-density descriptor ("photo-800.jpg 800w"). The URL is
// delimited by whitespace rather than by the comma, so the value cannot simply be
// split on commas — a URL is allowed to contain one, as data: URIs routinely do.
// This follows the splitting rules of the HTML "parse a srcset attribute" algorithm.
func rewriteSrcset(base *url.URL, srcset string) string {
	var candidates []string

	i, n := 0, len(srcset)
	for i < n {
		for i < n && (isASCIIWhitespace(srcset[i]) || srcset[i] == ',') {
			i++
		}
		if i >= n {
			break
		}

		start := i
		for i < n && !isASCIIWhitespace(srcset[i]) {
			i++
		}
		rawURL := srcset[start:i]

		// A URL running up to a comma has no descriptor: the comma ends the candidate.
		descriptor := ""
		if trimmed := strings.TrimRight(rawURL, ","); len(trimmed) != len(rawURL) {
			rawURL = trimmed
		} else {
			descriptorStart := i
			for i < n && srcset[i] != ',' {
				i++
			}
			descriptor = strings.TrimSpace(srcset[descriptorStart:i])
		}

		if rawURL == "" {
			continue
		}
		candidate := resolveURL(base, rawURL)
		if descriptor != "" {
			candidate += " " + descriptor
		}
		candidates = append(candidates, candidate)
	}

	return strings.Join(candidates, ", ")
}

// isASCIIWhitespace reports whether b is one of the five characters HTML treats
// as ASCII whitespace.
func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\f' || b == '\r'
}

// documentBase returns the URL that relative references in the document resolve
// against. That is the page URL, unless the document carries a <base href> — the
// element exists precisely to override it, and browsers honour the first one with
// an href. A base href may itself be relative, so it is resolved against the page
// URL before being used.
func documentBase(doc *goquery.Document, page *url.URL) *url.URL {
	href, exists := doc.Find("base[href]").First().Attr("href")
	if !exists {
		return page
	}
	href = strings.TrimSpace(href)
	if href == "" {
		return page
	}
	ref, err := url.Parse(href)
	if err != nil {
		return page
	}
	return page.ResolveReference(ref)
}

func resolveURL(base *url.URL, rawRef string) string {
	if strings.HasPrefix(rawRef, "data:") ||
		strings.HasPrefix(rawRef, "javascript:") ||
		strings.HasPrefix(rawRef, "mailto:") ||
		strings.HasPrefix(rawRef, "#") {
		return rawRef
	}
	ref, err := url.Parse(rawRef)
	if err != nil {
		return rawRef
	}
	return base.ResolveReference(ref).String()
}
