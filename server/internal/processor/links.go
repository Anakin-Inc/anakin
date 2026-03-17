package processor

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func extractLinks(baseURL, html string, includeSubdomains bool, search string) []string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}

	hostname := strings.ToLower(u.Hostname())
	search = strings.ToLower(search)
	seen := make(map[string]struct{})

	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		href, ok := sel.Attr("href")
		if !ok || strings.TrimSpace(href) == "" {
			return
		}
		parsed, parseErr := url.Parse(strings.TrimSpace(href))
		if parseErr != nil {
			return
		}

		resolved := u.ResolveReference(parsed)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			return
		}

		linkHost := strings.ToLower(resolved.Hostname())
		if includeSubdomains {
			if linkHost != hostname && !strings.HasSuffix(linkHost, "."+hostname) {
				return
			}
		} else if linkHost != hostname {
			return
		}

		resolved.Fragment = ""
		link := resolved.String()

		if search != "" && !strings.Contains(strings.ToLower(link), search) {
			return
		}
		seen[link] = struct{}{}
	})

	links := make([]string, 0, len(seen))
	for link := range seen {
		links = append(links, link)
	}
	sort.Strings(links)
	return links
}

func filterLinksByPatterns(links []string, includePatterns, excludePatterns []string) []string {
	if len(includePatterns) == 0 && len(excludePatterns) == 0 {
		return links
	}

	includes := compilePatterns(includePatterns)
	excludes := compilePatterns(excludePatterns)
	filtered := make([]string, 0, len(links))

	for _, link := range links {
		if len(includes) > 0 && !matchAny(link, includes) {
			continue
		}
		if len(excludes) > 0 && matchAny(link, excludes) {
			continue
		}
		filtered = append(filtered, link)
	}

	return filtered
}

func compilePatterns(patterns []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		quoted := regexp.QuoteMeta(p)
		quoted = strings.ReplaceAll(quoted, "\\*\\*", ".*")
		quoted = strings.ReplaceAll(quoted, "\\*", "[^/]*")
		re, err := regexp.Compile(quoted)
		if err != nil {
			continue
		}
		out = append(out, re)
	}
	return out
}

func matchAny(s string, patterns []*regexp.Regexp) bool {
	for _, re := range patterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
