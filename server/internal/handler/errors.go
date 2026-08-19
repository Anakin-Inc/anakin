// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"errors"
	"fmt"
	"net/http"
)

// StatusError is returned by handlers when the upstream responded with an HTTP error status (>= 400). It preserves the status code so the processor

type StatusError struct {
	Handler    string
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s handler: HTTP %d %s", e.Handler, e.StatusCode, http.StatusText(e.StatusCode))
}

func (e *StatusError) IsBlocked() bool {
	return IsBlockedStatus(e.StatusCode)
}

func IsBlockedStatus(code int) bool {
	return code == http.StatusForbidden || code == http.StatusTooManyRequests
}

func IsBlockedErr(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.IsBlocked()
	}
	return false
}
