// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// --- test doubles ---

// recordingBackend is a ScrapingHandler that records how it was called.
type recordingBackend struct {
	name string

	mu      sync.Mutex
	calls   int
	seen    []*models.HandlerRequest
	result  *models.ScrapeResult
	err     error
	succeed bool
}

func newRecordingBackend() *recordingBackend {
	return &recordingBackend{
		name:   "backend",
		result: &models.ScrapeResult{HTML: "<h1>scraped</h1>", StatusCode: 200},
	}
}

func (b *recordingBackend) Name() string { return b.name }
func (b *recordingBackend) CanHandle(context.Context, *models.HandlerRequest) bool {
	return true
}
func (b *recordingBackend) IsHealthy() bool { return true }

func (b *recordingBackend) Scrape(_ context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	b.seen = append(b.seen, req)
	if b.err != nil {
		return nil, b.err
	}
	return b.result, nil
}

func (b *recordingBackend) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func (b *recordingBackend) lastRequest() *models.HandlerRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.seen) == 0 {
		return nil
	}
	return b.seen[len(b.seen)-1]
}

// facilitatorStub stands in for an x402 facilitator and records the order in
// which its endpoints were hit, so tests can assert verify → scrape → settle.
type facilitatorStub struct {
	server *httptest.Server

	mu           sync.Mutex
	calls        []string
	verifyValid  bool
	verifyReason string
	verifyPayer  string
	settleOK     bool
	settleReason string
	status       int
}

func newFacilitatorStub(t *testing.T) *facilitatorStub {
	t.Helper()
	f := &facilitatorStub{verifyValid: true, settleOK: true, verifyPayer: "nano_payer"}
	mux := http.NewServeMux()
	mux.HandleFunc("/verify", f.handleVerify)
	mux.HandleFunc("/settle", f.handleSettle)
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *facilitatorStub) handleVerify(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, "verify")
	valid, reason, payer, status := f.verifyValid, f.verifyReason, f.verifyPayer, f.status
	f.mu.Unlock()

	if status != 0 {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"isValid":       valid,
		"payer":         payer,
		"invalidReason": reason,
	})
}

func (f *facilitatorStub) handleSettle(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, "settle")
	ok, reason := f.settleOK, f.settleReason
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     ok,
		"transaction": "0xsettled",
		"network":     "nano",
		"errorReason": reason,
	})
}

func (f *facilitatorStub) callOrder() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// paymentOption customises the generated payment payload.
type paymentOption func(map[string]any)

func withNonce(nonce string) paymentOption {
	return func(p map[string]any) { p["nonce"] = nonce }
}

func withValidBefore(v int64) paymentOption {
	return func(p map[string]any) { p["validBefore"] = v }
}

