// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/domain"
)

// fakeRepo records what reached the store, so a test can tell "rejected at the
// boundary" apart from "written and then ignored".
type fakeRepo struct {
	configs map[string]*domain.DomainConfig

	created []*domain.DomainConfig
	updated []*domain.DomainConfig
}

func newFakeRepo(existing ...*domain.DomainConfig) *fakeRepo {
	r := &fakeRepo{configs: map[string]*domain.DomainConfig{}}
	for _, cfg := range existing {
		r.configs[cfg.Domain] = cfg
	}
	return r
}

func (r *fakeRepo) GetAll(context.Context) ([]*domain.DomainConfig, error) {
	out := make([]*domain.DomainConfig, 0, len(r.configs))
	for _, cfg := range r.configs {
		out = append(out, cfg)
	}
	return out, nil
}

func (r *fakeRepo) GetByDomain(_ context.Context, domainName string) (*domain.DomainConfig, error) {
	cfg, ok := r.configs[domainName]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return cfg, nil
}

func (r *fakeRepo) Create(_ context.Context, cfg *domain.DomainConfig) error {
	r.created = append(r.created, cfg)
	r.configs[cfg.Domain] = cfg
	return nil
}

// Update mirrors the repository: no row for that domain is sql.ErrNoRows.
func (r *fakeRepo) Update(_ context.Context, cfg *domain.DomainConfig) error {
	if _, ok := r.configs[cfg.Domain]; !ok {
		return sql.ErrNoRows
	}
	r.updated = append(r.updated, cfg)
	r.configs[cfg.Domain] = cfg
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, domainName string) error {
	delete(r.configs, domainName)
	return nil
}

func newConfigApp(repo configRepository) *fiber.App {
	h := &DomainConfigHandler{repo: repo}
	app := fiber.New()
	app.Post("/v1/domain-configs", h.Create)
	app.Put("/v1/domain-configs/:domain", h.Update)
	app.Get("/v1/domain-configs/:domain", h.Get)
	return app
}

type apiError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func send(t *testing.T, app *fiber.App, method, path, body string) (int, apiError) {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()

	var out apiError
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestCreateRejectsPatternThatDoesNotCompile(t *testing.T) {
	repo := newFakeRepo()
	app := newConfigApp(repo)

	status, body := send(t, app, "POST", "/v1/domain-configs",
		`{"domain":"example.com","failurePatterns":["(captcha"]}`)

	if status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, fiber.StatusBadRequest)
	}
	if body.Error != "invalid_request" {
		t.Errorf("error code = %q, want %q", body.Error, "invalid_request")
	}
	if !strings.Contains(body.Message, "failurePatterns[0]") {
		t.Errorf("message should name the offending pattern, got %q", body.Message)
	}
	if len(repo.created) != 0 {
		t.Errorf("a config the detector cannot use was still persisted: %+v", repo.created)
	}
}

func TestCreateAcceptsValidPatterns(t *testing.T) {
	repo := newFakeRepo()
	app := newConfigApp(repo)

	status, body := send(t, app, "POST", "/v1/domain-configs",
		`{"domain":"example.com","failurePatterns":["captcha.{0,50}","authwall"]}`)

	if status != fiber.StatusCreated {
		t.Fatalf("status = %d (%s), want %d", status, body.Message, fiber.StatusCreated)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created %d configs, want 1", len(repo.created))
	}
	if got := repo.created[0].FailurePatterns; len(got) != 2 {
		t.Errorf("stored patterns = %q, want 2 entries", got)
	}
}

func TestUpdateRejectsPatternThatDoesNotCompile(t *testing.T) {
	repo := newFakeRepo(&domain.DomainConfig{Domain: "example.com"})
	app := newConfigApp(repo)

	status, body := send(t, app, "PUT", "/v1/domain-configs/example.com",
		`{"requiredPatterns":["*oops"]}`)

	if status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, fiber.StatusBadRequest)
	}
	if !strings.Contains(body.Message, "requiredPatterns[0]") {
		t.Errorf("message should name the offending pattern, got %q", body.Message)
	}
	if len(repo.updated) != 0 {
		t.Error("an invalid config was still written")
	}
}

// TestUpdateUnknownDomainReturnsNotFound covers the second half: the UPDATE matched
// no row, so reporting 200 told the caller their config was saved when it was not.
func TestUpdateUnknownDomainReturnsNotFound(t *testing.T) {
	repo := newFakeRepo(&domain.DomainConfig{Domain: "example.com"})
	app := newConfigApp(repo)

	status, body := send(t, app, "PUT", "/v1/domain-configs/never-configured.com",
		`{"failurePatterns":["captcha"]}`)

	if status != fiber.StatusNotFound {
		t.Errorf("status = %d, want %d", status, fiber.StatusNotFound)
	}
	if body.Error != "not_found" {
		t.Errorf("error code = %q, want %q", body.Error, "not_found")
	}
	if len(repo.updated) != 0 {
		t.Error("nothing should have been written for an unknown domain")
	}
}

func TestUpdateExistingDomainSucceeds(t *testing.T) {
	repo := newFakeRepo(&domain.DomainConfig{Domain: "example.com"})
	app := newConfigApp(repo)

	status, body := send(t, app, "PUT", "/v1/domain-configs/example.com",
		`{"failurePatterns":["captcha.{0,50}"]}`)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d (%s), want %d", status, body.Message, fiber.StatusOK)
	}
	if len(repo.updated) != 1 {
		t.Fatalf("updated %d configs, want 1", len(repo.updated))
	}
	if repo.updated[0].Domain != "example.com" {
		t.Errorf("updated domain = %q, want the one from the path", repo.updated[0].Domain)
	}
}

func TestCreateStillRequiresADomain(t *testing.T) {
	app := newConfigApp(newFakeRepo())

	status, body := send(t, app, "POST", "/v1/domain-configs", `{"failurePatterns":["captcha"]}`)
	if status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, fiber.StatusBadRequest)
	}
	if body.Error != "invalid_request" {
		t.Errorf("error code = %q, want %q", body.Error, "invalid_request")
	}
}
