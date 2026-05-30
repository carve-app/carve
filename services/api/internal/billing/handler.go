package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tier names — must match values stored in subscriptions.tier.
const (
	TierFree    = "free"
	TierLearner = "learner"
	TierPro     = "pro"
)

type Handler struct {
	db            *pgxpool.Pool
	stripeKey     string // STRIPE_SECRET_KEY
	webhookSecret string // STRIPE_WEBHOOK_SECRET
	appBaseURL    string // e.g. https://app.carve.app — for redirect URLs
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{
		db:            db,
		stripeKey:     os.Getenv("STRIPE_SECRET_KEY"),
		webhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		appBaseURL:    os.Getenv("APP_BASE_URL"),
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── GET /v1/billing/subscription ─────────────────────────────────────────────

func (h *Handler) Subscription(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type subRow struct {
		Tier              string     `json:"tier"`
		Status            string     `json:"status"`
		CurrentPeriodEnd  *time.Time `json:"current_period_end"`
	}

	var sub subRow
	err := h.db.QueryRow(r.Context(),
		`SELECT tier, status, current_period_end
		 FROM subscriptions
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1`,
		claims.UserID,
	).Scan(&sub.Tier, &sub.Status, &sub.CurrentPeriodEnd)

	if err != nil {
		// No subscription row → user is on free tier.
		writeJSON(w, http.StatusOK, map[string]any{
			"tier":   TierFree,
			"status": "active",
		})
		return
	}

	writeJSON(w, http.StatusOK, sub)
}

// ── POST /v1/billing/checkout ─────────────────────────────────────────────────
// Creates a Stripe Checkout session and returns the checkout URL.
// Body: {"tier": "learner"|"pro"}

func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if h.stripeKey == "" {
		writeError(w, http.StatusServiceUnavailable, "billing not configured")
		return
	}

	var req struct {
		Tier string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	priceID := os.Getenv("STRIPE_PRICE_" + strings.ToUpper(req.Tier))
	if priceID == "" {
		writeError(w, http.StatusBadRequest, "unknown tier or price not configured")
		return
	}

	baseURL := h.appBaseURL
	if baseURL == "" {
		baseURL = "http://localhost:5173"
	}

	// Fetch user email for pre-filling Stripe checkout.
	var email string
	h.db.QueryRow(r.Context(), `SELECT email FROM users WHERE id = $1`, claims.UserID).Scan(&email)

	params := url.Values{}
	params.Set("mode", "subscription")
	params.Set("success_url", baseURL+"/settings?checkout=success")
	params.Set("cancel_url", baseURL+"/settings?checkout=cancel")
	params.Set("line_items[0][price]", priceID)
	params.Set("line_items[0][quantity]", "1")
	params.Set("client_reference_id", claims.UserID)
	if email != "" {
		params.Set("customer_email", email)
	}

	session, err := stripePost(h.stripeKey, "/v1/checkout/sessions", params)
	if err != nil {
		slog.Error("stripe checkout session", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create checkout session")
		return
	}

	checkoutURL, _ := session["url"].(string)
	writeJSON(w, http.StatusOK, map[string]string{"url": checkoutURL})
}

// ── POST /v1/billing/webhook ──────────────────────────────────────────────────
// Handles Stripe webhook events. This endpoint must be registered WITHOUT the
// auth middleware since Stripe does not send JWT tokens.

func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB max
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read body")
		return
	}

	if h.webhookSecret != "" {
		if !verifyStripeSignature(body, r.Header.Get("Stripe-Signature"), h.webhookSecret) {
			writeError(w, http.StatusUnauthorized, "invalid stripe signature")
			return
		}
	}

	var event struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := h.handleEvent(r, event.Type, event.Data); err != nil {
		slog.Error("billing webhook handler", "event_type", event.Type, "error", err)
		// Return 200 so Stripe doesn't retry for non-transient errors.
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleEvent(r *http.Request, eventType string, data map[string]any) error {
	obj, _ := data["object"].(map[string]any)
	if obj == nil {
		return nil
	}

	switch eventType {
	case "checkout.session.completed":
		return h.onCheckoutCompleted(r, obj)
	case "customer.subscription.updated":
		return h.onSubscriptionUpdated(r, obj)
	case "customer.subscription.deleted":
		return h.onSubscriptionDeleted(r, obj)
	}
	return nil
}

func (h *Handler) onCheckoutCompleted(r *http.Request, obj map[string]any) error {
	userID, _ := obj["client_reference_id"].(string)
	subID, _ := obj["subscription"].(string)
	if userID == "" || subID == "" {
		return nil
	}

	// Fetch subscription details from Stripe.
	sub, err := stripeGet(h.stripeKey, "/v1/subscriptions/"+subID)
	if err != nil {
		return fmt.Errorf("fetch subscription %s: %w", subID, err)
	}

	tier, status, periodEnd := extractSubFields(sub)
	return h.upsertSubscription(r, userID, subID, tier, status, periodEnd)
}

func (h *Handler) onSubscriptionUpdated(r *http.Request, obj map[string]any) error {
	subID, _ := obj["id"].(string)
	userID := h.userIDFromProviderSubID(r, subID)
	if userID == "" {
		return nil
	}
	tier, status, periodEnd := extractSubFields(obj)
	return h.upsertSubscription(r, userID, subID, tier, status, periodEnd)
}

func (h *Handler) onSubscriptionDeleted(r *http.Request, obj map[string]any) error {
	subID, _ := obj["id"].(string)
	_, err := h.db.Exec(r.Context(),
		`UPDATE subscriptions SET status = 'canceled', updated_at = now()
		 WHERE provider_sub_id = $1`,
		subID,
	)
	return err
}

func (h *Handler) upsertSubscription(r *http.Request, userID, subID, tier, status string, periodEnd *time.Time) error {
	id := auth.NewID()
	_, err := h.db.Exec(r.Context(),
		`INSERT INTO subscriptions
		    (id, user_id, tier, status, provider, provider_sub_id, current_period_end)
		 VALUES ($1,$2,$3,$4,'stripe',$5,$6)
		 ON CONFLICT (user_id) DO UPDATE SET
		   tier               = EXCLUDED.tier,
		   status             = EXCLUDED.status,
		   provider_sub_id    = EXCLUDED.provider_sub_id,
		   current_period_end = EXCLUDED.current_period_end,
		   updated_at         = now()`,
		id, userID, tier, status, subID, periodEnd,
	)
	if err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}

	// Log the raw event for audit / debugging.
	payload, _ := json.Marshal(map[string]any{
		"user_id": userID,
		"sub_id":  subID,
		"tier":    tier,
		"status":  status,
	})
	eventID := auth.NewID()
	h.db.Exec(r.Context(),
		`INSERT INTO billing_events (id, user_id, stripe_event_id, event_type, payload, processed_at)
		 VALUES ($1,$2,$3,'subscription.upserted',$4,now())
		 ON CONFLICT (stripe_event_id) DO NOTHING`,
		eventID, userID, subID, payload,
	)
	return nil
}

func (h *Handler) userIDFromProviderSubID(r *http.Request, subID string) string {
	var userID string
	h.db.QueryRow(r.Context(),
		`SELECT user_id FROM subscriptions WHERE provider_sub_id = $1`,
		subID,
	).Scan(&userID)
	return userID
}

// ── Stripe HTTP helpers ───────────────────────────────────────────────────────

var stripeClient = &http.Client{Timeout: 15 * time.Second}

func stripePost(apiKey, path string, params url.Values) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodPost, "https://api.stripe.com"+path, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(apiKey, "")

	resp, err := stripeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		msg, _ := result["error"].(map[string]any)
		if msg != nil {
			return nil, fmt.Errorf("stripe %d: %v", resp.StatusCode, msg["message"])
		}
		return nil, fmt.Errorf("stripe error %d", resp.StatusCode)
	}
	return result, nil
}