// newPaymentHeader builds a base64 X-PAYMENT header the handler accepts.
func newPaymentHeader(t *testing.T, network string, opts ...paymentOption) string {
	t.Helper()
	auth := map[string]any{
		"from":        "nano_payer",
		"to":          "nano_receiver",
		"value":       10,
		"validAfter":  time.Now().Add(-time.Minute).Unix(),
		"validBefore": time.Now().Add(2 * time.Minute).Unix(),
		"nonce":       "0xnonce-1",
	}
	for _, opt := range opts {
		opt(auth)
	}
	body := map[string]any{
		"x402Version": 1,
		"scheme":      "exact",
		"network":     network,
		"payload": map[string]any{
			"signature":     "0xsignature",
			"authorization": auth,
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal payment: %v", err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// newTestHandler builds a handler wired to the stub facilitator.
func newTestHandler(t *testing.T, f *facilitatorStub, backend ScrapingHandler, mutate func(*XNOHandlerConfig)) *XNOHandler {
	t.Helper()
	cfg := XNOHandlerConfig{
		FacilitatorURL: f.server.URL,
		Network:        "nano",
		PayTo:          "nano_receiver",
		Amount:         "10",
		AssetName:      "XNO",
		Backend:        backend,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	h := NewXNOHandler(cfg)
	if h == nil {
		t.Fatal("NewXNOHandler returned nil for a fully configured handler")
	}
	return h
}

// assertPaymentRequired asserts err is a *PaymentRequiredError and returns it.
func assertPaymentRequired(t *testing.T, err error) *PaymentRequiredError {
	t.Helper()
	var payErr *PaymentRequiredError
	if !errors.As(err, &payErr) {
		t.Fatalf("expected *PaymentRequiredError, got %T: %v", err, err)
	}
	return payErr
}

// --- configuration ---

func TestNewXNOHandler_NilWithoutFacilitatorURL(t *testing.T) {
	h := NewXNOHandler(XNOHandlerConfig{Network: "nano", PayTo: "nano_x", Amount: "10"})
	if h != nil {
		t.Fatal("expected nil handler when FacilitatorURL is empty")
	}
}

func TestXNOHandler_DefaultsApplied(t *testing.T) {
	h := NewXNOHandler(XNOHandlerConfig{
		FacilitatorURL: "https://facilitator.example",
		Network:        "nano",
		PayTo:          "nano_receiver",
		Amount:         "10",
	})
	if h == nil {
		t.Fatal("expected a handler")
	}
	terms := h.Terms()
	if terms.Scheme != "exact" {
		t.Errorf("expected default scheme 'exact', got %q", terms.Scheme)
	}
	if terms.Resource != "/v1/scrape" {
		t.Errorf("expected default resource '/v1/scrape', got %q", terms.Resource)
	}
	if terms.MaxTimeoutSeconds != 60 {
		t.Errorf("expected default MaxTimeoutSeconds 60, got %d", terms.MaxTimeoutSeconds)
	}
	if !strings.Contains(terms.Description, "XNO") && terms.Description == "" {
		t.Errorf("expected a generated description, got %q", terms.Description)
	}
}

func TestXNOHandler_TrailingSlashStrippedFromFacilitator(t *testing.T) {
	h := NewXNOHandler(XNOHandlerConfig{
		FacilitatorURL: "https://facilitator.example/",
		Network:        "nano",
		PayTo:          "nano_receiver",
		Amount:         "10",
	})
	if h.facilitatorURL != "https://facilitator.example" {
		t.Errorf("expected trailing slash stripped, got %q", h.facilitatorURL)
	}
}

func TestXNOHandler_DisabledWhenTermsIncomplete(t *testing.T) {
	tests := []struct {
		name string
		cfg  XNOHandlerConfig
	}{
		{"missing network", XNOHandlerConfig{FacilitatorURL: "https://f.example", PayTo: "nano_x", Amount: "10"}},
		{"missing payTo", XNOHandlerConfig{FacilitatorURL: "https://f.example", Network: "nano", Amount: "10"}},
		{"missing amount", XNOHandlerConfig{FacilitatorURL: "https://f.example", Network: "nano", PayTo: "nano_x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewXNOHandler(tt.cfg)
			if h.CanHandle(context.Background(), &models.HandlerRequest{}) {
				t.Error("expected CanHandle=false with incomplete terms")
			}
			if h.IsHealthy() {
				t.Error("expected IsHealthy=false with incomplete terms")
			}
		})
	}
}

func TestXNOHandler_CanHandleWhenConfigured(t *testing.T) {
	f := newFacilitatorStub(t)
	h := newTestHandler(t, f, newRecordingBackend(), nil)

	if h.Name() != "xno" {
		t.Errorf("expected handler name 'xno', got %q", h.Name())
	}
	if !h.CanHandle(context.Background(), &models.HandlerRequest{}) {
		t.Error("expected CanHandle=true when configured")
	}
	if !h.IsHealthy() {
		t.Error("expected IsHealthy=true when configured")
	}
}

// --- payment required ---

func TestXNOHandler_MissingPaymentIsPaymentRequired(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	_, err := h.Scrape(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	payErr := assertPaymentRequired(t, err)

	if payErr.Terms.MaxAmountRequired != "10" || payErr.Terms.PayTo != "nano_receiver" {
		t.Errorf("expected terms to be returned to the caller, got %+v", payErr.Terms)
	}
	if !strings.Contains(err.Error(), PaymentHeader) {
		t.Errorf("expected error to name the %s header, got %q", PaymentHeader, err.Error())
	}
	if backend.callCount() != 0 {
		t.Error("backend must not run for an unpaid request")
	}
	if len(f.callOrder()) != 0 {
		t.Errorf("facilitator must not be contacted for an unpaid request, got %v", f.callOrder())
	}
}

func TestXNOHandler_RejectsMalformedPayments(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantReason string
	}{
		{"not base64 or json", "!!!not-valid!!!", "not valid JSON"},
		{"valid json wrong version", rawPayment(t, map[string]any{"x402Version": 2, "scheme": "exact", "network": "nano", "payload": map[string]any{}}), "unsupported x402 version"},
		{"valid json wrong scheme", rawPayment(t, map[string]any{"x402Version": 1, "scheme": " upto-date", "network": "nano", "payload": map[string]any{}}), "unsupported payment scheme"},
		{"wrong network", rawPayment(t, map[string]any{"x402Version": 1, "scheme": "exact", "network": "base-sepolia", "payload": map[string]any{}}), "payment is for network"},
		{"empty payload", rawPayment(t, map[string]any{"x402Version": 1, "scheme": "exact", "network": "nano", "payload": nil}), "payload is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFacilitatorStub(t)
			backend := newRecordingBackend()
			h := newTestHandler(t, f, backend, nil)

			_, err := h.Scrape(context.Background(), &models.HandlerRequest{
				URL:           "https://example.com",
				PaymentHeader: tt.header,
			})
			payErr := assertPaymentRequired(t, err)
			if !strings.Contains(payErr.Reason, tt.wantReason) {
				t.Errorf("expected reason to contain %q, got %q", tt.wantReason, payErr.Reason)
			}
			if backend.callCount() != 0 {
				t.Error("backend must not run for a malformed payment")
			}
		})
	}
}

func TestXNOHandler_AcceptsUnencodedJSONPayment(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	header := rawPayment(t, map[string]any{
		"x402Version": 1,
		"scheme":      "exact",
		"network":     "nano",
		"payload": map[string]any{
			"authorization": map[string]any{"nonce": "0xabc", "validBefore": time.Now().Add(time.Minute).Unix()},
		},
	})
	if _, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: header,
	}); err != nil {
		t.Fatalf("expected raw JSON payment to be accepted, got %v", err)
	}
	if backend.callCount() != 1 {
		t.Errorf("expected backend to run once, got %d", backend.callCount())
	}
}

func TestXNOHandler_RejectsExpiredPayment(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	header := newPaymentHeader(t, "nano", withValidBefore(time.Now().Add(-time.Minute).Unix()))
	_, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: header,
	})
	payErr := assertPaymentRequired(t, err)
	if !strings.Contains(payErr.Reason, "expired") {
		t.Errorf("expected an expiry reason, got %q", payErr.Reason)
	}
	if backend.callCount() != 0 {
		t.Error("backend must not run for an expired payment")
	}
}

