// SPDX-License-Identifier: AGPL-3.0-or-later

package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Client handles interactions with Google Gemini API for structured JSON extraction.
// Users supply their own GEMINI_API_KEY. If not configured, the client is disabled
// and all calls gracefully return nil.
type Client struct {
	enabled bool
	apiKey  string
}

// TokenUsage tracks Gemini API token consumption.
type TokenUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

// NewClient creates a new Gemini client.
// Returns a disabled (no-op) client if apiKey is empty.
func NewClient(apiKey string) *Client {
	if apiKey == "" {
		slog.Info("gemini API key not configured — JSON generation disabled")
		return &Client{enabled: false}
	}
	slog.Info("gemini client initialized — JSON generation enabled")
	return &Client{enabled: true, apiKey: apiKey}
}

// IsEnabled returns whether the client has a valid API key.
func (c *Client) IsEnabled() bool { return c.enabled }

// Chunking constants
const (
	maxChunkSize = 30000 // ~7500 tokens per chunk
	chunkOverlap = 4500  // 15% overlap
	maxChunks    = 20

	defaultMaxOutputTokens        = 8192
	productListingMaxOutputTokens = 65535

	defaultModel = "gemini-2.5-flash"

	productListingTimeout = 120 * time.Second
	defaultTimeout        = 60 * time.Second

	productListingMinPrices = 10
	productListingMinLinks  = 10
)

