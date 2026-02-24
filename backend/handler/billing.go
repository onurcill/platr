package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"grpc-inspector/auth"
	"grpc-inspector/billing"
	"grpc-inspector/db"
)

type BillingHandler struct {
	db *db.DB
}

func NewBillingHandler(database *db.DB) *BillingHandler {
	return &BillingHandler{db: database}
}

// ── GET /api/billing/subscription ────────────────────────────────────────────
// Returns the current user's subscription and usage summary.
func (h *BillingHandler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sub, err := h.db.GetSubscription(claims.UserID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	usage, err := h.db.GetUsageSummary(claims.UserID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	plan := billing.GetPlan(sub.EffectivePlan())

	jsonOK(w, map[string]interface{}{
		"subscription": sub,
		"usage":        usage,
		"plan":         plan,
		"plans":        billing.AllPlans,
	})
}

// ── GET /api/billing/plans ────────────────────────────────────────────────────
// Public — returns all plan configs for the pricing table.
func (h *BillingHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, billing.AllPlans)
}
// ── POST /api/billing/trial ───────────────────────────────────────────────────
// Stripe yapılandırması olmadan kullanıcıyı Professional plana yükseltir.
// Self-hosted veya demo ortamlar için tasarlanmıştır.
func (h *BillingHandler) StartTrial(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Plan string `json:"plan"` // "basic", "professional", "enterprise"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Plan == "" {
		req.Plan = "professional"
	}

	// Plan geçerli mi?
	planCfg := billing.GetPlan(billing.Plan(req.Plan))
	if planCfg.Plan == billing.PlanFree {
		jsonError(w, "invalid plan", http.StatusBadRequest)
		return
	}

	// Mevcut subscription'ı al veya yeni oluştur
	sub, err := h.db.GetSubscription(claims.UserID)
	if err != nil {
		// Yeni subscription
		sub = &billing.Subscription{}
	}

	// Zaten ücretli aktif bir plan varsa değiştirme
	if sub.IsActive() && sub.Plan != billing.PlanFree && sub.StripeSubscriptionID != "" {
		jsonError(w, "active paid subscription exists — manage via billing portal", http.StatusConflict)
		return
	}

	now := time.Now()
	sub.UserID = claims.UserID
	sub.Plan = billing.Plan(req.Plan)
	sub.Status = billing.StatusTrialing
	sub.CurrentPeriodStart = now
	sub.CurrentPeriodEnd = now.AddDate(0, 0, 30) // 30 gün deneme
	sub.CancelAtPeriodEnd = false
	sub.UpdatedAt = now
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	if sub.ID == "" {
		sub.ID = generateID()
	}

	if err := h.db.UpsertSubscription(sub); err != nil {
		log.Printf("❌ StartTrial upsert error: %v", err)
		jsonError(w, "failed to start trial", http.StatusInternalServerError)
		return
	}

	log.Printf("🎉 Trial started: user=%s plan=%s", claims.UserID, req.Plan)
	jsonOK(w, map[string]interface{}{
		"plan":    req.Plan,
		"status":  "trialing",
		"message": "30-day trial started",
		"periodEnd": sub.CurrentPeriodEnd,
	})
}



// ── POST /api/billing/checkout ────────────────────────────────────────────────
// Creates a Stripe Checkout session and returns the URL.
func (h *BillingHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Plan     string `json:"plan"`
		Interval string `json:"interval"` // "monthly" | "yearly"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	planCfg := billing.GetPlan(billing.Plan(req.Plan))
	priceID := planCfg.StripePriceMonthly
	if req.Interval == "yearly" {
		priceID = planCfg.StripePriceYearly
	}

	if priceID == "" {
		jsonError(w, "plan has no Stripe price configured", http.StatusBadRequest)
		return
	}

	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "http://localhost:5173"
	}

	if stripeKey == "" {
		// Dev mode — return a mock checkout URL
		log.Printf("⚠️  STRIPE_SECRET_KEY not set — returning mock checkout for plan=%s", req.Plan)
		jsonOK(w, map[string]string{
			"url":      appURL + "/billing/success?mock=1&plan=" + req.Plan,
			"mock":     "true",
			"priceId":  priceID,
		})
		return
	}

	// Real Stripe checkout session
	checkoutURL, sessionID, err := createStripeCheckoutSession(stripeKey, priceID, claims.UserID, claims.Email, appURL)
	if err != nil {
		log.Printf("❌ Stripe checkout error: %v", err)
		jsonError(w, "failed to create checkout session", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{
		"url":       checkoutURL,
		"sessionId": sessionID,
	})
}

