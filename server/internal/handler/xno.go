// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// XNOHandler settles individual scrapes through an x402 facilitator instead of an
// API-key contract. It sits at the end of the handler chain and gates one scrape
// behind one payment, so an agent pays for exactly the requests it makes.
//
// HANDLER CHAIN POSITION:
//
//	HTTP ──fail──▶ Browser ──fail──▶ [API] ──fail──▶ xno (settled) ──▶ ERROR
//
// The handler is a resource server in the x402 model. A facilitator never fetches
// pages — it only verifies and settles payments — so a scrape runs like this:
//
//  1. Caller sends X-PAYMENT with a signed payment payload for Terms.
//  2. Handler asks the facilitator to /verify the payload against those Terms.
//  3. Handler delegates the scrape to Backend.
//  4. Handler asks the facilitator to /settle, then returns the ScrapeResult.
//
// A request that reaches this handler without a usable payment fails with
// *PaymentRequiredError, which carries the Terms a caller needs in order to pay.
//
// NETWORK AGNOSTIC:
//
//	Nothing here is specific to Nano. Scheme, Network, PayTo and Asset are all
//	operator-supplied, so the same handler settles on whichever network the
//	configured facilitator supports. Point XNO_NETWORK at a Nano facilitator to
//	settle in XNO; point it at an EVM facilitator to settle in USDC. The server
//	never holds a key: callers sign payments with their own wallet, and the
//	operator only configures a receiving address.
//
// SECURITY:
//
//	Verified payloads are single-use. Each authorization nonce is recorded on
//	verify and rejected on reuse for the lifetime of its validity window, so a
//	captured X-PAYMENT header cannot be replayed to obtain extra scrapes. See
//	nonceCache.
type XNOHandler struct {
	facilitatorURL string
	terms          X402Terms
	client         *http.Client
	timeout        time.Duration
	maxPaymentAge  time.Duration
	settle         bool
	backend        ScrapingHandler
	nonces         *nonceCache
}

// XNOHandlerConfig configures a settlement handler.
type XNOHandlerConfig struct {
	// FacilitatorURL is the base URL of the x402 facilitator that verifies and
	// settles payments. The handler is disabled when this is empty.
	FacilitatorURL string

	// Network is the facilitator's identifier for the settlement network,
	// e.g. "nano" or "base-sepolia". Required.
	Network string

	// PayTo is the address that receives payment. Required.
	PayTo string

	// Amount is the price of a single scrape in the asset's base units —
	// the smallest indivisible unit, as an integer string. Required.
	// For Nano this is rawNano ("1 XNO" = 1000000); for USDC it is the
	// 6-decimal base unit ("$0.01" = 10000).
	Amount string

	// Scheme is the x402 payment scheme. Defaults to "exact".
	Scheme string

	// Asset identifies the asset being paid in, using the facilitator's
	// convention (a contract address on EVM networks). Ignored by facilitators
	// that identify assets by network.
	Asset string

	// AssetName is the human-readable asset label used in Terms.Description.
	// Defaults to "the settlement asset" when empty.
	AssetName string

	// Resource is the resource identifier published in Terms.
	// Defaults to "/v1/scrape".
	Resource string

	// Description is the human-readable summary published in Terms.
	// Defaults to a generated "<amount> <asset> per scrape" string.
	Description string

	// MaxTimeoutSeconds is the validity window advertised to callers.
	// Defaults to 60.
	MaxTimeoutSeconds int

	// MaxPaymentAge rejects authorizations whose expiry is further out than
	// this, bounding how long a leaked payload stays replayable.
	// Defaults to 10 minutes.
	MaxPaymentAge time.Duration

	// Settle controls whether a verified payment is also settled with the
	// facilitator. Defaults to true. When false the handler only verifies,
	// which suits a deployment where settlement is batched elsewhere.
	Settle *bool

	// Timeout bounds each facilitator call. Defaults to 15s.
	Timeout time.Duration

	// Backend performs the scrape once payment is verified. Defaults to a
	// direct HTTP GET built with this handler's Timeout, so the SSRF guard in
	// NewHTTPHandler applies.
	//
	// To gate the paid external API instead, pass an *APIHandler here: the
	// payment then buys the API call rather than a plain fetch. Never pass a
	// *Chain — it would recurse.
	Backend ScrapingHandler
}