func TestXNOHandler_RejectsOverlongValidityWindow(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	// A long-lived authorization would stay replayable for its whole window.
	header := newPaymentHeader(t, "nano", withValidBefore(time.Now().Add(48*time.Hour).Unix()))
	_, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: header,
	})
	payErr := assertPaymentRequired(t, err)
	if !strings.Contains(payErr.Reason, "valid for more than") {
		t.Errorf("expected a validity-window reason, got %q", payErr.Reason)
	}
	if backend.callCount() != 0 {
		t.Error("backend must not run for an over-long payment")
	}
}

// --- happy path ---

func TestXNOHandler_VerifyScrapeSettleOrder(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	result, err := h.Scrape(context.Background(), &models.HandlerRequest{
		JobID:         "job-1",
		URL:           "https://example.com",
		PaymentHeader: newPaymentHeader(t, "nano"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HTML != "<h1>scraped</h1>" {
		t.Errorf("expected backend HTML to be returned, got %q", result.HTML)
	}

	order := f.callOrder()
	want := []string{"verify", "settle"}
	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] {
		t.Errorf("expected facilitator calls %v, got %v", want, order)
	}
}

func TestXNOHandler_DoesNotForwardPaymentToBackend(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	if _, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL:           "https://example.com",
		PaymentHeader: newPaymentHeader(t, "nano"),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := backend.lastRequest().PaymentHeader; got != "" {
		t.Errorf("payment header must not reach the scraped origin, got %q", got)
	}
}

func TestXNOHandler_PreservesCallerRequestFields(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	if _, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL:             "https://example.com",
		Country:         "us",
		CustomUserAgent: "test-agent",
		ProxyURL:        "http://proxy:8080",
		PaymentHeader:   newPaymentHeader(t, "nano"),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	seen := backend.lastRequest()
	if seen.Country != "us" || seen.CustomUserAgent != "test-agent" || seen.ProxyURL != "http://proxy:8080" {
		t.Errorf("expected request fields to be preserved, got %+v", seen)
	}
}

// --- replay protection ---

func TestXNOHandler_RejectsReplayedPayment(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	header := newPaymentHeader(t, "nano", withNonce("0xsingle-use"))

	if _, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: header,
	}); err != nil {
		t.Fatalf("first scrape should succeed, got %v", err)
	}

	// The facilitator still considers the captured payload valid — the handler
	// has to refuse it on its own.
	_, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: header,
	})
	payErr := assertPaymentRequired(t, err)
	if !strings.Contains(payErr.Reason, "already been used") {
		t.Errorf("expected a replay reason, got %q", payErr.Reason)
	}
	if backend.callCount() != 1 {
		t.Errorf("replay must not produce a second scrape, backend ran %d times", backend.callCount())
	}
}

