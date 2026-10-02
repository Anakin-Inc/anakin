// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// Bodies are spelled out as raw bytes so the fixtures do not depend on the decoder
// under test.
func TestHTTPHandler_DecodesCharset(t *testing.T) {
	// Pushes the non-ASCII text past the first 1024 bytes, which is as far as
	// encoding detection looks.
	padding := "<!-- " + strings.Repeat("x", 2048) + " -->"

	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{
			name:        "charset from Content-Type header",
			contentType: "text/html; charset=ISO-8859-1",
			body:        "<html><body><p>caf\xe9 cr\xe8me</p></body></html>",
			want:        "café crème",
		},
		{
			name:        "cyrillic charset from Content-Type header",
			contentType: "text/html; charset=windows-1251",
			body:        "<html><body><p>\xcf\xf0\xe8\xe2\xe5\xf2</p></body></html>",
			want:        "Привет",
		},
		{
			name:        "charset from meta tag",
			contentType: "text/html",
			body:        "<html><head><meta charset=\"Shift_JIS\"></head><body><p>\x93\xfa\x96\x7b\x8c\xea</p></body></html>",
			want:        "日本語",
		},
		{
			name:        "charset from http-equiv meta tag",
			contentType: "text/html",
			body:        "<html><head><meta http-equiv=\"Content-Type\" content=\"text/html; charset=windows-1251\"></head><body><p>\xcf\xf0\xe8\xe2\xe5\xf2</p></body></html>",
			want:        "Привет",
		},
		{
			name:        "undeclared legacy page falls back to windows-1252",
			contentType: "text/html",
			body:        "<html><body><p>caf\xe9</p></body></html>",
			want:        "café",
		},
		{
			name:        "declared UTF-8 is untouched",
			contentType: "text/html; charset=utf-8",
			body:        "<html><body><p>café 日本語</p></body></html>",
			want:        "café 日本語",
		},
		{
			name:        "undeclared UTF-8 is untouched even when it starts as plain ASCII",
			contentType: "text/html",
			body:        "<html><body>" + padding + "<p>café 日本語</p></body></html>",
			want:        "café 日本語",
		},
		{
			name:        "UTF-8 mislabelled as a legacy charset is untouched",
			contentType: "text/html; charset=ISO-8859-1",
			body:        "<html><body><p>café 日本語</p></body></html>",
			want:        "café 日本語",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			h := NewHTTPHandler(5*time.Second, "", true)
			res, err := h.Scrape(context.Background(), &models.HandlerRequest{URL: srv.URL})
			if err != nil {
				t.Fatalf("scrape failed: %v", err)
			}
			if !utf8.ValidString(res.HTML) {
				t.Errorf("HTML is not valid UTF-8: %q", res.HTML)
			}
			if !strings.Contains(res.HTML, tt.want) {
				t.Errorf("expected HTML to contain %q, got: %q", tt.want, res.HTML)
			}
		})
	}
}