// X402Terms are the payment terms a resource server publishes. The field names
// match the x402 "accepts" entry so Terms can be handed to a caller or an x402
// client library unchanged.
type X402Terms struct {
	Scheme            string `json:"scheme"`
	Network           string `json:"network"`
	MaxAmountRequired string `json:"maxAmountRequired"`
	Resource          string `json:"resource,omitempty"`
	Description       string `json:"description,omitempty"`
	MimeType          string `json:"mimeType,omitempty"`
	PayTo             string `json:"payTo"`
	MaxTimeoutSeconds int    `json:"maxTimeoutSeconds,omitempty"`
	Asset             string `json:"asset,omitempty"`

	// AssetName is the operator's human-readable label for the asset. It is not
	// part of the x402 schema, so it is never sent to the facilitator; it only
	// appears in logs and error messages.
	AssetName string `json:"-"`
}

// PaymentRequiredError reports that a request reached a settlement handler with
// no usable payment. Terms carries the terms the caller has to satisfy, so a
// caller (or the HTTP layer) can turn this into a 402 challenge.
type PaymentRequiredError struct {
	Handler string
	Terms   X402Terms
	Reason  string
}

// Error always states the terms, because a caller that cannot satisfy them
// cannot act on a bare reason — and appends the reason when there is one.
func (e *PaymentRequiredError) Error() string {
	msg := fmt.Sprintf(
		"%s handler: payment required — %s %s on network %q to %s, valid for %ds (send a signed x402 payment in the %s header)",
		e.Handler, e.Terms.MaxAmountRequired, assetLabel(e.Terms), e.Terms.Network,
		e.Terms.PayTo, e.Terms.MaxTimeoutSeconds, PaymentHeader,
	)
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	return msg
}

// PaymentHeader is the request header carrying a base64-encoded x402 payment
// payload from the caller.
const PaymentHeader = "X-PAYMENT"

// x402Version is the protocol version this handler speaks.
const x402Version = 1

const (
	defaultXNOScheme            = "exact"
	defaultXNOResource          = "/v1/scrape"
	defaultXNOMaxTimeoutSeconds = 60
	defaultXNOTimeout           = 15 * time.Second
	defaultXNOMaxPaymentAge     = 10 * time.Minute
)

// NewXNOHandler creates a settlement handler from cfg. It returns nil when the
// facilitator URL is empty, so callers can register it unconditionally —
// matching the env-var guard used by the other handlers.
func NewXNOHandler(cfg XNOHandlerConfig) *XNOHandler {
	if cfg.FacilitatorURL == "" {
		return nil
	}
	if cfg.Scheme == "" {
		cfg.Scheme = defaultXNOScheme
	}
	if cfg.Resource == "" {
		cfg.Resource = defaultXNOResource
	}
	if cfg.MaxTimeoutSeconds <= 0 {
		cfg.MaxTimeoutSeconds = defaultXNOMaxTimeoutSeconds
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultXNOTimeout
	}
	if cfg.MaxPaymentAge <= 0 {
		cfg.MaxPaymentAge = defaultXNOMaxPaymentAge
	}
	if cfg.AssetName == "" {
		cfg.AssetName = "the settlement asset"
	}
	if cfg.Description == "" {
		cfg.Description = fmt.Sprintf("%s %s per scrape", cfg.Amount, cfg.AssetName)
	}
	settle := true
	if cfg.Settle != nil {
		settle = *cfg.Settle
	}

	h := &XNOHandler{
		facilitatorURL: strings.TrimRight(cfg.FacilitatorURL, "/"),
		client:         &http.Client{Timeout: cfg.Timeout},
		timeout:        cfg.Timeout,
		maxPaymentAge:  cfg.MaxPaymentAge,
		settle:         settle,
		nonces:         newNonceCache(cfg.MaxPaymentAge),
		terms: X402Terms{
			Scheme:            cfg.Scheme,
			Network:           cfg.Network,
			MaxAmountRequired: cfg.Amount,
			Resource:          cfg.Resource,
			Description:       cfg.Description,
			MimeType:          "text/html",
			PayTo:             cfg.PayTo,
			MaxTimeoutSeconds: cfg.MaxTimeoutSeconds,
			Asset:             cfg.Asset,
			AssetName:         cfg.AssetName,
		},
	}

	if cfg.Backend != nil {
		h.backend = cfg.Backend
	} else {
		h.backend = NewHTTPHandler(cfg.Timeout, "", false)
	}
	return h
}

