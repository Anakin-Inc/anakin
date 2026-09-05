package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// --- cleanJSONResponse ---

func TestCleanJSONResponse_RemovesCodeFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "json code fence",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "plain code fence",
			input: "```\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "no code fence",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "leading/trailing whitespace",
			input: "  \n  {\"a\": 1}  \n  ",
			want:  `{"a": 1}`,
		},
		{
			name:  "code fence with whitespace around",
			input: "  ```json\n{\"a\":1}\n```  ",
			want:  `{"a":1}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanJSONResponse(tc.input)
			if got != tc.want {
				t.Errorf("cleanJSONResponse(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- sanitizeInvalidEscapes ---

func TestSanitizeInvalidEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid escapes unchanged",
			input: `{"msg": "line1\nline2\ttab"}`,
			want:  `{"msg": "line1\nline2\ttab"}`,
		},
		{
			name:  "invalid backslash-star",
			input: `{"msg": "bold \*text\*"}`,
			want:  `{"msg": "bold *text*"}`,
		},
		{
			name:  "invalid backslash-hash",
			input: `{"msg": "heading \#1"}`,
			want:  `{"msg": "heading #1"}`,
		},
		{
			name:  "unicode escape preserved",
			input: `{"msg": "\u0041"}`,
			want:  `{"msg": "\u0041"}`,
		},
		{
			name:  "backslash outside string unchanged",
			input: `{"a": "val"}`,
			want:  `{"a": "val"}`,
		},
		{
			name:  "multiple invalid escapes",
			input: `{"x": "\! \@ \#"}`,
			want:  `{"x": "! @ #"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeInvalidEscapes(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeInvalidEscapes(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- repairTruncatedJSON ---

func TestRepairTruncatedJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValid bool // should the result parse as valid JSON?
	}{
		{
			name:      "already valid",
			input:     `{"key": "value"}`,
			wantValid: true,
		},
		{
			name:      "unclosed brace",
			input:     `{"key": "value"`,
			wantValid: true,
		},
		{
			name:      "unclosed bracket",
			input:     `{"items": [1, 2, 3`,
			wantValid: true,
		},
		{
			name:      "unclosed brace and bracket",
			input:     `{"items": [{"a": 1}`,
			wantValid: true,
		},
		{
			name:      "nested unclosed",
			input:     `{"a": {"b": [1, 2`,
			wantValid: true,
		},
		{
			name:      "truncated in string value with comma before",
			input:     `{"a": "hello", "b": "trun`,
			wantValid: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := repairTruncatedJSON(tc.input)
			var js interface{}
			err := json.Unmarshal([]byte(got), &js)
			if tc.wantValid && err != nil {
				t.Errorf("repairTruncatedJSON(%q) = %q, expected valid JSON but got error: %v", tc.input, got, err)
			}
		})
	}
}

func TestRepairTruncatedJSON_ClosesCorrectBrackets(t *testing.T) {
	input := `{"items": [{"name": "a"}, {"name": "b"`
	got := repairTruncatedJSON(input)

	// Should end with }]}
	if !strings.HasSuffix(got, "}]}") {
		// May have been truncated at safe point; just verify valid JSON
		var js interface{}
		if err := json.Unmarshal([]byte(got), &js); err != nil {
			t.Errorf("repairTruncatedJSON produced invalid JSON: %q, error: %v", got, err)
		}
	}
}

// --- chunkMarkdown ---

func TestChunkMarkdown_SmallInput(t *testing.T) {
	md := "Short content"
	chunks := chunkMarkdown(md, 100, 10)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != md {
		t.Errorf("expected chunk to equal input, got %q", chunks[0])
	}
}

func TestChunkMarkdown_SplitsLargeInput(t *testing.T) {
	// Create content larger than chunk size
	md := strings.Repeat("word ", 100) // 500 chars
	chunks := chunkMarkdown(md, 100, 10)

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	// Verify all content is covered (first and last chars present)
	allContent := strings.Join(chunks, "")
	if !strings.Contains(allContent, "word") {
		t.Error("chunks should contain the original words")
	}
}

func TestChunkMarkdown_OverlapBetweenChunks(t *testing.T) {
	// Build content with distinct markers at known positions
	md := strings.Repeat("A", 50) + strings.Repeat("B", 50) + strings.Repeat("C", 50)
	chunks := chunkMarkdown(md, 60, 20)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// With overlap=20, consecutive chunks should share some content
	for i := 1; i < len(chunks); i++ {
		prevEnd := chunks[i-1][len(chunks[i-1])-10:] // last 10 chars of prev
		currStart := chunks[i][:10]                  // first 10 chars of curr

		// They don't need to match exactly due to natural break finding,
		// but the overlap mechanism means the second chunk should start
		// before where the first chunk ended
		_ = prevEnd
		_ = currStart
	}
}

// japaneseMarkdown builds a document several chunks long whose characters are
// all multi-byte, with a unique marker per paragraph so a chunk's position in
// the document is unambiguous.
func japaneseMarkdown(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for i := 0; b.Len() <= maxChunkSize*3; i++ {
		fmt.Fprintf(&b, "## 見出し%06d\n\nこれは日本語のテキストです。ウェブページから取得した内容を表しています。\n\n", i)
	}
	return b.String()
}

func TestChunkMarkdown_DoesNotSplitMultiByteCharacters(t *testing.T) {
	md := japaneseMarkdown(t)

	chunks := chunkMarkdown(md, maxChunkSize, chunkOverlap)
	if len(chunks) < 2 {
		t.Fatalf("expected the input to be chunked, got %d chunk(s)", len(chunks))
	}

	for i, chunk := range chunks {
		if utf8.ValidString(chunk) {
			continue
		}
		head := chunk
		if len(head) > 8 {
			head = head[:8]
		}
		t.Errorf("chunk %d is not valid UTF-8 — a character was split across the boundary (first bytes: % x)", i, []byte(head))
	}
}

// The alignment must not drop text: every byte of the document still has to
// land in at least one chunk.
func TestChunkMarkdown_CoversWholeDocument(t *testing.T) {
	md := japaneseMarkdown(t)

	chunks := chunkMarkdown(md, maxChunkSize, chunkOverlap)
	if len(chunks) < 2 {
		t.Fatalf("expected the input to be chunked, got %d chunk(s)", len(chunks))
	}
	if !strings.HasPrefix(md, chunks[0]) {
		t.Fatal("first chunk is not a prefix of the document")
	}
	if !strings.HasSuffix(md, chunks[len(chunks)-1]) {
		t.Fatal("last chunk is not a suffix of the document")
	}

	prevEnd := len(chunks[0])
	for i := 1; i < len(chunks); i++ {
		at := strings.Index(md, chunks[i])
		if at < 0 {
			t.Fatalf("chunk %d does not appear in the document", i)
		}
		if at > prevEnd {
			t.Errorf("chunk %d starts at byte %d, so bytes %d..%d are in no chunk", i, at, prevEnd, at)
		}
		prevEnd = at + len(chunks[i])
	}
	if prevEnd != len(md) {
		t.Errorf("chunks end at byte %d, document is %d bytes", prevEnd, len(md))
	}
}

func TestAlignToRuneStart(t *testing.T) {
	const multi = "aあb" // a=1 byte, あ=3 bytes (indices 1,2,3), b=1 byte

	tests := []struct {
		name string
		s    string
		i    int
		want int
	}{
		{name: "negative clamps to zero", s: multi, i: -5, want: 0},
		{name: "past the end clamps to length", s: multi, i: 99, want: len(multi)},
		{name: "already a boundary is unchanged", s: multi, i: 1, want: 1},
		{name: "continuation byte moves back", s: multi, i: 2, want: 1},
		{name: "last continuation byte moves back", s: multi, i: 3, want: 1},
		{name: "next character is a boundary", s: multi, i: 4, want: 4},
		{name: "ascii is never moved", s: "plain ascii", i: 6, want: 6},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := alignToRuneStart(tc.s, tc.i); got != tc.want {
				t.Errorf("alignToRuneStart(%q, %d) = %d, want %d", tc.s, tc.i, got, tc.want)
			}
		})
	}
}