func TestXNOHandler_ReplayRejectedWithoutConsultingFacilitator(t *testing.T) {
	f := newFacilitatorStub(t)
	h := newTestHandler(t, f, newRecordingBackend(), nil)

	header := newPaymentHeader(t, "nano", withNonce("0xonce"))
	req := &models.HandlerRequest{URL: "https://example.com", PaymentHeader: header}
	if _, err := h.Scrape(context.Background(), req); err != nil {
		t.Fatalf("first scrape should succeed, got %v", err)
	}
	before := len(f.callOrder())

	if _, err := h.Scrape(context.Background(), req); err == nil {
		t.Fatal("expected the replay to be refused")
	}
	if after := len(f.callOrder()); after != before {
		t.Errorf("replay should be refused locally, but facilitator was called %d more times", after-before)
	}
}

func TestXNOHandler_DistinctNoncesEachSettleOnce(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	for i := 0; i < 3; i++ {
		header := newPaymentHeader(t, "nano", withNonce(fmt.Sprintf("0xnonce-%d", i)))
		if _, err := h.Scrape(context.Background(), &models.HandlerRequest{
			URL: "https://example.com", PaymentHeader: header,
		}); err != nil {
			t.Fatalf("scrape %d should succeed, got %v", i, err)
		}
	}
	if backend.callCount() != 3 {
		t.Errorf("expected 3 scrapes, got %d", backend.callCount())
	}
	if got := len(f.callOrder()); got != 6 {
		t.Errorf("expected 6 facilitator calls (3 verify + 3 settle), got %d", got)
	}
}

// --- failure handling ---

func TestXNOHandler_InvalidPaymentNeverScrapes(t *testing.T) {
	f := newFacilitatorStub(t)
	f.verifyValid = false
	f.verifyReason = "amount exceeds terms"
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	_, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: newPaymentHeader(t, "nano"),
	})
	payErr := assertPaymentRequired(t, err)
	if !strings.Contains(payErr.Reason, "amount exceeds terms") {
		t.Errorf("expected the facilitator reason to surface, got %q", payErr.Reason)
	}
	if backend.callCount() != 0 {
		t.Error("backend must not run when verification fails")
	}
	for _, call := range f.callOrder() {
		if call == "settle" {
			t.Error("must not settle a payment that failed verification")
		}
	}
}

func TestXNOHandler_FacilitatorErrorIsNotPaymentRequired(t *testing.T) {
	f := newFacilitatorStub(t)
	f.status = http.StatusBadGateway
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	// An unreachable facilitator is a server-side fault, not a client payment
	// problem, so it must not be reported as PaymentRequired — the processor
	// treats that type as "stop retrying, tell the caller to pay".
	_, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: newPaymentHeader(t, "nano"),
	})
	var payErr *PaymentRequiredError
	if errors.As(err, &payErr) {
		t.Errorf("facilitator failure should not be a PaymentRequiredError, got %v", err)
	}
	if err == nil {
		t.Fatal("expected an error when the facilitator returns HTTP 502")
	}
	if backend.callCount() != 0 {
		t.Error("backend must not run when verification could not be performed")
	}
}

func TestXNOHandler_SettleFailureFailsClosed(t *testing.T) {
	f := newFacilitatorStub(t)
	f.settleOK = false
	f.settleReason = "insufficient facilitator liquidity"
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	result, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: newPaymentHeader(t, "nano"),
	})
	if err == nil {
		t.Fatal("expected an error when settlement fails")
	}
	if result != nil {
		t.Error("must not return scraped content for an unsettled payment")
	}
	if !strings.Contains(err.Error(), "settlement failed") {
		t.Errorf("expected a settlement failure, got %v", err)
	}
}