// ExtractJSONFromMarkdown uses Gemini to extract structured JSON from markdown content.
// Returns nil, nil, nil if the client is disabled (graceful degradation).
func (c *Client) ExtractJSONFromMarkdown(ctx context.Context, markdown string, url string) (*string, *TokenUsage, error) {
	if !c.enabled {
		return nil, nil, nil
	}

	if len(strings.TrimSpace(markdown)) < 100 {
		return nil, nil, fmt.Errorf("markdown too short for extraction (%d chars)", len(strings.TrimSpace(markdown)))
	}

	timeout := defaultTimeout
	if isProductListing(markdown) {
		timeout = productListingTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if len(markdown) <= maxChunkSize {
		return c.extractFromSingleChunk(ctx, markdown, url)
	}

	slog.Info("large markdown detected, using chunked extraction", "chars", len(markdown))
	return c.extractFromChunks(ctx, markdown, url)
}

func (c *Client) extractFromSingleChunk(ctx context.Context, markdown string, url string) (*string, *TokenUsage, error) {
	productListing := isProductListing(markdown)

	var prompt string
	maxTokens := defaultMaxOutputTokens
	if productListing {
		slog.Info("product listing detected — using specialized extraction")
		prompt = buildProductListingPrompt(markdown, url)
		maxTokens = productListingMaxOutputTokens
	} else {
		prompt = buildExtractionPrompt(markdown, url)
	}

	jsonText, tokenUsage, wasTruncated, err := c.callGeminiAPI(ctx, prompt, maxTokens)
	if err != nil {
		return nil, nil, fmt.Errorf("gemini API error: %w", err)
	}

	return validateAndFormatJSON(jsonText, tokenUsage, wasTruncated)
}

func (c *Client) extractFromChunks(ctx context.Context, markdown string, url string) (*string, *TokenUsage, error) {
	productListing := isProductListing(markdown)

	chunkInput := markdown
	if productListing {
		chunkInput = stripImageMarkdown(markdown)
		slog.Info("stripped images for chunking",
			"before", len(markdown), "after", len(chunkInput))
	}
	chunks := chunkMarkdown(chunkInput, maxChunkSize, chunkOverlap)

	if len(chunks) > maxChunks {
		slog.Info("limiting chunks", "total", len(chunks), "max", maxChunks)
		chunks = selectMostRelevantChunks(chunks, maxChunks)
	}

	var allResults []map[string]interface{}
	var totalUsage TokenUsage

	if productListing {
		// Parallel chunk processing for product listings
		type chunkResult struct {
			index  int
			result map[string]interface{}
			usage  *TokenUsage
		}

		resultsCh := make(chan chunkResult, len(chunks))
		var wg sync.WaitGroup

		for i, chunk := range chunks {
			wg.Add(1)
			go func(idx int, chk string) {
				defer wg.Done()
				slog.Debug("processing chunk", "chunk", idx+1, "total", len(chunks), "chars", len(chk))

				prompt := buildProductListingChunkPrompt(chk, url, idx+1, len(chunks))
				jsonText, tokenUsage, wasTruncated, err := c.callGeminiAPI(ctx, prompt, productListingMaxOutputTokens)
				if err != nil {
					slog.Warn("chunk extraction failed", "chunk", idx+1, "error", err)
					return
				}

				var parsed map[string]interface{}
				jsonText = cleanJSONResponse(jsonText)
				if wasTruncated {
					jsonText = repairTruncatedJSON(jsonText)
				}
				if err := json.Unmarshal([]byte(jsonText), &parsed); err != nil {
					slog.Warn("chunk returned invalid JSON", "chunk", idx+1, "error", err)
					return
				}

				resultsCh <- chunkResult{index: idx, result: parsed, usage: tokenUsage}
			}(i, chunk)
		}

		wg.Wait()
		close(resultsCh)

		ordered := make([]chunkResult, 0, len(chunks))
		for cr := range resultsCh {
			ordered = append(ordered, cr)
			if cr.usage != nil {
				totalUsage.PromptTokens += cr.usage.PromptTokens
				totalUsage.CompletionTokens += cr.usage.CompletionTokens
				totalUsage.TotalTokens += cr.usage.TotalTokens
			}
		}
		// Sort by index
		for i := 0; i < len(ordered); i++ {
			for j := i + 1; j < len(ordered); j++ {
				if ordered[j].index < ordered[i].index {
					ordered[i], ordered[j] = ordered[j], ordered[i]
				}
			}
		}
		for _, cr := range ordered {
			allResults = append(allResults, cr.result)
		}
	} else {
		// Sequential processing for non-product content
		for i, chunk := range chunks {
			slog.Debug("processing chunk", "chunk", i+1, "total", len(chunks), "chars", len(chunk))

			prompt := buildChunkExtractionPrompt(chunk, url, i+1, len(chunks))
			jsonText, tokenUsage, wasTruncated, err := c.callGeminiAPI(ctx, prompt, defaultMaxOutputTokens)
			if err != nil {
				slog.Warn("chunk extraction failed", "chunk", i+1, "error", err)
				continue
			}

			if tokenUsage != nil {
				totalUsage.PromptTokens += tokenUsage.PromptTokens
				totalUsage.CompletionTokens += tokenUsage.CompletionTokens
				totalUsage.TotalTokens += tokenUsage.TotalTokens
			}

			var chunkResult map[string]interface{}
			jsonText = cleanJSONResponse(jsonText)
			if wasTruncated {
				jsonText = repairTruncatedJSON(jsonText)
			}
			if err := json.Unmarshal([]byte(jsonText), &chunkResult); err != nil {
				slog.Warn("chunk returned invalid JSON", "chunk", i+1, "error", err)
				continue
			}

			allResults = append(allResults, chunkResult)
		}
	}

	if len(allResults) == 0 {
		return nil, nil, fmt.Errorf("all chunk extractions failed")
	}

	var mergedResult map[string]interface{}
	if productListing {
		mergedResult = mergeProductListingResults(allResults)
	} else {
		mergedResult = mergeChunkResults(allResults)
	}

	slog.Info("gemini extraction complete",
		"prompt_tokens", totalUsage.PromptTokens,
		"completion_tokens", totalUsage.CompletionTokens,
		"total_tokens", totalUsage.TotalTokens,
	)

	prettyJSON, err := json.MarshalIndent(mergedResult, "", "  ")
	if err != nil {
		return nil, &totalUsage, fmt.Errorf("failed to format merged JSON: %w", err)
	}

	s := string(prettyJSON)
	return &s, &totalUsage, nil
}

// callGeminiAPI calls the Gemini REST API directly.
// Returns: jsonText, tokenUsage, wasTruncated, error
func (c *Client) callGeminiAPI(ctx context.Context, prompt string, maxOutputTokens int) (string, *TokenUsage, bool, error) {
	requestBody := map[string]interface{}{
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": "You are an expert data extraction AI. Extract structured information from web content as JSON. Always return valid JSON only, with no additional text."},
			},
		},
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
				"role": "user",
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":      0.1,
			"maxOutputTokens":  maxOutputTokens,
			"responseMimeType": "application/json",
			"thinkingConfig": map[string]interface{}{
				"thinkingBudget": 0,
			},
		},
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", nil, false, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", defaultModel, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", nil, false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", nil, false, fmt.Errorf("gemini API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errorBody)
		return "", nil, false, fmt.Errorf("gemini API returned %d: %v", resp.StatusCode, errorBody)
	}

	var apiResponse struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return "", nil, false, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(apiResponse.Candidates) == 0 || len(apiResponse.Candidates[0].Content.Parts) == 0 {
		return "", nil, false, fmt.Errorf("empty response from Gemini")
	}

	jsonText := apiResponse.Candidates[0].Content.Parts[0].Text
	wasTruncated := apiResponse.Candidates[0].FinishReason == "MAX_TOKENS"
	if wasTruncated {
		slog.Warn("gemini output truncated (MAX_TOKENS), attempting repair")
		jsonText = repairTruncatedJSON(jsonText)
	}

	var tokenUsage *TokenUsage
	if apiResponse.UsageMetadata != nil {
		tokenUsage = &TokenUsage{
			PromptTokens:     apiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: apiResponse.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      apiResponse.UsageMetadata.TotalTokenCount,
		}
	}

	return jsonText, tokenUsage, wasTruncated, nil
}