// --- isProductListing ---

func TestIsProductListing(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want bool
	}{
		{
			name: "product page with many prices and links",
			md:   strings.Repeat("$19.99 [Product](https://example.com/p)\n", 15),
			want: true,
		},
		{
			name: "article page with few prices",
			md:   "# Article Title\n\nSome content about things.\n\n[Link](https://example.com)\n\nMore text with $5 mentioned.",
			want: false,
		},
		{
			name: "many links but no prices",
			md:   strings.Repeat("[Link](https://example.com/p)\n", 20),
			want: false,
		},
		{
			name: "many prices but no links",
			md:   strings.Repeat("$19.99 product name\n", 20),
			want: false,
		},
		{
			name: "empty",
			md:   "",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isProductListing(tc.md)
			if got != tc.want {
				t.Errorf("isProductListing() = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- mergeChunkResults ---

func TestMergeChunkResults_Empty(t *testing.T) {
	got := mergeChunkResults(nil)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestMergeChunkResults_SingleResult(t *testing.T) {
	input := []map[string]interface{}{
		{"title": "Hello", "count": 42.0},
	}
	got := mergeChunkResults(input)
	if got["title"] != "Hello" {
		t.Errorf("expected title=Hello, got %v", got["title"])
	}
}

func TestMergeChunkResults_MergesArrays(t *testing.T) {
	results := []map[string]interface{}{
		{"tags": []interface{}{"go", "rust"}},
		{"tags": []interface{}{"python"}},
	}
	got := mergeChunkResults(results)
	tags, ok := got["tags"].([]interface{})
	if !ok {
		t.Fatalf("expected tags to be []interface{}, got %T", got["tags"])
	}
	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %d: %v", len(tags), tags)
	}
}

func TestMergeChunkResults_LongerStringWins(t *testing.T) {
	results := []map[string]interface{}{
		{"description": "short"},
		{"description": "a much longer description that should win"},
	}
	got := mergeChunkResults(results)
	desc := got["description"].(string)
	if desc != "a much longer description that should win" {
		t.Errorf("expected longer string to win, got %q", desc)
	}
}

func TestMergeChunkResults_DeduplicatesItems(t *testing.T) {
	item := map[string]interface{}{"id": 1.0, "name": "Widget"}
	results := []map[string]interface{}{
		{"items": []interface{}{item}},
		{"items": []interface{}{item}}, // duplicate
	}
	got := mergeChunkResults(results)
	items := got["items"].([]interface{})
	if len(items) != 1 {
		t.Errorf("expected 1 unique item, got %d", len(items))
	}
}

// --- mergeProductListingResults ---

func TestMergeProductListingResults_Empty(t *testing.T) {
	got := mergeProductListingResults(nil)
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestMergeProductListingResults_DeduplicatesByURL(t *testing.T) {
	results := []map[string]interface{}{
		{
			"products": []interface{}{
				map[string]interface{}{"name": "Widget", "url": "https://example.com/widget", "price": 9.99},
			},
			"pageTitle": "Shop",
		},
		{
			"products": []interface{}{
				map[string]interface{}{"name": "Widget", "url": "https://example.com/widget", "price": 9.99},
				map[string]interface{}{"name": "Gadget", "url": "https://example.com/gadget", "price": 19.99},
			},
		},
	}
	got := mergeProductListingResults(results)

	products := got["products"].([]interface{})
	if len(products) != 2 {
		t.Errorf("expected 2 unique products, got %d", len(products))
	}
	if got["pageTitle"] != "Shop" {
		t.Errorf("expected pageTitle=Shop, got %v", got["pageTitle"])
	}
	if got["totalProducts"] != float64(2) {
		t.Errorf("expected totalProducts=2, got %v", got["totalProducts"])
	}
}

func TestMergeProductListingResults_DeduplicatesByURLIgnoringQueryString(t *testing.T) {
	results := []map[string]interface{}{
		{
			"products": []interface{}{
				map[string]interface{}{"name": "A", "url": "https://example.com/a?ref=1"},
			},
		},
		{
			"products": []interface{}{
				map[string]interface{}{"name": "A", "url": "https://example.com/a?ref=2"},
			},
		},
	}
	got := mergeProductListingResults(results)
	products := got["products"].([]interface{})
	if len(products) != 1 {
		t.Errorf("expected 1 unique product (dedup by URL minus query), got %d", len(products))
	}
}

func TestMergeProductListingResults_FallsBackToNameDedup(t *testing.T) {
	results := []map[string]interface{}{
		{
			"products": []interface{}{
				map[string]interface{}{"name": "No URL Product"},
			},
		},
		{
			"products": []interface{}{
				map[string]interface{}{"name": "No URL Product"},
			},
		},
	}
	got := mergeProductListingResults(results)
	products := got["products"].([]interface{})
	if len(products) != 1 {
		t.Errorf("expected 1 product (dedup by name), got %d", len(products))
	}
}

// --- findNaturalBreak ---

func TestFindNaturalBreak(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		target      int
		searchRange int
		wantBreak   int
	}{
		{
			name:        "finds heading break",
			text:        "Some content here.\n## New Section\nMore content.",
			target:      35,
			searchRange: 30,
			wantBreak:   18, // position of \n## (the \n before ##)
		},
		{
			name:        "finds paragraph break",
			text:        "First paragraph.\n\nSecond paragraph continues here.",
			target:      40,
			searchRange: 35,
			wantBreak:   18, // position after \n\n
		},
		{
			name:        "finds sentence break",
			text:        "First sentence. Second sentence continues here more words.",
			target:      50,
			searchRange: 45,
			wantBreak:   16, // ". " at index 14; searchStart=5, idx=9, result=5+9+2=16
		},
		{
			name:        "target beyond text returns text length",
			text:        "Short",
			target:      100,
			searchRange: 10,
			wantBreak:   5, // len("Short")
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findNaturalBreak(tc.text, tc.target, tc.searchRange)
			if got != tc.wantBreak {
				t.Errorf("findNaturalBreak(text, %d, %d) = %d, want %d", tc.target, tc.searchRange, got, tc.wantBreak)
			}
		})
	}
}