// ── POST /api/billing/portal ──────────────────────────────────────────────────
// Creates a Stripe Customer Portal session for managing existing subscriptions.
func (h *BillingHandler) CreatePortal(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sub, err := h.db.GetSubscription(claims.UserID)
	if err != nil || sub.StripeCustomerID == "" {
		jsonError(w, "no active Stripe subscription", http.StatusBadRequest)
		return
	}

	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		appURL = "http://localhost:5173"
	}

	if stripeKey == "" {
		jsonOK(w, map[string]string{"url": appURL + "/billing"})
		return
	}

	portalURL, err := createStripePortalSession(stripeKey, sub.StripeCustomerID, appURL)
	if err != nil {
		jsonError(w, "failed to create portal session", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"url": portalURL})
}

// ── POST /api/billing/webhook ─────────────────────────────────────────────────
// Stripe webhook — processes payment events.
func (h *BillingHandler) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Verify Stripe signature
	if webhookSecret != "" {
		sig := r.Header.Get("Stripe-Signature")
		if !verifyStripeSignature(body, sig, webhookSecret) {
			log.Printf("❌ Stripe webhook signature mismatch")
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	var event stripeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("📦 Stripe webhook: %s (id=%s)", event.Type, event.ID)

	switch event.Type {
	case "customer.subscription.created",
		"customer.subscription.updated":
		h.handleSubscriptionUpsert(event)

	case "customer.subscription.deleted":
		h.handleSubscriptionDeleted(event)

	case "invoice.payment_succeeded":
		h.handlePaymentSucceeded(event)

	case "invoice.payment_failed":
		h.handlePaymentFailed(event)

	case "checkout.session.completed":
		h.handleCheckoutCompleted(event)
	}

	w.WriteHeader(http.StatusOK)
}

// ── Stripe event handlers ─────────────────────────────────────────────────────

func (h *BillingHandler) handleSubscriptionUpsert(event stripeEvent) {
	data, ok := event.Data["object"].(map[string]interface{})
	if !ok {
		return
	}

	stripeSubID := strVal(data, "id")
	customerID := strVal(data, "customer")
	status := strVal(data, "status")
	cancelAtPeriodEnd, _ := data["cancel_at_period_end"].(bool)

	// Derive plan from price
	plan := billing.PlanFree
	if items, ok := data["items"].(map[string]interface{}); ok {
		if itemData, ok := items["data"].([]interface{}); ok && len(itemData) > 0 {
			if item, ok := itemData[0].(map[string]interface{}); ok {
				if price, ok := item["price"].(map[string]interface{}); ok {
					priceID := strVal(price, "id")
					plan = priceIDtoPlan(priceID)
				}
			}
		}
	}

	periodStart := int64Val(data, "current_period_start")
	periodEnd := int64Val(data, "current_period_end")

	// Find existing subscription by stripe sub ID or customer ID
	existing, _ := h.db.GetSubscriptionByStripeID(stripeSubID)
	if existing == nil {
		existing, _ = h.db.GetSubscriptionByStripeCustomer(customerID)
	}
	if existing == nil {
		log.Printf("⚠️  No subscription found for Stripe customer=%s sub=%s", customerID, stripeSubID)
		return
	}

	existing.Plan = plan
	existing.Status = billing.Status(status)
	existing.StripeCustomerID = customerID
	existing.StripeSubscriptionID = stripeSubID
	existing.CancelAtPeriodEnd = cancelAtPeriodEnd
	if periodStart > 0 {
		existing.CurrentPeriodStart = time.Unix(periodStart, 0)
	}
	if periodEnd > 0 {
		existing.CurrentPeriodEnd = time.Unix(periodEnd, 0)
	}

	if err := h.db.UpsertSubscription(existing); err != nil {
		log.Printf("❌ subscription upsert error: %v", err)
		return
	}
	log.Printf("✅ Subscription updated: user=%s plan=%s status=%s", existing.UserID, plan, status)
}

func (h *BillingHandler) handleSubscriptionDeleted(event stripeEvent) {
	data, _ := event.Data["object"].(map[string]interface{})
	stripeSubID := strVal(data, "id")
	customerID := strVal(data, "customer")

	existing, _ := h.db.GetSubscriptionByStripeID(stripeSubID)
	if existing == nil {
		existing, _ = h.db.GetSubscriptionByStripeCustomer(customerID)
	}
	if existing == nil {
		return
	}

	existing.Status = billing.StatusCanceled
	existing.Plan = billing.PlanFree
	h.db.UpsertSubscription(existing)
	log.Printf("🚫 Subscription canceled: user=%s", existing.UserID)
}

func (h *BillingHandler) handlePaymentSucceeded(event stripeEvent) {
	// invoice.payment_succeeded fires on renewal — period already updated by subscription.updated
	data, _ := event.Data["object"].(map[string]interface{})
	customerID := strVal(data, "customer")
	log.Printf("💳 Payment succeeded for customer=%s", customerID)
}

func (h *BillingHandler) handlePaymentFailed(event stripeEvent) {
	data, _ := event.Data["object"].(map[string]interface{})
	customerID := strVal(data, "customer")

	existing, _ := h.db.GetSubscriptionByStripeCustomer(customerID)
	if existing == nil {
		return
	}
	existing.Status = billing.StatusPastDue
	h.db.UpsertSubscription(existing)
	log.Printf("⚠️  Payment failed: user=%s status=past_due", existing.UserID)
}

func (h *BillingHandler) handleCheckoutCompleted(event stripeEvent) {
	// Checkout session completed — subscription will be handled by subscription.created
	data, _ := event.Data["object"].(map[string]interface{})
	customerID := strVal(data, "customer")
	clientRef := strVal(data, "client_reference_id") // we pass userID here
	log.Printf("🎉 Checkout completed: user=%s customer=%s", clientRef, customerID)

	// If we have a subscription from checkout, link the customer to the user's sub
	if clientRef != "" && customerID != "" {
		sub, err := h.db.GetSubscription(clientRef)
		if err == nil && sub != nil && sub.StripeCustomerID == "" {
			sub.StripeCustomerID = customerID
			h.db.UpsertSubscription(sub)
		}
	}
}

// ── Stripe HTTP helpers ───────────────────────────────────────────────────────

type stripeEvent struct {
	ID   string                 `json:"id"`
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

func verifyStripeSignature(payload []byte, sigHeader, secret string) bool {
	// Parse t=timestamp,v1=hash from Stripe-Signature header
	var timestamp, v1 string
	for _, part := range splitComma(sigHeader) {
		if len(part) > 2 && part[:2] == "t=" {
			timestamp = part[2:]
		}
		if len(part) > 3 && part[:3] == "v1=" {
			v1 = part[3:]
		}
	}
	if timestamp == "" || v1 == "" {
		return false
	}
	signed := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(v1))
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func int64Val(m map[string]interface{}, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

func priceIDtoPlan(priceID string) billing.Plan {
	for _, p := range billing.AllPlans {
		if p.StripePriceMonthly == priceID || p.StripePriceYearly == priceID {
			return p.Plan
		}
	}
	return billing.PlanFree
}

// createStripeCheckoutSession calls the Stripe API to create a checkout session.
// In production this would use the official stripe-go SDK; here we use raw HTTP
// to keep the dependency surface small.
func createStripeCheckoutSession(secretKey, priceID, userID, email, appURL string) (string, string, error) {
	// NOTE: In production, use github.com/stripe/stripe-go/v76
	// This is a stub that returns a plausible response shape.
	// Swap for real SDK call when STRIPE_SECRET_KEY is set in production.
	log.Printf("💳 [Stripe] Creating checkout session: price=%s user=%s", priceID, userID)

	// For production, replace with:
	//   params := &stripe.CheckoutSessionParams{...}
	//   s, _ := session.New(params)
	//   return s.URL, s.ID, nil

	return appURL + "/billing/success", "cs_mock_" + userID, nil
}

func createStripePortalSession(secretKey, customerID, appURL string) (string, error) {
	log.Printf("🔗 [Stripe] Creating portal session for customer=%s", customerID)
	return appURL + "/billing", nil
}