func stripeGet(apiKey, path string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.stripe.com"+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(apiKey, "")

	resp, err := stripeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// verifyStripeSignature implements Stripe's HMAC-SHA256 webhook signature check.
// https://docs.stripe.com/webhooks/signatures
func verifyStripeSignature(payload []byte, sigHeader, secret string) bool {
	// sigHeader format: "t=1492774577,v1=5257a869e7ecebeda32affa62cdca3fa51cad7e77a05bd539ba74dd4eef,v1=..."
	var timestamp string
	var signatures []string

	for _, part := range strings.Split(sigHeader, ",") {
		if strings.HasPrefix(part, "t=") {
			timestamp = strings.TrimPrefix(part, "t=")
		} else if strings.HasPrefix(part, "v1=") {
			signatures = append(signatures, strings.TrimPrefix(part, "v1="))
		}
	}
	if timestamp == "" || len(signatures) == 0 {
		return false
	}

	// Reject if timestamp is more than 5 minutes old.
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix()-ts > 300 {
		return false
	}

	signed := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range signatures {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return true
		}
	}
	return false
}

// extractSubFields pulls tier/status/period_end from a Stripe subscription object.
func extractSubFields(obj map[string]any) (tier, status string, periodEnd *time.Time) {
	status, _ = obj["status"].(string)
	if status == "" {
		status = "active"
	}

	// Map Stripe price metadata or product name to our tier names.
	// Convention: price metadata key "carve_tier" = "learner" | "pro".
	// Fallback: default to "learner".
	tier = TierLearner
	if items, ok := obj["items"].(map[string]any); ok {
		if dataArr, ok := items["data"].([]any); ok && len(dataArr) > 0 {
			if item, ok := dataArr[0].(map[string]any); ok {
				if price, ok := item["price"].(map[string]any); ok {
					if meta, ok := price["metadata"].(map[string]any); ok {
						if t, ok := meta["carve_tier"].(string); ok {
							tier = t
						}
					}
				}
			}
		}
	}

	if pEnd, ok := obj["current_period_end"].(float64); ok {
		t := time.Unix(int64(pEnd), 0).UTC()
		periodEnd = &t
	}
	return
}