// --- JSON validation and repair ---

func validateAndFormatJSON(jsonText string, tokenUsage *TokenUsage, wasTruncated bool) (*string, *TokenUsage, error) {
	jsonText = cleanJSONResponse(jsonText)
	if wasTruncated {
		jsonText = repairTruncatedJSON(jsonText)
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(jsonText), &parsed); err != nil {
		return nil, tokenUsage, fmt.Errorf("invalid JSON from Gemini: %w", err)
	}

	prettyJSON, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return &jsonText, tokenUsage, nil
	}

	s := string(prettyJSON)
	return &s, tokenUsage, nil
}

func cleanJSONResponse(jsonText string) string {
	jsonText = strings.TrimSpace(jsonText)

	if strings.HasPrefix(jsonText, "```json") {
		jsonText = strings.TrimPrefix(jsonText, "```json")
	} else if strings.HasPrefix(jsonText, "```") {
		jsonText = strings.TrimPrefix(jsonText, "```")
	}
	if strings.HasSuffix(jsonText, "```") {
		jsonText = strings.TrimSuffix(jsonText, "```")
	}

	jsonText = strings.TrimSpace(jsonText)
	jsonText = sanitizeInvalidEscapes(jsonText)
	return jsonText
}

func sanitizeInvalidEscapes(jsonText string) string {
	var buf strings.Builder
	buf.Grow(len(jsonText))

	inString := false
	i := 0
	for i < len(jsonText) {
		ch := jsonText[i]

		if !inString {
			if ch == '"' {
				inString = true
			}
			buf.WriteByte(ch)
			i++
			continue
		}

		if ch == '\\' && i+1 < len(jsonText) {
			next := jsonText[i+1]
			switch next {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				buf.WriteByte(ch)
				buf.WriteByte(next)
				i += 2
			case 'u':
				buf.WriteByte(ch)
				buf.WriteByte(next)
				i += 2
			default:
				buf.WriteByte(next)
				i += 2
			}
			continue
		}

		if ch == '"' {
			inString = false
		}

		buf.WriteByte(ch)
		i++
	}

	return buf.String()
}

