package converter

import (
	"strings"
	"testing"
)

func TestHTMLToMarkdown(t *testing.T) {
	t.Run("simple HTML with heading and paragraph", func(t *testing.T) {
		html := "<html><body><h1>Title</h1><p>Body</p></body></html>"
		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Markdown, "# Title") {
			t.Errorf("expected markdown to contain '# Title', got: %q", result.Markdown)
		}
		if !strings.Contains(result.Markdown, "Body") {
			t.Errorf("expected markdown to contain 'Body', got: %q", result.Markdown)
		}
	})

	t.Run("empty HTML", func(t *testing.T) {
		html := ""
		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		trimmed := strings.TrimSpace(result.Markdown)
		if len(trimmed) > 0 {
			// empty HTML may yield empty or whitespace-only markdown
			// but if it does produce content, that's also acceptable
			t.Logf("empty HTML produced markdown: %q (acceptable)", trimmed)
		}
	})

	t.Run("HTML with links produces markdown links", func(t *testing.T) {
		html := `<html><body><p>Visit <a href="https://example.com">Example</a> today.</p></body></html>`
		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Markdown, "[Example]") {
			t.Errorf("expected markdown link text '[Example]', got: %q", result.Markdown)
		}
		if !strings.Contains(result.Markdown, "https://example.com") {
			t.Errorf("expected markdown to contain link URL, got: %q", result.Markdown)
		}
	})

	t.Run("HTML with unordered list produces markdown list", func(t *testing.T) {
		html := `<html><body><ul><li>Alpha</li><li>Beta</li><li>Gamma</li></ul></body></html>`
		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Markdown, "Alpha") {
			t.Errorf("expected markdown to contain list item 'Alpha', got: %q", result.Markdown)
		}
		if !strings.Contains(result.Markdown, "Beta") {
			t.Errorf("expected markdown to contain list item 'Beta', got: %q", result.Markdown)
		}
		// Common markdown list markers: -, *, or numbered
		if !strings.Contains(result.Markdown, "- ") && !strings.Contains(result.Markdown, "* ") {
			t.Errorf("expected markdown list markers (- or *), got: %q", result.Markdown)
		}
	})

	t.Run("HTML with ordered list produces markdown list", func(t *testing.T) {
		html := `<html><body><ol><li>First</li><li>Second</li></ol></body></html>`
		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Markdown, "First") {
			t.Errorf("expected markdown to contain 'First', got: %q", result.Markdown)
		}
		if !strings.Contains(result.Markdown, "Second") {
			t.Errorf("expected markdown to contain 'Second', got: %q", result.Markdown)
		}
	})

	t.Run("boilerplate tags are removed", func(t *testing.T) {
		html := `<html>
			<body>
				<nav><a href="/home">Home</a><a href="/about">About</a></nav>
				<script>var x = 1;</script>
				<style>.hidden{display:none}</style>
				<h1>Main Content</h1>
				<p>This is the real content.</p>
				<footer>Copyright 2025</footer>
			</body>
		</html>`
		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Markdown, "Main Content") {
			t.Errorf("expected main content to be present, got: %q", result.Markdown)
		}
		if strings.Contains(result.Markdown, "var x = 1") {
			t.Errorf("expected script content to be removed, got: %q", result.Markdown)
		}
		if strings.Contains(result.Markdown, ".hidden{display:none}") {
			t.Errorf("expected style content to be removed, got: %q", result.Markdown)
		}
		if strings.Contains(result.Markdown, "Copyright 2025") {
			t.Errorf("expected footer content to be removed, got: %q", result.Markdown)
		}
	})

	t.Run("nav content is removed", func(t *testing.T) {
		html := `<html><body><nav><ul><li>Link1</li><li>Link2</li></ul></nav><p>Real content here</p></body></html>`
		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(result.Markdown, "Link1") {
			t.Errorf("expected nav content to be removed, got: %q", result.Markdown)
		}
		if !strings.Contains(result.Markdown, "Real content here") {
			t.Errorf("expected real content to be preserved, got: %q", result.Markdown)
		}
	})

	t.Run("relative URLs resolved when pageURL provided", func(t *testing.T) {
		html := `<html><body><p><a href="/about">About Us</a></p></body></html>`
		result, err := HTMLToMarkdown(html, "https://example.com/page")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Markdown, "https://example.com/about") {
			t.Errorf("expected resolved URL 'https://example.com/about', got: %q", result.Markdown)
		}
	})

	t.Run("main content extraction prefers main tag", func(t *testing.T) {
		// Build content long enough (>= 100 chars) for extractMainContent to be used
		longContent := strings.Repeat("This is important content. ", 10)
		html := `<html><body>
			<div>Sidebar junk</div>
			<main><p>` + longContent + `</p></main>
			<div>More junk</div>
		</body></html>`
		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Markdown, "important content") {
			t.Errorf("expected main content to be extracted, got: %q", result.Markdown)
		}
	})

	t.Run("result includes cleaned HTML", func(t *testing.T) {
		html := `<html><body><p>Hello World</p></body></html>`
		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.CleanedHTML == "" {
			t.Error("expected CleanedHTML to be non-empty")
		}
		if !strings.Contains(result.CleanedHTML, "Hello World") {
			t.Errorf("expected CleanedHTML to contain 'Hello World', got: %q", result.CleanedHTML)
		}
	})
}