// Terms returns the payment terms this handler requires.
func (h *XNOHandler) Terms() X402Terms { return h.terms }

func (h *XNOHandler) Name() string { return "xno" }

// CanHandle reports whether the handler is configured and the chain may use it.
// Payment enforcement lives in Scrape, not here: returning true for an unpaid
// request is what lets the caller get Terms back instead of silently falling
// through to a different handler.
func (h *XNOHandler) CanHandle(_ context.Context, _ *models.HandlerRequest) bool {
	return h.configured()
}

func (h *XNOHandler) IsHealthy() bool { return h.configured() }

// configured reports whether the handler has enough configuration to attempt a
// settlement. A handler missing its network, pay-to address or price is disabled
// rather than silently serving unpaid scrapes.
func (h *XNOHandler) configured() bool {
	return h != nil &&
		h.facilitatorURL != "" &&
		h.terms.Network != "" &&
		h.terms.PayTo != "" &&
		h.terms.MaxAmountRequired != ""
}

// Scrape verifies the caller's payment, delegates the scrape, then settles.
func (h *XNOHandler) Scrape(ctx context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error) {
	payment, err := h.decodePayment(req.PaymentHeader)
	if err != nil {
		return nil, &PaymentRequiredError{Handler: h.Name(), Terms: h.terms, Reason: err.Error()}
	}

	if err := h.checkFreshness(payment); err != nil {
		return nil, &PaymentRequiredError{Handler: h.Name(), Terms: h.terms, Reason: err.Error()}
	}

	// The nonce is claimed before the facilitator is contacted, so a replayed
	// payload is refused locally instead of costing a round trip and telling the
	// facilitator a payment it has already seen.
	if !h.nonces.claim(payment.Nonce()) {
		return nil, &PaymentRequiredError{
			Handler: h.Name(),
			Terms:   h.terms,
			Reason:  "payment authorization has already been used",
		}
	}

	payer, rejected, err := h.verify(ctx, payment)
	if err != nil {
		if rejected {
			return nil, &PaymentRequiredError{Handler: h.Name(), Terms: h.terms, Reason: err.Error()}
		}
		// The facilitator was unreachable or answered unintelligibly. That is
		// this server's problem, not the caller's payment, so it is returned as
		// a plain error and the job stays eligible for a retry. The nonce is
		// released so a retry does not need a freshly signed payment.
		h.nonces.release(payment.Nonce())
		return nil, fmt.Errorf("%s handler: could not verify payment: %w", h.Name(), err)
	}

	scrapeReq := *req
	scrapeReq.PaymentHeader = ""
	result, err := h.backend.Scrape(ctx, &scrapeReq)
	if err != nil {
		// The scrape failed, so there is nothing to bill for. The nonce stays
		// claimed: the facilitator already accepted the authorization, and
		// releasing it would let a caller retry a failing target without paying
		// again.
		return nil, fmt.Errorf("%s handler: scrape failed after payment: %w", h.Name(), err)
	}

	if h.settle {
		if err := h.settleWith(ctx, payment); err != nil {
			// The scrape already happened and its result is already paid for by
			// a valid authorization, so fail closed on an unsettled payment
			// rather than hand out an unbilled result.
			return nil, fmt.Errorf("%s handler: payment settlement failed: %w", h.Name(), err)
		}
	}

	slog.Info("settled scrape",
		"handler", h.Name(),
		"job_id", req.JobID,
		"payer", payer,
		"amount", h.terms.MaxAmountRequired,
		"asset", assetLabel(h.terms),
		"network", h.terms.Network,
		"backend", h.backend.Name(),
	)

	return result, nil
}