func repairTruncatedJSON(jsonText string) string {
	jsonText = strings.TrimSpace(jsonText)

	var stack []rune
	inString := false
	escaped := false

	for _, char := range jsonText {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && inString {
			escaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch char {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == char {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if inString {
		lastSafe := findLastSafePoint(jsonText)
		if lastSafe > 0 {
			jsonText = jsonText[:lastSafe]
			stack = recalculateStack(jsonText)
		} else {
			jsonText += `"`
		}
	}

	for i := len(stack) - 1; i >= 0; i-- {
		jsonText += string(stack[i])
	}

	return jsonText
}

func findLastSafePoint(jsonText string) int {
	inString := false
	escaped := false
	lastComma := -1
	braceDepth := 0
	bracketDepth := 0

	for i, char := range jsonText {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && inString {
			escaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch char {
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case ',':
			if braceDepth >= 0 && bracketDepth >= 0 {
				lastComma = i
			}
		}
	}
	return lastComma
}

func recalculateStack(jsonText string) []rune {
	var stack []rune
	inString := false
	escaped := false

	for _, char := range jsonText {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && inString {
			escaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch char {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == char {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return stack
}

// --- Prompts ---

func buildExtractionPrompt(markdown string, url string) string {
	return fmt.Sprintf(`Analyze the following markdown content from a web page and extract the most important structured information as JSON.

SOURCE URL: %s

INSTRUCTIONS:
1. Analyze the content and automatically determine the best JSON structure
2. Extract key entities, data points, and relationships
3. Create a logical, nested structure that captures the essence of the content
4. Include metadata like title, description, author, dates if clearly present
5. For articles/blogs: extract title, author, published date, content summary, tags
6. For products: extract name, price, description, features, specifications
7. For general pages: extract main heading, sections, key information
8. Use camelCase for all property names
9. Omit fields that are not clearly present in the content
10. Return ONLY valid JSON with no additional text or explanation

MARKDOWN CONTENT:
%s

Extract the structured data as JSON:`, url, markdown)
}

func buildChunkExtractionPrompt(chunk string, url string, chunkNum int, totalChunks int) string {
	return fmt.Sprintf(`Extract structured information from this CHUNK (%d of %d) of a web page as JSON.

SOURCE URL: %s

IMPORTANT: This is part %d of %d chunks from a larger document. Extract ALL relevant data from THIS chunk.

INSTRUCTIONS:
1. Extract key entities, data points, lists, and structured information
2. For product listings: extract each product's name, price, description, features
3. For articles: extract title, sections, key points, metadata
4. For data tables: preserve the tabular structure
5. Use camelCase for property names
6. If this chunk contains items from a list, put them in an "items" array
7. Return ONLY valid JSON

CHUNK CONTENT:
%s

Extract the structured data as JSON:`, chunkNum, totalChunks, url, chunkNum, totalChunks, chunk)
}

func buildProductListingPrompt(markdown string, url string) string {
	return fmt.Sprintf(`Extract ALL products from this product listing page as a JSON object.

SOURCE URL: %s

OUTPUT FORMAT - use this EXACT schema for every product:
{
  "products": [
    {
      "name": "Product Name",
      "url": "full product URL",
      "price": 99.99,
      "originalPrice": 129.99,
      "discount": "23%%%% off",
      "category": "Category Name",
      "colors": 3,
      "imageUrl": "image URL",
      "badges": ["Best Seller"]
    }
  ],
  "totalProducts": 81,
  "pageTitle": "Page Title"
}

CRITICAL RULES:
1. Extract EVERY product - do not skip any
2. The "url" field is REQUIRED - extract from the markdown link [Name](URL)
3. Use the EXACT field names shown above
4. "price" is the current/sale price as a number (no $ sign)
5. "originalPrice" is the original price as a number (null if not shown)
6. "colors" is a number (null if not shown)
7. Include ALL products even if they have fewer fields
8. "totalProducts" should be the actual count of products in the array

MARKDOWN CONTENT:
%s`, url, markdown)
}

func buildProductListingChunkPrompt(chunk string, url string, chunkNum int, totalChunks int) string {
	return fmt.Sprintf(`Extract ALL products from this CHUNK (%d of %d) of a product listing page as a JSON object.

SOURCE URL: %s

IMPORTANT: This is part %d of %d chunks. Extract ALL products from THIS chunk using the EXACT schema below.

OUTPUT FORMAT:
{
  "products": [
    {
      "name": "Product Name",
      "url": "full product URL",
      "price": 99.99,
      "originalPrice": 129.99,
      "discount": "23%%%% off",
      "category": "Category Name",
      "colors": 3,
      "imageUrl": "image URL",
      "badges": ["Best Seller"]
    }
  ],
  "totalProducts": 0,
  "pageTitle": "Page Title"
}

CRITICAL RULES:
1. Extract EVERY product in this chunk - do not skip any
2. The "url" field is REQUIRED - extract from the markdown link [Name](URL)
3. Use the EXACT field names shown above
4. "price" is the current/sale price as a number (no $ sign)
5. "originalPrice" is the original price as a number (null if not shown)
6. "colors" is a number (null if not shown)
7. Include ALL products even if they have fewer fields
8. Set "totalProducts" to the count of products in THIS chunk

CHUNK CONTENT:
%s`, chunkNum, totalChunks, url, chunkNum, totalChunks, chunk)
}

// --- Markdown chunking ---

var markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\([^)]+\)`)
var imageMarkdownPattern = regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)`)

func stripImageMarkdown(markdown string) string {
	return imageMarkdownPattern.ReplaceAllString(markdown, "")
}

func isProductListing(markdown string) bool {
	priceCount := strings.Count(markdown, "$")
	linkCount := len(markdownLinkPattern.FindAllStringIndex(markdown, -1))
	return priceCount >= productListingMinPrices && linkCount >= productListingMinLinks
}

func chunkMarkdown(markdown string, chunkSize int, overlap int) []string {
	if len(markdown) <= chunkSize {
		return []string{markdown}
	}

	var chunks []string
	start := 0

	for start < len(markdown) {
		end := start + chunkSize
		if end >= len(markdown) {
			chunks = append(chunks, markdown[start:])
			break
		}

		breakPoint := findNaturalBreak(markdown, end, chunkSize/4)
		if breakPoint > start {
			end = breakPoint
		}

		chunks = append(chunks, markdown[start:end])

		start = end - overlap
		if start < 0 {
			start = 0
		}
	}

	return chunks
}

func findNaturalBreak(text string, target int, searchRange int) int {
	if target >= len(text) {
		return len(text)
	}

	searchStart := target - searchRange
	if searchStart < 0 {
		searchStart = 0
	}

	searchText := text[searchStart:target]

	if idx := strings.LastIndex(searchText, "\n## "); idx != -1 {
		return searchStart + idx
	}
	if idx := strings.LastIndex(searchText, "\n### "); idx != -1 {
		return searchStart + idx
	}
	if idx := strings.LastIndex(searchText, "\n\n"); idx != -1 {
		return searchStart + idx + 2
	}
	if idx := strings.LastIndex(searchText, "\n"); idx != -1 {
		return searchStart + idx + 1
	}
	for _, sep := range []string{". ", "! ", "? "} {
		if idx := strings.LastIndex(searchText, sep); idx != -1 {
			return searchStart + idx + len(sep)
		}
	}
	return target
}

func selectMostRelevantChunks(chunks []string, maxCount int) []string {
	if len(chunks) <= maxCount {
		return chunks
	}

	type scoredChunk struct {
		index int
		score int
		chunk string
	}

	scored := make([]scoredChunk, len(chunks))
	for i, chunk := range chunks {
		score := 0
		score += strings.Count(chunk, "##") * 10
		score += strings.Count(chunk, "- ") * 2
		score += strings.Count(chunk, "* ") * 2
		score += strings.Count(chunk, "|") * 3
		score += strings.Count(chunk, "$") * 5
		score += strings.Count(chunk, "http") * 2
		score += strings.Count(chunk, "@") * 2
		score += len(strings.Fields(chunk)) / 10
		if i == 0 {
			score += 50
		}
		scored[i] = scoredChunk{index: i, score: score, chunk: chunk}
	}

	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	selected := scored[:maxCount]
	for i := 0; i < len(selected)-1; i++ {
		for j := i + 1; j < len(selected); j++ {
			if selected[j].index < selected[i].index {
				selected[i], selected[j] = selected[j], selected[i]
			}
		}
	}

	result := make([]string, maxCount)
	for i, s := range selected {
		result[i] = s.chunk
	}
	return result
}

// --- Result merging ---

func mergeProductListingResults(results []map[string]interface{}) map[string]interface{} {
	if len(results) == 0 {
		return map[string]interface{}{}
	}
	if len(results) == 1 {
		return results[0]
	}

	var allProducts []map[string]interface{}
	pageTitle := ""

	for _, result := range results {
		for _, key := range []string{"products", "items"} {
			if arr, ok := result[key].([]interface{}); ok {
				for _, item := range arr {
					if product, ok := item.(map[string]interface{}); ok {
						allProducts = append(allProducts, product)
					}
				}
				break
			}
		}
		if pageTitle == "" {
			if pt, ok := result["pageTitle"].(string); ok && pt != "" {
				pageTitle = pt
			}
		}
	}

	// Deduplicate by URL, fall back to name
	seen := make(map[string]map[string]interface{})
	var orderedKeys []string

	for _, product := range allProducts {
		key := ""
		if u, ok := product["url"].(string); ok && u != "" {
			if idx := strings.Index(u, "?"); idx >= 0 {
				key = strings.ToLower(u[:idx])
			} else {
				key = strings.ToLower(u)
			}
		}
		if key == "" {
			if name, ok := product["name"].(string); ok && name != "" {
				key = strings.ToLower(strings.TrimSpace(name))
			}
		}
		if key == "" {
			unnamed := fmt.Sprintf("__unnamed_%d", len(seen))
			seen[unnamed] = product
			orderedKeys = append(orderedKeys, unnamed)
			continue
		}

		if existing, ok := seen[key]; ok {
			mergeProductFields(existing, product)
		} else {
			seen[key] = product
			orderedKeys = append(orderedKeys, key)
		}
	}

	uniqueProducts := make([]interface{}, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		uniqueProducts = append(uniqueProducts, seen[key])
	}

	slog.Info("product listing merge", "total", len(allProducts), "unique", len(uniqueProducts))

	merged := map[string]interface{}{
		"products":      uniqueProducts,
		"totalProducts": float64(len(uniqueProducts)),
	}
	if pageTitle != "" {
		merged["pageTitle"] = pageTitle
	}
	return merged
}

func mergeProductFields(dst, src map[string]interface{}) {
	for key, srcVal := range src {
		dstVal, exists := dst[key]
		if !exists || dstVal == nil {
			dst[key] = srcVal
			continue
		}
		if dstStr, ok := dstVal.(string); ok {
			if srcStr, ok := srcVal.(string); ok && dstStr == "" && srcStr != "" {
				dst[key] = srcVal
			}
		}
	}
}

func mergeChunkResults(results []map[string]interface{}) map[string]interface{} {
	if len(results) == 0 {
		return map[string]interface{}{}
	}
	if len(results) == 1 {
		return results[0]
	}

	merged := make(map[string]interface{})
	var allItems []interface{}

	for _, result := range results {
		for key, value := range result {
			if key == "items" {
				if items, ok := value.([]interface{}); ok {
					allItems = append(allItems, items...)
					continue
				}
			}

			existing, exists := merged[key]
			if !exists {
				merged[key] = value
				continue
			}

			if existingArr, ok := existing.([]interface{}); ok {
				if newArr, ok := value.([]interface{}); ok {
					merged[key] = append(existingArr, newArr...)
					continue
				}
			}

			if existingStr, ok := existing.(string); ok {
				if newStr, ok := value.(string); ok {
					if len(newStr) > len(existingStr) {
						merged[key] = value
					}
					continue
				}
			}
		}
	}

	if len(allItems) > 0 {
		seen := make(map[string]bool)
		var uniqueItems []interface{}
		for _, item := range allItems {
			itemJSON, _ := json.Marshal(item)
			key := string(itemJSON)
			if !seen[key] {
				seen[key] = true
				uniqueItems = append(uniqueItems, item)
			}
		}
		merged["items"] = uniqueItems
	}

	return merged
}
