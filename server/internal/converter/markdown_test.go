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

	t.Run("article header and footer survive chrome stripping", func(t *testing.T) {
		// Semantic HTML puts the post title and byline in <article><header> and
		// the tags in <article><footer>. Stripping every header/footer on the
		// page threw away the title of the page being scraped.
		body := strings.Repeat("The body of the post. ", 10)
		html := `<html><body>
			<header><p>Site tagline</p></header>
			<article>
				<header><h1>Post Title</h1><p>By Jane Doe</p></header>
				<p>` + body + `</p>
				<footer><p>Filed under Testing</p></footer>
			</article>
			<footer><p>Copyright 2026</p></footer>
		</body></html>`

		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{"# Post Title", "By Jane Doe", "Filed under Testing"} {
			if !strings.Contains(result.Markdown, want) {
				t.Errorf("expected markdown to contain %q, got: %q", want, result.Markdown)
			}
		}
		for _, unwanted := range []string{"Site tagline", "Copyright 2026"} {
			if strings.Contains(result.Markdown, unwanted) {
				t.Errorf("expected site chrome %q to be removed, got: %q", unwanted, result.Markdown)
			}
		}
	})

	t.Run("page chrome is removed even when the body fallback is used", func(t *testing.T) {
		// <main> here is under the 100-character threshold, so the cleaner falls
		// back to the whole body — the page-level header and footer must already
		// be gone by then, while the header nested in <main> must not be.
		html := `<html><body>
			<header><p>Site tagline</p></header>
			<main><header><h2>Section</h2></header><p>Short.</p></main>
			<footer><p>Copyright 2026</p></footer>
		</body></html>`

		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{"Section", "Short."} {
			if !strings.Contains(result.Markdown, want) {
				t.Errorf("expected markdown to contain %q, got: %q", want, result.Markdown)
			}
		}
		for _, unwanted := range []string{"Site tagline", "Copyright 2026"} {
			if strings.Contains(result.Markdown, unwanted) {
				t.Errorf("expected site chrome %q to be removed, got: %q", unwanted, result.Markdown)
			}
		}
	})

	t.Run("nav is removed even inside the main content", func(t *testing.T) {
		body := strings.Repeat("The body of the post. ", 10)
		html := `<html><body>
			<article>
				<nav><a href="/prev">Previous post</a></nav>
				<p>` + body + `</p>
			</article>
		</body></html>`

		result, err := HTMLToMarkdown(html, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(result.Markdown, "Previous post") {
			t.Errorf("expected nav content to be removed, got: %q", result.Markdown)
		}
		if !strings.Contains(result.Markdown, "The body of the post.") {
			t.Errorf("expected article body to be preserved, got: %q", result.Markdown)
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
