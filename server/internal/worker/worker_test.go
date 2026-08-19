// SPDX-License-Identifier: AGPL-3.0-or-later

package worker

import (
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

func TestSubmitDoesNotBlockWhenFull(t *testing.T) {
	p := NewPool(nil, 0, 1, time.Second) // buffer of 1, no workers

	if ok := p.Submit(models.JobMessage{JobID: "1"}); !ok {
		t.Fatal("first submit should succeed while the buffer has space")
	}
	if ok := p.Submit(models.JobMessage{JobID: "2"}); ok {
		t.Fatal("second submit should be rejected (buffer full), not block")
	}
}
