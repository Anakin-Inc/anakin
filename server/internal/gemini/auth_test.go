// SPDX-License-Identifier: AGPL-3.0-or-later

package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testAPIKey is shaped like a real Google API key so a partial leak is still caught.
const testAPIKey = "AIzaSyA0testKEYdoNOTuse0123456789abcd"

// newTestClient returns an enabled client pointed at srv.
func newTestClient(baseURL string) *Client {
	return &Client{enabled: true, apiKey: testAPIKey, baseURL: baseURL}
}

// geminiOK is a minimal successful generateContent response.
const geminiOK = `{
  "candidates": [{"content": {"parts": [{"text": "{\"title\":\"ok\"}"}]}, "finishReason": "STOP"}],
  "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}
}`

func TestAPIKeyIsSentAsHeaderNotQueryParameter(t *testing.T) {
	var gotURL, gotQuery, gotHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		gotQuery = r.URL.RawQuery
		gotHeader = r.Header.Get(apiKeyHeader)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(geminiOK))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, _, _, err := c.callGeminiAPI(context.Background(), "prompt", 1024); err != nil {
		t.Fatalf("callGeminiAPI returned an unexpected error: %v", err)
	}

	if gotHeader != testAPIKey {
		t.Errorf("%s header = %q, want the API key", apiKeyHeader, gotHeader)
	}
	if gotQuery != "" {
		t.Errorf("request carried a query string %q; the key must not travel in the URL", gotQuery)
	}
	if strings.Contains(gotURL, testAPIKey) {
		t.Errorf("request URL %q contains the API key", gotURL)
	}
}

// TestTransportErrorDoesNotLeakAPIKey is the regression test for the leak: net/http
// puts the request URL into the *url.Error it returns, and the processor logs that
// error, so anything in the URL ends up in the server log.
func TestTransportErrorDoesNotLeakAPIKey(t *testing.T) {
	// A server that is closed before use gives a deterministic dial failure.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable := srv.URL
	srv.Close()

	c := newTestClient(unreachable)
	_, _, _, err := c.callGeminiAPI(context.Background(), "prompt", 1024)
	if err == nil {
		t.Fatal("expected a transport error from an unreachable endpoint")
	}
	assertNoKey(t, err.Error(), "transport error")
}

func TestHTTPErrorResponseDoesNotLeakAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"quota exceeded"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, _, _, err := c.callGeminiAPI(context.Background(), "prompt", 1024)
	if err == nil {
		t.Fatal("expected an error for HTTP 429")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should report the status code, got: %v", err)
	}
	assertNoKey(t, err.Error(), "HTTP error")
}

// TestExtractJSONFromMarkdownDoesNotLeakAPIKey walks the exported path the processor
// actually calls, since that is the error it hands to slog.
func TestExtractJSONFromMarkdownDoesNotLeakAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable := srv.URL
	srv.Close()

	c := newTestClient(unreachable)
	_, _, err := c.ExtractJSONFromMarkdown(context.Background(), strings.Repeat("content ", 40), "https://example.com")
	if err == nil {
		t.Fatal("expected an error from an unreachable endpoint")
	}
	assertNoKey(t, err.Error(), "ExtractJSONFromMarkdown error")
}

func TestExtractJSONFromMarkdownSucceedsWithHeaderAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(apiKeyHeader) != testAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(geminiOK))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, usage, err := c.ExtractJSONFromMarkdown(context.Background(), strings.Repeat("content ", 40), "https://example.com")
	if err != nil {
		t.Fatalf("extraction failed: %v", err)
	}
	if got == nil {
		t.Fatal("extraction returned no JSON")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(*got), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if parsed["title"] != "ok" {
		t.Errorf("title = %v, want \"ok\"", parsed["title"])
	}
	if usage == nil || usage.TotalTokens != 15 {
		t.Errorf("token usage = %+v, want TotalTokens 15", usage)
	}
}

func TestGenerateContentURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "default endpoint",
			baseURL: "",
			want:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent",
		},
		{
			name:    "override",
			baseURL: "http://127.0.0.1:1234",
			want:    "http://127.0.0.1:1234/v1beta/models/gemini-2.5-flash:generateContent",
		},
		{
			name:    "override with a trailing slash",
			baseURL: "http://127.0.0.1:1234/",
			want:    "http://127.0.0.1:1234/v1beta/models/gemini-2.5-flash:generateContent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{enabled: true, apiKey: testAPIKey, baseURL: tt.baseURL}
			got := c.generateContentURL()
			if got != tt.want {
				t.Errorf("generateContentURL() = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, testAPIKey) || strings.Contains(got, "key=") {
				t.Errorf("generateContentURL() must not carry the API key, got %q", got)
			}
		})
	}
}

// TestDisabledClientMakesNoRequest keeps the graceful-degradation contract: with no
// GEMINI_API_KEY there is nothing to send and nothing to leak.
func TestDisabledClientMakesNoRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := &Client{enabled: false, baseURL: srv.URL}
	got, usage, err := c.ExtractJSONFromMarkdown(context.Background(), strings.Repeat("content ", 40), "https://example.com")
	if err != nil || got != nil || usage != nil {
		t.Errorf("disabled client should return (nil, nil, nil), got (%v, %v, %v)", got, usage, err)
	}
	if called {
		t.Error("disabled client sent a request")
	}
}

func assertNoKey(t *testing.T, s, what string) {
	t.Helper()
	if strings.Contains(s, testAPIKey) {
		t.Errorf("%s leaks the API key: %s", what, s)
	}
	// Catch a reintroduced query parameter even if the key itself were redacted.
	if strings.Contains(s, "key=") {
		t.Errorf("%s carries a key= query parameter: %s", what, s)
	}
}
