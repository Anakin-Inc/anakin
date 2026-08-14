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

// mainContentSelectors identify the region of a page that holds the real
// content, in priority order. They are used both to extract that region and to
// tell site chrome apart from chrome-shaped tags nested inside the content.
var mainContentSelectors = []string{
	"main", "article", "[role='main']",
	"#content", "#main-content", ".content", ".main-content",
}

// mainContentSelector matches any of the main-content containers at once.
var mainContentSelector = strings.Join(mainContentSelectors, ", ")

// alwaysStripped never carries readable content, wherever it appears. <nav> is
// navigation at any depth, so it belongs here too.
const alwaysStripped = "script, style, noscript, iframe, svg, nav"

// scopedChrome is boilerplate at page level, but semantic HTML also nests these
// inside the content: <article><header> holds the headline and byline, and
// <article><footer> holds tags and the author bio. Only the page-level ones are
// removed — see stripChrome.
const scopedChrome = "header, footer"

// HTMLToMarkdown converts raw HTML to clean markdown.
func HTMLToMarkdown(rawHTML string, pageURL string) (*ConvertResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	doc.Find(alwaysStripped).Remove()
	stripChrome(doc)

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

// stripChrome removes the page-level header and footer without touching the
// same tags when they sit inside a main-content container. Removing every
// <header> also deletes <article><header><h1>…</h1></header>, which is where
// most blog and news pages keep the title of the very page being scraped.
func stripChrome(doc *goquery.Document) {
	doc.Find(scopedChrome).FilterFunction(func(_ int, s *goquery.Selection) bool {
		// ParentsFiltered walks ancestors only, so a <header id="content">
		// still counts as chrome rather than matching itself.
		return s.ParentsFiltered(mainContentSelector).Length() == 0
	}).Remove()
}

func extractMainContent(doc *goquery.Document) string {
	for _, sel := range mainContentSelectors {
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
	base, err := url.Parse(pageURL)
	if err != nil {
		return
	}

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}
		if resolved := resolveURL(base, href); resolved != "" {
			s.SetAttr("href", resolved)
		}
	})

	doc.Find("img[src]").Each(func(_ int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists || src == "" {
			return
		}
		if resolved := resolveURL(base, src); resolved != "" {
			s.SetAttr("src", resolved)
		}
	})
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
