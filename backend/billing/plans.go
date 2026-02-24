package billing

import "time"

// Plan — subscription tier identifier
type Plan string

const (
	PlanFree         Plan = "free"
	PlanBasic        Plan = "basic"
	PlanProfessional Plan = "professional"
	PlanEnterprise   Plan = "enterprise"
)

// Status — subscription lifecycle state
type Status string

const (
	StatusActive   Status = "active"
	StatusTrialing Status = "trialing"
	StatusPastDue  Status = "past_due"
	StatusCanceled Status = "canceled"
)

// Limits defines what a plan is allowed to do. -1 = unlimited.
type Limits struct {
	InvocationsPerMonth      int  `json:"invocationsPerMonth"`
	Workspaces               int  `json:"workspaces"`
	MembersPerWorkspace      int  `json:"membersPerWorkspace"`
	CollectionsPerWorkspace  int  `json:"collectionsPerWorkspace"`
	EnvironmentsPerWorkspace int  `json:"environmentsPerWorkspace"`
	HistoryRetentionDays     int  `json:"historyRetentionDays"`
	ProtoUpload              bool `json:"protoUpload"`
	K8sIntegration           bool `json:"k8sIntegration"`
	ExportImport             bool `json:"exportImport"`
	TeamRoles                bool `json:"teamRoles"`
	LoadTesting              bool `json:"loadTesting"`
	AiAssistant              bool `json:"aiAssistant"`
}

type PlanConfig struct {
	Plan               Plan   `json:"plan"`
	DisplayName        string `json:"displayName"`
	Description        string `json:"description"`
	PriceMonthlyUSD    int    `json:"priceMonthlyUSD"`
	PriceYearlyUSD     int    `json:"priceYearlyUSD"`
	StripePriceMonthly string `json:"stripePriceMonthly"`
	StripePriceYearly  string `json:"stripePriceYearly"`
	Limits             Limits `json:"limits"`
	Highlighted        bool   `json:"highlighted"`
	Badge              string `json:"badge"`
}

var AllPlans = []PlanConfig{
	{
		Plan:            PlanFree,
		DisplayName:     "Free",
		Description:     "For solo developers exploring gRPC APIs",
		PriceMonthlyUSD: 0,
		PriceYearlyUSD:  0,
		Limits: Limits{
			InvocationsPerMonth: 100, Workspaces: 1, MembersPerWorkspace: 1,
			CollectionsPerWorkspace: 5, EnvironmentsPerWorkspace: 2,
			HistoryRetentionDays: 7, ProtoUpload: true, LoadTesting: false, AiAssistant: false,
		},
	},
	{
		Plan:               PlanBasic,
		DisplayName:        "Basic",
		Description:        "For small teams building production services",
		PriceMonthlyUSD:    1200,
		PriceYearlyUSD:     9600,
		StripePriceMonthly: "price_basic_monthly",
		StripePriceYearly:  "price_basic_yearly",
		Limits: Limits{
			InvocationsPerMonth: 2000, Workspaces: 3, MembersPerWorkspace: 5,
			CollectionsPerWorkspace: -1, EnvironmentsPerWorkspace: 10,
			HistoryRetentionDays: 30, ProtoUpload: true, ExportImport: true, TeamRoles: true, LoadTesting: true, AiAssistant: false,
		},
	},
	{
		Plan:               PlanProfessional,
		DisplayName:        "Professional",
		Description:        "Unlimited power for engineering teams",
		PriceMonthlyUSD:    3900,
		PriceYearlyUSD:     31200,
		StripePriceMonthly: "price_pro_monthly",
		StripePriceYearly:  "price_pro_yearly",
		Highlighted:        true,
		Badge:              "Most Popular",
		Limits: Limits{
			InvocationsPerMonth: -1, Workspaces: -1, MembersPerWorkspace: -1,
			CollectionsPerWorkspace: -1, EnvironmentsPerWorkspace: -1,
			HistoryRetentionDays: 90, ProtoUpload: true, K8sIntegration: true,
			ExportImport: true, TeamRoles: true, LoadTesting: true, AiAssistant: true,
		},
	},
	{
		Plan:        PlanEnterprise,
		DisplayName: "Enterprise",
		Description: "Custom SLA, SSO, audit logs and dedicated support",
		Badge:       "Custom Pricing",
		Limits: Limits{
			InvocationsPerMonth: -1, Workspaces: -1, MembersPerWorkspace: -1,
			CollectionsPerWorkspace: -1, EnvironmentsPerWorkspace: -1,
			HistoryRetentionDays: -1, ProtoUpload: true, K8sIntegration: true,
			ExportImport: true, TeamRoles: true, LoadTesting: true, AiAssistant: true,
		},
	},
}

func GetPlan(p Plan) PlanConfig {
	for _, cfg := range AllPlans {
		if cfg.Plan == p {
			return cfg
		}
	}
	return AllPlans[0]
}

type Subscription struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"userId"`
	Plan                 Plan      `json:"plan"`
	Status               Status    `json:"status"`
	StripeCustomerID     string    `json:"stripeCustomerId,omitempty"`
	StripeSubscriptionID string    `json:"stripeSubscriptionId,omitempty"`
	StripePriceID        string    `json:"stripePriceId,omitempty"`
	CurrentPeriodStart   time.Time `json:"currentPeriodStart"`
	CurrentPeriodEnd     time.Time `json:"currentPeriodEnd"`
	CancelAtPeriodEnd    bool      `json:"cancelAtPeriodEnd"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type UsageSummary struct {
	UserID           string `json:"userId"`
	PeriodStart      string `json:"periodStart"`
	PeriodEnd        string `json:"periodEnd"`
	InvocationsUsed  int    `json:"invocationsUsed"`
	InvocationsLimit int    `json:"invocationsLimit"`
	WorkspacesCount  int    `json:"workspacesCount"`
	WorkspacesLimit  int    `json:"workspacesLimit"`
}

func (s *Subscription) IsActive() bool {
	return s.Status == StatusActive || s.Status == StatusTrialing
}

func (s *Subscription) EffectivePlan() Plan {
	if s.IsActive() {
		return s.Plan
	}
	return PlanFree
}
