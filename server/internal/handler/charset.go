// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"unicode/utf8"

	"golang.org/x/net/html/charset"
)

// decodeBody returns a response body as UTF-8 text.
//
// Everything after the handler — the HTML parser, the markdown converter, the JSON
// encoding of the result — assumes UTF-8, so a page served in a legacy encoding
// (Shift_JIS, windows-1251, ISO-8859-1, ...) has to be transcoded here or every
// non-ASCII character in it ends up as U+FFFD.
//
// A body that is already valid UTF-8 is returned as is, whatever the page declares,
// so pages that work today are not affected. Otherwise the encoding is determined the
// way a browser does it: the Content-Type header, then a <meta> declaration in the
// first 1024 bytes, then windows-1252.
func decodeBody(body []byte, contentType string) string {
	if utf8.Valid(body) {
		return string(body)
	}

	enc, _, _ := charset.DetermineEncoding(body, contentType)
	decoded, err := enc.NewDecoder().Bytes(body)
	if err != nil {
		return string(body)
	}
	return string(decoded)
}
