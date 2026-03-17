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