// --- payment payload ---

// x402PaymentPayload is the signed payment a caller presents. Payload stays
// raw because its shape is scheme-specific: the exact scheme carries an EIP-3009
// authorization, other schemes carry something else.
type x402PaymentPayload struct {
	X402Version int             `json:"x402Version"`
	Scheme      string          `json:"scheme"`
	Network     string          `json:"network"`
	Payload     json.RawMessage `json:"payload"`
}

// Nonce returns the authorization nonce, which is the replay-protection key.
// It is empty for payloads whose scheme carries no nonce; the facilitator still
// has to verify those, but this handler cannot deduplicate them.
func (p *x402PaymentPayload) Nonce() string {
	if len(p.Payload) == 0 {
		return ""
	}
	var envelope struct {
		Authorization struct {
			Nonce string `json:"nonce"`
		} `json:"authorization"`
		Nonce string `json:"nonce"`
	}
	if err := json.Unmarshal(p.Payload, &envelope); err != nil {
		return ""
	}
	if envelope.Authorization.Nonce != "" {
		return envelope.Authorization.Nonce
	}
	return envelope.Nonce
}

// Expiry returns the authorization expiry as unix seconds, and whether one was
// present. The field sits at different depths per scheme, so the common shapes
// are tried in turn.
func (p *x402PaymentPayload) Expiry() (int64, bool) {
	if len(p.Payload) == 0 {
		return 0, false
	}
	var envelope struct {
		Authorization struct {
			ValidBefore json.RawMessage `json:"validBefore"`
		} `json:"authorization"`
		ValidBefore json.RawMessage `json:"validBefore"`
		Expiry      json.RawMessage `json:"expiry"`
	}
	if err := json.Unmarshal(p.Payload, &envelope); err != nil {
		return 0, false
	}
	for _, raw := range []json.RawMessage{
		envelope.Authorization.ValidBefore,
		envelope.ValidBefore,
		envelope.Expiry,
	} {
		if secs, ok := parseUnixSeconds(raw); ok {
			return secs, true
		}
	}
	return 0, false
}

// parseUnixSeconds reads a unix timestamp that may be a JSON number or a decimal
// string, which is how the exact scheme encodes it.
func parseUnixSeconds(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case string:
		secs, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, false
		}
		return secs, true
	default:
		return 0, false
	}
}

// decodePayment parses the X-PAYMENT header. The header is base64-encoded JSON;
// a raw JSON object is also accepted so the handler is usable from curl.
func (h *XNOHandler) decodePayment(header string) (*x402PaymentPayload, error) {
	if strings.TrimSpace(header) == "" {
		return nil, errors.New("no payment supplied")
	}

	raw := []byte(header)
	if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header)); err == nil {
		raw = decoded
	}

	var payment x402PaymentPayload
	if err := json.Unmarshal(raw, &payment); err != nil {
		return nil, fmt.Errorf("payment payload is not valid JSON: %w", err)
	}
	if payment.X402Version != x402Version {
		return nil, fmt.Errorf("unsupported x402 version %d (this handler speaks v%d)", payment.X402Version, x402Version)
	}
	if payment.Scheme != h.terms.Scheme {
		return nil, fmt.Errorf("unsupported payment scheme %q (this handler requires %q)", payment.Scheme, h.terms.Scheme)
	}
	if payment.Network != h.terms.Network {
		return nil, fmt.Errorf("payment is for network %q, this handler settles on %q", payment.Network, h.terms.Network)
	}
	if payment.Payload == nil || isJSONNull(payment.Payload) {
		return nil, errors.New("payment payload is empty")
	}
	return &payment, nil
}