func TestXNOHandler_ScrapeFailureDoesNotSettle(t *testing.T) {
	f := newFacilitatorStub(t)
	backend := newRecordingBackend()
	backend.err = errors.New("upstream 403")
	h := newTestHandler(t, f, backend, nil)

	_, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: newPaymentHeader(t, "nano"),
	})
	if err == nil {
		t.Fatal("expected the scrape failure to propagate")
	}
	for _, call := range f.callOrder() {
		if call == "settle" {
			t.Error("must not settle when the scrape produced nothing to bill for")
		}
	}
}

func TestXNOHandler_SettleDisabledSkipsSettlement(t *testing.T) {
	f := newFacilitatorStub(t)
	off := false
	h := newTestHandler(t, f, newRecordingBackend(), func(cfg *XNOHandlerConfig) {
		cfg.Settle = &off
	})

	if _, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: newPaymentHeader(t, "nano"),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	order := f.callOrder()
	if len(order) != 1 || order[0] != "verify" {
		t.Errorf("expected verification only, got %v", order)
	}
}

func TestXNOHandler_FacilitatorOutageReleasesNonceForRetry(t *testing.T) {
	f := newFacilitatorStub(t)
	f.status = http.StatusServiceUnavailable
	backend := newRecordingBackend()
	h := newTestHandler(t, f, backend, nil)

	req := &models.HandlerRequest{
		URL:           "https://example.com",
		PaymentHeader: newPaymentHeader(t, "nano", withNonce("0xretry")),
	}
	if _, err := h.Scrape(context.Background(), req); err == nil {
		t.Fatal("expected an error while the facilitator is down")
	}

	// The payment was never accepted by anyone, so a retry with the same signed
	// payload must work once the facilitator recovers.
	f.mu.Lock()
	f.status = 0
	f.mu.Unlock()

	if _, err := h.Scrape(context.Background(), req); err != nil {
		t.Fatalf("expected retry to succeed after recovery, got %v", err)
	}
	if backend.callCount() != 1 {
		t.Errorf("expected exactly one scrape, got %d", backend.callCount())
	}
}

func TestNonceCache_ReleaseAllowsReclaim(t *testing.T) {
	c := newNonceCache(time.Minute)
	if !c.claim("a") {
		t.Fatal("first claim should succeed")
	}
	if c.claim("a") {
		t.Fatal("second claim should be refused")
	}
	c.release("a")
	if !c.claim("a") {
		t.Error("expected a released nonce to be claimable again")
	}
	if c.size() != 1 {
		t.Errorf("expected 1 tracked entry, got %d", c.size())
	}
}

func TestNonceCache_ReleaseIgnoresEmptyNonce(t *testing.T) {
	c := newNonceCache(time.Minute)
	c.release("")
	if c.size() != 0 {
		t.Errorf("expected nothing tracked, got %d", c.size())
	}
}

func TestXNOHandler_AssetNameIsNotSentToFacilitator(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		if r.URL.Path == "/settle" {
			_, _ = w.Write([]byte(`{"success":true,"transaction":"0x1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"isValid":true,"payer":"p"}`))
	}))
	t.Cleanup(srv.Close)

	backend := newRecordingBackend()
	h := NewXNOHandler(XNOHandlerConfig{
		FacilitatorURL: srv.URL,
		Network:        "nano",
		PayTo:          "nano_receiver",
		Amount:         "10",
		Asset:          "nano:xno",
		AssetName:      "XNO",
		Backend:        backend,
	})

	if _, err := h.Scrape(context.Background(), &models.HandlerRequest{
		URL: "https://example.com", PaymentHeader: newPaymentHeader(t, "nano"),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Terms double as the facilitator's paymentRequirements, so the local-only
	// label must not be serialised into a schema the facilitator has to accept.
	var sent struct {
		Requirements map[string]any `json:"paymentRequirements"`
	}
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("failed to decode facilitator request: %v", err)
	}
	if _, present := sent.Requirements["assetName"]; present {
		t.Errorf("expected assetName to stay local, facilitator received: %s", gotBody)
	}
	if sent.Requirements["asset"] != "nano:xno" {
		t.Errorf("expected the machine asset identifier to be sent, got: %s", gotBody)
	}
}

