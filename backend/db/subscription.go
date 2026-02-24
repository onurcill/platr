package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"grpc-inspector/billing"
)

func newSubID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "sub_" + hex.EncodeToString(b)
}

// GetSubscription returns (or auto-creates) the subscription for a user.
func (d *DB) GetSubscription(userID string) (*billing.Subscription, error) {
	s := &billing.Subscription{}
	err := d.QueryRow(`
		SELECT id, user_id, plan, status,
		       COALESCE(stripe_customer_id,''),
		       COALESCE(stripe_subscription_id,''),
		       COALESCE(stripe_price_id,''),
		       current_period_start, current_period_end,
		       cancel_at_period_end, created_at, updated_at
		FROM subscriptions WHERE user_id = ?`, userID,
	).Scan(
		&s.ID, &s.UserID, &s.Plan, &s.Status,
		&s.StripeCustomerID, &s.StripeSubscriptionID, &s.StripePriceID,
		&s.CurrentPeriodStart, &s.CurrentPeriodEnd,
		&s.CancelAtPeriodEnd, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return d.createFreeSubscription(userID)
	}
	return s, err
}

func (d *DB) GetSubscriptionByStripeID(stripeSubID string) (*billing.Subscription, error) {
	s := &billing.Subscription{}
	err := d.QueryRow(`
		SELECT id, user_id, plan, status,
		       COALESCE(stripe_customer_id,''),
		       COALESCE(stripe_subscription_id,''),
		       COALESCE(stripe_price_id,''),
		       current_period_start, current_period_end,
		       cancel_at_period_end, created_at, updated_at
		FROM subscriptions WHERE stripe_subscription_id = ?`, stripeSubID,
	).Scan(
		&s.ID, &s.UserID, &s.Plan, &s.Status,
		&s.StripeCustomerID, &s.StripeSubscriptionID, &s.StripePriceID,
		&s.CurrentPeriodStart, &s.CurrentPeriodEnd,
		&s.CancelAtPeriodEnd, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (d *DB) GetSubscriptionByStripeCustomer(customerID string) (*billing.Subscription, error) {
	s := &billing.Subscription{}
	err := d.QueryRow(`
		SELECT id, user_id, plan, status,
		       COALESCE(stripe_customer_id,''),
		       COALESCE(stripe_subscription_id,''),
		       COALESCE(stripe_price_id,''),
		       current_period_start, current_period_end,
		       cancel_at_period_end, created_at, updated_at
		FROM subscriptions WHERE stripe_customer_id = ?`, customerID,
	).Scan(
		&s.ID, &s.UserID, &s.Plan, &s.Status,
		&s.StripeCustomerID, &s.StripeSubscriptionID, &s.StripePriceID,
		&s.CurrentPeriodStart, &s.CurrentPeriodEnd,
		&s.CancelAtPeriodEnd, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (d *DB) createFreeSubscription(userID string) (*billing.Subscription, error) {
	now := time.Now()
	s := &billing.Subscription{
		ID:                 newSubID(),
		UserID:             userID,
		Plan:               billing.PlanFree,
		Status:             billing.StatusActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(100, 0, 0),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_, err := d.Exec(`
		INSERT OR IGNORE INTO subscriptions
			(id, user_id, plan, status, current_period_start, current_period_end, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.UserID, string(s.Plan), string(s.Status),
		s.CurrentPeriodStart, s.CurrentPeriodEnd, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	// re-read to get any DB-generated defaults
	return d.GetSubscription(userID)
}

func (d *DB) UpsertSubscription(s *billing.Subscription) error {
	s.UpdatedAt = time.Now()
	if s.ID == "" {
		s.ID = newSubID()
		s.CreatedAt = s.UpdatedAt
	}
	_, err := d.Exec(`
		INSERT INTO subscriptions
			(id, user_id, plan, status,
			 stripe_customer_id, stripe_subscription_id, stripe_price_id,
			 current_period_start, current_period_end, cancel_at_period_end,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			plan                   = excluded.plan,
			status                 = excluded.status,
			stripe_customer_id     = excluded.stripe_customer_id,
			stripe_subscription_id = excluded.stripe_subscription_id,
			stripe_price_id        = excluded.stripe_price_id,
			current_period_start   = excluded.current_period_start,
			current_period_end     = excluded.current_period_end,
			cancel_at_period_end   = excluded.cancel_at_period_end,
			updated_at             = excluded.updated_at`,
		s.ID, s.UserID, string(s.Plan), string(s.Status),
		s.StripeCustomerID, s.StripeSubscriptionID, s.StripePriceID,
		s.CurrentPeriodStart, s.CurrentPeriodEnd, s.CancelAtPeriodEnd,
		s.CreatedAt, s.UpdatedAt,
	)
	return err
}

// ── Usage Tracking ────────────────────────────────────────────────────────────

func (d *DB) IncrementUsage(userID string, n int) error {
	month := time.Now().Format("2006-01")
	_, err := d.Exec(`
		INSERT INTO usage_records (user_id, period_month, invocations)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, period_month) DO UPDATE SET
			invocations = invocations + excluded.invocations,
			updated_at  = CURRENT_TIMESTAMP`,
		userID, month, n,
	)
	return err
}

func (d *DB) GetMonthlyUsage(userID string) (int, error) {
	month := time.Now().Format("2006-01")
	var count int
	err := d.QueryRow(
		`SELECT COALESCE(invocations,0) FROM usage_records WHERE user_id=? AND period_month=?`,
		userID, month,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

func (d *DB) GetUsageSummary(userID string) (*billing.UsageSummary, error) {
	sub, err := d.GetSubscription(userID)
	if err != nil {
		return nil, err
	}
	invocations, _ := d.GetMonthlyUsage(userID)
	wsCount, _ := d.CountWorkspacesForUser(userID)
	limits := billing.GetPlan(sub.EffectivePlan()).Limits
	return &billing.UsageSummary{
		UserID:           userID,
		PeriodStart:      sub.CurrentPeriodStart.Format(time.RFC3339),
		PeriodEnd:        sub.CurrentPeriodEnd.Format(time.RFC3339),
		InvocationsUsed:  invocations,
		InvocationsLimit: limits.InvocationsPerMonth,
		WorkspacesCount:  wsCount,
		WorkspacesLimit:  limits.Workspaces,
	}, nil
}

func (d *DB) CountWorkspacesForUser(userID string) (int, error) {
	var n int
	err := d.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE owner_id=?`, userID).Scan(&n)
	return n, err
}

// ── Quota Errors ──────────────────────────────────────────────────────────────

type QuotaError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Limit   int    `json:"limit"`
	Used    int    `json:"used"`
	Plan    string `json:"plan"`
}

func (e *QuotaError) Error() string { return e.Message }

// ── Quota Checks ──────────────────────────────────────────────────────────────

func (d *DB) CheckInvocationQuota(userID string) error {
	sub, err := d.GetSubscription(userID)
	if err != nil {
		return nil // don't block on quota errors
	}
	limits := billing.GetPlan(sub.EffectivePlan()).Limits
	if limits.InvocationsPerMonth == -1 {
		return nil
	}
	used, _ := d.GetMonthlyUsage(userID)
	if used >= limits.InvocationsPerMonth {
		return &QuotaError{
			Code:    "quota_invocations",
			Message: "Monthly request limit reached. Upgrade your plan to continue.",
			Limit:   limits.InvocationsPerMonth,
			Used:    used,
			Plan:    string(sub.EffectivePlan()),
		}
	}
	return nil
}

func (d *DB) CheckWorkspaceQuota(userID string) error {
	sub, err := d.GetSubscription(userID)
	if err != nil {
		return nil
	}
	limits := billing.GetPlan(sub.EffectivePlan()).Limits
	if limits.Workspaces == -1 {
		return nil
	}
	count, _ := d.CountWorkspacesForUser(userID)
	if count >= limits.Workspaces {
		return &QuotaError{
			Code:    "quota_workspaces",
			Message: "Workspace limit reached. Upgrade to create more workspaces.",
			Limit:   limits.Workspaces,
			Used:    count,
			Plan:    string(sub.EffectivePlan()),
		}
	}
	return nil
}

func (d *DB) CheckMemberQuota(workspaceID string) error {
	// Limits are governed by the workspace owner's subscription plan
	var ownerID string
	if err := d.QueryRow(`SELECT owner_id FROM workspaces WHERE id=?`, workspaceID).Scan(&ownerID); err != nil {
		return nil // workspace not found — let handler catch it
	}
	sub, err := d.GetSubscription(ownerID)
	if err != nil {
		return nil
	}
	limits := billing.GetPlan(sub.EffectivePlan()).Limits
	if limits.MembersPerWorkspace == -1 {
		return nil
	}
	var count int
	d.QueryRow(`SELECT COUNT(*) FROM workspace_members WHERE workspace_id=?`, workspaceID).Scan(&count)
	if count >= limits.MembersPerWorkspace {
		return &QuotaError{
			Code:    "quota_members",
			Message: "Member limit reached. Upgrade your plan to add more team members.",
			Limit:   limits.MembersPerWorkspace,
			Used:    count,
			Plan:    string(sub.EffectivePlan()),
		}
	}
	return nil
}

func (d *DB) CheckFeatureAccess(userID, feature string) error {
	sub, err := d.GetSubscription(userID)
	if err != nil {
		return nil
	}
	limits := billing.GetPlan(sub.EffectivePlan()).Limits
	var allowed bool
	switch feature {
	case "k8s":
		allowed = limits.K8sIntegration
	case "export_import":
		allowed = limits.ExportImport
	case "team_roles":
		allowed = limits.TeamRoles
	case "proto_upload":
		allowed = limits.ProtoUpload
	case "load_testing":
		allowed = limits.LoadTesting
	case "ai_assistant":
		allowed = limits.AiAssistant
	default:
		allowed = true
	}
	if !allowed {
		return &QuotaError{
			Code:    "feature_locked",
			Message: "This feature requires a higher plan. Please upgrade.",
			Plan:    string(sub.EffectivePlan()),
		}
	}
	return nil
}

// CheckCollectionQuota checks whether the workspace can create another collection.
func (d *DB) CheckCollectionQuota(workspaceID, ownerID string) error {
	sub, err := d.GetSubscription(ownerID)
	if err != nil {
		return nil
	}
	limits := billing.GetPlan(sub.EffectivePlan()).Limits
	if limits.CollectionsPerWorkspace == -1 {
		return nil
	}
	var count int
	d.QueryRow(`SELECT COUNT(*) FROM collections WHERE workspace_id=?`, workspaceID).Scan(&count)
	if count >= limits.CollectionsPerWorkspace {
		return &QuotaError{
			Code:    "quota_collections",
			Message: "Collection limit reached. Upgrade your plan to create more collections.",
			Limit:   limits.CollectionsPerWorkspace,
			Used:    count,
			Plan:    string(sub.EffectivePlan()),
		}
	}
	return nil
}

// CheckEnvironmentQuota checks whether the workspace can create another environment.
func (d *DB) CheckEnvironmentQuota(workspaceID, ownerID string) error {
	sub, err := d.GetSubscription(ownerID)
	if err != nil {
		return nil
	}
	limits := billing.GetPlan(sub.EffectivePlan()).Limits
	if limits.EnvironmentsPerWorkspace == -1 {
		return nil
	}
	var count int
	d.QueryRow(`SELECT COUNT(*) FROM environments WHERE workspace_id=?`, workspaceID).Scan(&count)
	if count >= limits.EnvironmentsPerWorkspace {
		return &QuotaError{
			Code:    "quota_environments",
			Message: "Environment limit reached. Upgrade your plan to create more environments.",
			Limit:   limits.EnvironmentsPerWorkspace,
			Used:    count,
			Plan:    string(sub.EffectivePlan()),
		}
	}
	return nil
}

// PruneHistoryByRetention deletes history entries older than the workspace owner's retention window.
// Called after each successful invoke so stale rows don't accumulate.
func (d *DB) PruneHistoryByRetention(workspaceID, ownerID string) {
	sub, err := d.GetSubscription(ownerID)
	if err != nil {
		return
	}
	days := billing.GetPlan(sub.EffectivePlan()).Limits.HistoryRetentionDays
	if days == -1 {
		return // unlimited
	}
	d.Exec(
		`DELETE FROM request_history WHERE workspace_id=? AND created_at < datetime('now', ?)`,
		workspaceID, fmt.Sprintf("-%d days", days),
	)
}