// isJSONNull reports whether raw is the JSON literal null, which is what a
// payload field unmarshals to when the caller sent "payload": null. Its length is
// non-zero, so a length check alone would let it through.
func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// checkFreshness refuses an authorization that expires too far in the future.
// Without this bound a caller could mint one long-lived authorization and spend it
// on many scrapes inside the replay-cache TTL.
func (h *XNOHandler) checkFreshness(payment *x402PaymentPayload) error {
	expiry, ok := payment.Expiry()
	if !ok {
		return nil
	}
	now := time.Now().Unix()
	if expiry <= now {
		return fmt.Errorf("payment authorization expired at %d", expiry)
	}
	if maxExpiry := now + int64(h.maxPaymentAge/time.Second); expiry > maxExpiry {
		return fmt.Errorf(
			"payment authorization is valid for more than %s — reissue it with validBefore at most %d seconds from now",
			h.maxPaymentAge, int64(h.maxPaymentAge/time.Second),
		)
	}
	return nil
}

// --- facilitator ---

type x402FacilitatorRequest struct {
	X402Version         int                `json:"x402Version"`
	PaymentPayload      x402PaymentPayload `json:"paymentPayload"`
	PaymentRequirements X402Terms          `json:"paymentRequirements"`
}

type x402VerifyResponse struct {
	IsValid       bool   `json:"isValid"`
	Payer         string `json:"payer"`
	InvalidReason string `json:"invalidReason"`
}

type x402SettleResponse struct {
	Success     bool   `json:"success"`
	Transaction string `json:"transaction"`
	Network     string `json:"network"`
	Payer       string `json:"payer"`
	ErrorReason string `json:"errorReason"`
}

// verify asks the facilitator to confirm the payload satisfies the terms.
//
// rejected distinguishes the two failure modes. A facilitator that positively
// rejects the payment is reporting on the caller's payment, so the caller is told
// to fix it. A transport or protocol failure says nothing about the payment and
// must stay retryable.
func (h *XNOHandler) verify(ctx context.Context, payment *x402PaymentPayload) (payer string, rejected bool, err error) {
	var resp x402VerifyResponse
	if err := h.call(ctx, "/verify", payment, &resp); err != nil {
		return "", false, err
	}
	if !resp.IsValid {
		reason := resp.InvalidReason
		if reason == "" {
			reason = "facilitator rejected the payment"
		}
		return "", true, errors.New(reason)
	}
	return resp.Payer, false, nil
}

// settleWith asks the facilitator to settle a verified payment on-chain.
func (h *XNOHandler) settleWith(ctx context.Context, payment *x402PaymentPayload) error {
	var resp x402SettleResponse
	if err := h.call(ctx, "/settle", payment, &resp); err != nil {
		return err
	}
	if !resp.Success {
		reason := resp.ErrorReason
		if reason == "" {
			reason = "facilitator reported settlement failure"
		}
		return errors.New(reason)
	}
	return nil
}

// call performs one facilitator request and decodes the JSON response into out.
func (h *XNOHandler) call(ctx context.Context, path string, payment *x402PaymentPayload, out any) error {
	body, err := json.Marshal(x402FacilitatorRequest{
		X402Version:         x402Version,
		PaymentPayload:      *payment,
		PaymentRequirements: h.terms,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal facilitator request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, h.facilitatorURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create facilitator request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	httpResp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("facilitator %s request failed: %w", path, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("failed to read facilitator %s response: %w", path, err)
	}
	if httpResp.StatusCode >= 400 {
		slog.Warn("facilitator returned error",
			"handler", h.Name(),
			"path", path,
			"status", httpResp.StatusCode,
			"body", truncate(string(respBody), 200),
		)
		return fmt.Errorf("facilitator %s returned HTTP %d", path, httpResp.StatusCode)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to parse facilitator %s response: %w", path, err)
	}
	return nil
}

// assetLabel renders the asset for humans, preferring the operator's label over
// the machine identifier.
func assetLabel(terms X402Terms) string {
	if terms.AssetName != "" {
		return terms.AssetName
	}
	if terms.Asset != "" {
		return terms.Asset
	}
	return "settlement units"
}