func TestXNOHandler_ErrorMessageNamesAsset(t *testing.T) {
	f := newFacilitatorStub(t)
	h := newTestHandler(t, f, newRecordingBackend(), nil)

	_, err := h.Scrape(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if !strings.Contains(err.Error(), "XNO") {
		t.Errorf("expected the asset label in %q", err.Error())
	}
}

// --- payload parsing ---

func TestX402PaymentPayload_NonceAndExpiry(t *testing.T) {
	expiry := time.Now().Add(90 * time.Second).Unix()
	tests := []struct {
		name       string
		payload    string
		wantNonce  string
		wantExpiry int64
		wantExpOK  bool
	}{
		{
			name:       "exact scheme nested authorization",
			payload:    fmt.Sprintf(`{"authorization":{"nonce":"0xabc","validBefore":%d}}`, expiry),
			wantNonce:  "0xabc",
			wantExpiry: expiry,
			wantExpOK:  true,
		},
		{
			name:       "flat nonce and expiry",
			payload:    fmt.Sprintf(`{"nonce":"0xflat","validBefore":%d}`, expiry),
			wantNonce:  "0xflat",
			wantExpiry: expiry,
			wantExpOK:  true,
		},
		{
			name:       "string encoded timestamp",
			payload:    fmt.Sprintf(`{"nonce":"0xstr","validBefore":"%d"}`, expiry),
			wantNonce:  "0xstr",
			wantExpiry: expiry,
			wantExpOK:  true,
		},
		{
			name:      "scheme without a nonce",
			payload:   `{"signature":"0xdeadbeef"}`,
			wantNonce: "",
		},
		{
			name:      "malformed payload",
			payload:   `not json`,
			wantNonce: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &x402PaymentPayload{Payload: json.RawMessage(tt.payload)}
			if got := p.Nonce(); got != tt.wantNonce {
				t.Errorf("Nonce() = %q, want %q", got, tt.wantNonce)
			}
			got, ok := p.Expiry()
			if ok != tt.wantExpOK {
				t.Fatalf("Expiry() ok = %v, want %v", ok, tt.wantExpOK)
			}
			if ok && got != tt.wantExpiry {
				t.Errorf("Expiry() = %d, want %d", got, tt.wantExpiry)
			}
		})
	}
}

// --- nonce cache ---

func TestNonceCache_ClaimIsSingleUse(t *testing.T) {
	c := newNonceCache(time.Minute)
	if !c.claim("a") {
		t.Error("first claim should succeed")
	}
	if c.claim("a") {
		t.Error("second claim of the same nonce should fail")
	}
	if !c.claim("b") {
		t.Error("a different nonce should still be claimable")
	}
}

func TestNonceCache_EmptyNonceIsAllowed(t *testing.T) {
	c := newNonceCache(time.Minute)
	if !c.claim("") {
		t.Error("a nonce-less payment should be allowed through")
	}
	if c.size() != 0 {
		t.Errorf("expected nothing tracked for an empty nonce, got %d", c.size())
	}
}

func TestNonceCache_PrunesAfterTTL(t *testing.T) {
	c := newNonceCache(time.Minute)
	now := time.Now()
	c.now = func() time.Time { return now }

	if !c.claim("a") {
		t.Fatal("first claim should succeed")
	}
	now = now.Add(30 * time.Second)
	if c.claim("a") {
		t.Error("nonce should still be spent inside the TTL")
	}

	now = now.Add(2 * time.Minute)
	if !c.claim("a") {
		t.Error("nonce should be forgotten once the TTL has passed")
	}
	if c.size() != 1 {
		t.Errorf("expected the stale entry to be pruned, size = %d", c.size())
	}
}

func TestNonceCache_FailsClosedWhenFull(t *testing.T) {
	c := newNonceCache(time.Minute)
	for i := 0; i < maxTrackedNonces; i++ {
		if !c.claim(fmt.Sprintf("0x%d", i)) {
			t.Fatalf("claim %d should succeed below the cap", i)
		}
	}
	if c.claim("0xoverflow") {
		t.Error("expected the cache to refuse new payments once full")
	}
}

func TestNonceCache_ConcurrentClaims(t *testing.T) {
	c := newNonceCache(time.Minute)
	const workers = 32

	var wg sync.WaitGroup
	granted := make([]bool, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			granted[idx] = c.claim("0xcontended")
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, g := range granted {
		if g {
			wins++
		}
	}
	if wins != 1 {
		t.Errorf("expected exactly one winner for a contended nonce, got %d", wins)
	}
}

// rawPayment marshals body and returns it without base64 encoding.
func rawPayment(t *testing.T, body map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal payment: %v", err)
	}
	return string(raw)
}
