import { useEffect, useState } from 'react'
import {
  X, Zap, Check, CreditCard, ExternalLink,
  AlertTriangle, ChevronRight, Loader, RefreshCw,
  Infinity, Users, Folder, History, Upload, Server, BarChart2, Sparkles
} from 'lucide-react'
import { useBillingStore } from '../../stores/billingStore'
import { useAuthStore } from '../../stores/authStore'
import { api, apiFetch } from '../../api/client'
import type { PlanConfig, PlanName, UsageSummary, Subscription } from '../../stores/billingStore'
import styles from './BillingPanel.module.css'

interface BillingPanelProps {
  onClose: () => void
}

export function BillingPanel({ onClose }: BillingPanelProps) {
  const { subscription, usage, currentPlan, plans, setSubscriptionData, setLoading, loading } = useBillingStore()
  const user = useAuthStore(s => s.user)
  const [interval, setInterval] = useState<'monthly' | 'yearly'>('monthly')
  const [checkoutLoading, setCheckoutLoading] = useState<PlanName | null>(null)
  const [trialLoading, setTrialLoading] = useState(false)
  const [trialSuccess, setTrialSuccess] = useState(false)
  const [portalLoading, setPortalLoading] = useState(false)
  const [tab, setTab] = useState<'plans' | 'usage'>('plans')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => { fetchBilling() }, [])

  async function fetchBilling() {
    setLoading(true)
    setError(null)
    try {
      const data = await api.billing.getSubscription()
      setSubscriptionData(data.subscription, data.usage, data.plan, data.plans)
    } catch (e: any) {
      setError(e.message || 'Failed to load billing info')
    } finally {
      setLoading(false)
    }
  }

  async function handleUpgrade(plan: PlanName) {
    if (plan === 'enterprise') {
      window.open('mailto:sales@platr.dev?subject=Enterprise%20Inquiry', '_blank')
      return
    }
    setCheckoutLoading(plan)
    setError(null)
    try {
      const { url } = await api.billing.createCheckout(plan, interval)
      window.location.href = url
    } catch (e: any) {
      setError(e.message || 'Failed to start checkout')
    } finally {
      setCheckoutLoading(null)
    }
  }

  async function handleTrial(plan: PlanName) {
    setTrialLoading(true)
    setError(null)
    try {
      const res = await apiFetch(`/billing/trial`, {
        method: 'POST',
        body: JSON.stringify({ plan }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || 'Failed to start trial')
      setTrialSuccess(true)
      // Store'u yenile
      const subRes = await apiFetch('/billing/subscription')
      const subData = await subRes.json()
      if (subRes.ok) setSubscriptionData(subData.subscription, subData.usage, subData.plan, subData.plans)
      setTimeout(() => setTrialSuccess(false), 4000)
    } catch (e: any) {
      setError(e.message || 'Failed to start trial')
    } finally {
      setTrialLoading(false)
    }
  }

  async function handleManage() {
    setPortalLoading(true)
    try {
      const { url } = await api.billing.createPortal()
      window.open(url, '_blank')
    } catch (e: any) {
      setError(e.message || 'Failed to open billing portal')
    } finally {
      setPortalLoading(false)
    }
  }

  const currentPlanName = subscription?.plan ?? 'free'
  const isActive = subscription?.status === 'active' || subscription?.status === 'trialing'

  return (
    <div className={styles.overlay} onClick={e => e.target === e.currentTarget && onClose()}>
      <div className={styles.panel}>

        {/* Header */}
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <Zap size={16} className={styles.headerIcon} />
            <span className={styles.headerTitle}>Plans & Billing</span>
            {subscription && (
              <span className={`${styles.planBadge} ${styles[`badge_${currentPlanName}`]}`}>
                {currentPlan?.displayName ?? currentPlanName}
              </span>
            )}
          </div>
          <div className={styles.headerActions}>
            {subscription?.stripeCustomerId && (
              <button
                className={styles.manageBtn}
                onClick={handleManage}
                disabled={portalLoading}
              >
                {portalLoading
                  ? <Loader size={12} className={styles.spin} />
                  : <CreditCard size={12} />
                }
                Manage billing
              </button>
            )}
            <button className={styles.refreshBtn} onClick={fetchBilling} disabled={loading}>
              <RefreshCw size={12} className={loading ? styles.spin : ''} />
            </button>
            <button className={styles.closeBtn} onClick={onClose}>
              <X size={15} />
            </button>
          </div>
        </div>

        {/* Status bar — past_due warning */}
        {subscription?.status === 'past_due' && (
          <div className={styles.alertBar}>
            <AlertTriangle size={13} />
            <span>Payment failed — your account will be downgraded soon. Please update your payment method.</span>
            <button className={styles.alertAction} onClick={handleManage}>Update card</button>
          </div>
        )}

        {subscription?.cancelAtPeriodEnd && (
          <div className={styles.warningBar}>
            <AlertTriangle size={13} />
            <span>
              Your plan will be canceled on {new Date(subscription.currentPeriodEnd).toLocaleDateString()}.
              Reactivate to keep access.
            </span>
            <button className={styles.alertAction} onClick={handleManage}>Reactivate</button>
          </div>
        )}

        {error && (
          <div className={styles.errorBar}>
            <AlertTriangle size={13} />
            <span>{error}</span>
            <button onClick={() => setError(null)}><X size={11} /></button>
          </div>
        )}

        {/* Tabs */}
        <div className={styles.tabs}>
          <button
            className={`${styles.tab} ${tab === 'plans' ? styles.tabActive : ''}`}
            onClick={() => setTab('plans')}
          >
            Plans
          </button>
          <button
            className={`${styles.tab} ${tab === 'usage' ? styles.tabActive : ''}`}
            onClick={() => setTab('usage')}
          >
            Usage
          </button>
        </div>

        {/* Plans tab */}
        {tab === 'plans' && (
          <div className={styles.plansContent}>

            {/* ── Stripe yoksa ücretsiz deneme banner'ı ── */}
            {currentPlanName === 'free' && (
              <div className={styles.trialBanner}>
                <div className={styles.trialBannerText}>
                  <Sparkles size={16} />
                  <span>Ödeme bilgisi gerekmeden <strong>30 gün Professional</strong> ücretsiz deneyin</span>
                </div>
                {trialSuccess ? (
                  <span className={styles.trialSuccessMsg}>✅ Professional plan aktif!</span>
                ) : (
                  <button
                    className={styles.trialBtn}
                    onClick={() => handleTrial('professional')}
                    disabled={trialLoading}
                  >
                    {trialLoading ? <Loader size={13} className={styles.spin} /> : null}
                    {trialLoading ? 'Başlatılıyor…' : 'Ücretsiz Dene'}
                  </button>
                )}
              </div>
            )}

            {/* Interval toggle */}
            <div className={styles.intervalToggle}>
              <button
                className={`${styles.intervalBtn} ${interval === 'monthly' ? styles.intervalActive : ''}`}
                onClick={() => setInterval('monthly')}
              >
                Monthly
              </button>
              <button
                className={`${styles.intervalBtn} ${interval === 'yearly' ? styles.intervalActive : ''}`}
                onClick={() => setInterval('yearly')}
              >
                Yearly
                <span className={styles.saveBadge}>Save 33%</span>
              </button>
            </div>

            {loading && !plans.length ? (
              <div className={styles.loadingState}>
                <Loader size={20} className={styles.spin} />
                <span>Loading plans…</span>
              </div>
            ) : (
              <div className={styles.plansGrid}>
                {(plans.length ? plans : FALLBACK_PLANS).map(plan => (
                  <PlanCard
                    key={plan.plan}
                    plan={plan}
                    interval={interval}
                    isCurrent={plan.plan === currentPlanName}
                    isActive={isActive}
                    loading={checkoutLoading === plan.plan}
                    onUpgrade={handleUpgrade}
                  />
                ))}
              </div>
            )}
          </div>
        )}

        {/* Usage tab */}
        {tab === 'usage' && (
          <div className={styles.usageContent}>
            {loading && !usage ? (
              <div className={styles.loadingState}>
                <Loader size={20} className={styles.spin} />
              </div>
            ) : usage ? (
              <UsagePanel usage={usage} plan={currentPlan} subscription={subscription} />
            ) : (
              <div className={styles.empty}>Usage data unavailable</div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// ── Plan Card ─────────────────────────────────────────────────────────────────

function PlanCard({
  plan, interval, isCurrent, isActive, loading, onUpgrade
}: {
  plan: PlanConfig
  interval: 'monthly' | 'yearly'
  isCurrent: boolean
  isActive: boolean
  loading: boolean
  onUpgrade: (plan: PlanName) => void
}) {
  const price = interval === 'yearly' ? plan.priceYearlyUSD : plan.priceMonthlyUSD
  const monthlyEquiv = interval === 'yearly' && plan.priceYearlyUSD > 0
    ? Math.round(plan.priceYearlyUSD / 12)
    : price

  const isEnterprise = plan.plan === 'enterprise'
  const isFree = plan.plan === 'free'
  const isDowngrade = planRank(plan.plan) < planRank(useCurrentPlanFromStore())

  function getButtonLabel() {
    if (isCurrent && isActive) return 'Current plan'
    if (isCurrent && !isActive) return 'Reactivate'
    if (isEnterprise) return 'Contact sales'
    if (isFree && !isActive) return 'Downgrade to Free'
    if (isDowngrade) return 'Downgrade'
    return 'Upgrade'
  }

  function getButtonVariant() {
    if (isCurrent && isActive) return styles.btnCurrent
    if (plan.highlighted) return styles.btnPrimary
    return styles.btnSecondary
  }

  return (
    <div className={`${styles.planCard} ${plan.highlighted ? styles.planCardHighlighted : ''} ${isCurrent ? styles.planCardCurrent : ''}`}>
      {plan.badge && (
        <div className={`${styles.planBadgeTop} ${plan.highlighted ? styles.planBadgePopular : ''}`}>
          {plan.badge}
        </div>
      )}

      <div className={styles.planCardHeader}>
        <div className={styles.planName}>{plan.displayName}</div>
        <div className={styles.planDescription}>{plan.description}</div>
      </div>

      <div className={styles.planPrice}>
        {isEnterprise ? (
          <span className={styles.priceCustom}>Custom</span>
        ) : isFree ? (
          <><span className={styles.priceAmount}>$0</span><span className={styles.pricePer}>/month</span></>
        ) : (
          <>
            <span className={styles.priceAmount}>${(monthlyEquiv / 100).toFixed(0)}</span>
            <span className={styles.pricePer}>/month</span>
            {interval === 'yearly' && price > 0 && (
              <span className={styles.priceBilled}>billed ${(price / 100).toFixed(0)}/yr</span>
            )}
          </>
        )}
      </div>

      <button
        className={`${styles.planBtn} ${getButtonVariant()}`}
        disabled={(isCurrent && isActive) || loading}
        onClick={() => !isCurrent && onUpgrade(plan.plan)}
      >
        {loading ? (
          <Loader size={13} className={styles.spin} />
        ) : (isCurrent && isActive) ? (
          <><Check size={13} /> {getButtonLabel()}</>
        ) : (
          <>{getButtonLabel()} {!isEnterprise && <ChevronRight size={13} />}</>
        )}
      </button>

      <div className={styles.planFeatures}>
        <FeatureList plan={plan} />
      </div>
    </div>
  )
}

function useCurrentPlanFromStore(): PlanName {
  return useBillingStore(s => s.subscription?.plan ?? 'free')
}

function planRank(p: PlanName): number {
  return { free: 0, basic: 1, professional: 2, enterprise: 3 }[p] ?? 0
}

function normalizeLimits(raw: any): PlanConfig['limits'] {
  if (!raw) return { invocationsPerMonth: 0, workspaces: 1, membersPerWorkspace: 1, collectionsPerWorkspace: 5, environmentsPerWorkspace: 2, historyRetentionDays: 7, protoUpload: true, k8sIntegration: false, exportImport: false, teamRoles: false, loadTesting: false, aiAssistant: false }
  // Backend may return PascalCase (no json tags) or camelCase
  return {
    invocationsPerMonth:      raw.invocationsPerMonth      ?? raw.InvocationsPerMonth      ?? 0,
    workspaces:               raw.workspaces               ?? raw.Workspaces               ?? 1,
    membersPerWorkspace:      raw.membersPerWorkspace      ?? raw.MembersPerWorkspace      ?? 1,
    collectionsPerWorkspace:  raw.collectionsPerWorkspace  ?? raw.CollectionsPerWorkspace  ?? 5,
    environmentsPerWorkspace: raw.environmentsPerWorkspace ?? raw.EnvironmentsPerWorkspace ?? 2,
    historyRetentionDays:     raw.historyRetentionDays     ?? raw.HistoryRetentionDays     ?? 7,
    protoUpload:    raw.protoUpload    ?? raw.ProtoUpload    ?? true,
    k8sIntegration: raw.k8sIntegration ?? raw.K8sIntegration ?? false,
    exportImport:   raw.exportImport   ?? raw.ExportImport   ?? false,
    teamRoles:      raw.teamRoles      ?? raw.TeamRoles      ?? false,
    loadTesting:    raw.loadTesting    ?? raw.LoadTesting    ?? false,
    aiAssistant:    raw.aiAssistant    ?? raw.AiAssistant    ?? false,
  }
}

function FeatureList({ plan }: { plan: PlanConfig }) {
  const l = normalizeLimits(plan?.limits)
  const items = [
    {
      icon: <Zap size={12} />,
      label: l.invocationsPerMonth === -1 ? 'Unlimited requests/month' : `${l.invocationsPerMonth.toLocaleString()} requests/month`,
      available: true
    },
    {
      icon: <Folder size={12} />,
      label: l.workspaces === -1 ? 'Unlimited workspaces' : `${l.workspaces} workspace${l.workspaces !== 1 ? 's' : ''}`,
      available: true
    },
    {
      icon: <Users size={12} />,
      label: l.membersPerWorkspace === -1 ? 'Unlimited team members' : `${l.membersPerWorkspace} member${l.membersPerWorkspace !== 1 ? 's' : ''} per workspace`,
      available: true
    },
    {
      icon: <History size={12} />,
      label: l.historyRetentionDays === -1 ? 'Unlimited history' : `${l.historyRetentionDays}-day history`,
      available: true
    },
    {
      icon: <Upload size={12} />,
      label: 'Proto file uploads',
      available: l.protoUpload
    },
    {
      icon: <Server size={12} />,
      label: 'Kubernetes integration',
      available: l.k8sIntegration
    },
    {
      icon: <ExternalLink size={12} />,
      label: 'Export & Import collections',
      available: l.exportImport
    },
    {
      icon: <Users size={12} />,
      label: 'Team roles & permissions',
      available: l.teamRoles
    },
    {
      icon: <BarChart2 size={12} />,
      label: 'Load testing (p50/p95/p99)',
      available: l.loadTesting
    },
    {
      icon: <Sparkles size={12} />,
      label: 'AI request assistant',
      available: l.aiAssistant
    },
  ]

  return (
    <ul className={styles.featureList}>
      {items.map((item) => (
        <li key={item.label} className={`${styles.featureItem} ${!item.available ? styles.featureDisabled : ''}`}>
          <span className={`${styles.featureIcon} ${item.available ? styles.featureIconOk : styles.featureIconOff}`}>
            {item.available ? <Check size={11} /> : <X size={11} />}
          </span>
          <span className={styles.featureLabel}>{item.label}</span>
        </li>
      ))}
    </ul>
  )
}

// ── Usage Panel ───────────────────────────────────────────────────────────────

function UsagePanel({ usage, plan, subscription }: {
  usage: UsageSummary
  plan: PlanConfig | null
  subscription: Subscription | null
}) {
  const invPct = usage.invocationsLimit === -1 ? 0
    : Math.min(100, Math.round((usage.invocationsUsed / usage.invocationsLimit) * 100))
  const wsPct = usage.workspacesLimit === -1 ? 0
    : Math.min(100, Math.round((usage.workspacesCount / usage.workspacesLimit) * 100))

  const periodEnd = subscription ? new Date(subscription.currentPeriodEnd) : null
  const periodStart = usage.periodStart ? new Date(usage.periodStart) : null

  return (
    <div className={styles.usageGrid}>
      {/* Period info */}
      {subscription && (
        <div className={styles.periodInfo}>
          <div className={styles.periodLabel}>Billing period</div>
          <div className={styles.periodValue}>
            {periodStart?.toLocaleDateString()} – {periodEnd?.toLocaleDateString()}
          </div>
        </div>
      )}

      {/* Invocations */}
      <div className={styles.usageCard}>
        <div className={styles.usageCardHeader}>
          <div className={styles.usageCardTitle}>
            <Zap size={14} className={styles.usageIcon} />
            API Requests
          </div>
          <div className={styles.usageCardValue}>
            <span className={styles.usageUsed}>{usage.invocationsUsed.toLocaleString()}</span>
            {usage.invocationsLimit !== -1 && (
              <span className={styles.usageLimit}> / {usage.invocationsLimit.toLocaleString()}</span>
            )}
            {usage.invocationsLimit === -1 && (
              <span className={styles.usageUnlimited}><Infinity size={13} /> Unlimited</span>
            )}
          </div>
        </div>
        {usage.invocationsLimit !== -1 && (
          <>
            <div className={styles.usageBar}>
              <div
                className={`${styles.usageBarFill} ${invPct > 85 ? styles.usageBarDanger : invPct > 60 ? styles.usageBarWarning : ''}`}
                style={{ width: `${invPct}%` }}
              />
            </div>
            {invPct > 85 && (
              <div className={styles.usageWarning}>
                <AlertTriangle size={11} />
                {invPct === 100
                  ? 'Limit reached — upgrade to continue making requests.'
                  : `${100 - invPct}% of monthly quota remaining.`
                }
              </div>
            )}
          </>
        )}
      </div>

      {/* Workspaces */}
      <div className={styles.usageCard}>
        <div className={styles.usageCardHeader}>
          <div className={styles.usageCardTitle}>
            <Folder size={14} className={styles.usageIcon} />
            Workspaces
          </div>
          <div className={styles.usageCardValue}>
            <span className={styles.usageUsed}>{usage.workspacesCount}</span>
            {usage.workspacesLimit !== -1 && (
              <span className={styles.usageLimit}> / {usage.workspacesLimit}</span>
            )}
            {usage.workspacesLimit === -1 && (
              <span className={styles.usageUnlimited}><Infinity size={13} /> Unlimited</span>
            )}
          </div>
        </div>
        {usage.workspacesLimit !== -1 && (
          <div className={styles.usageBar}>
            <div
              className={`${styles.usageBarFill} ${wsPct > 85 ? styles.usageBarDanger : ''}`}
              style={{ width: `${wsPct}%` }}
            />
          </div>
        )}
      </div>

      {/* Feature availability */}
      {plan && (
        <div className={styles.featureGrid}>
          <div className={styles.featureGridTitle}>Plan Features</div>
          {[
            { key: 'protoUpload', label: 'Proto uploads', icon: <Upload size={12} /> },
            { key: 'k8sIntegration', label: 'Kubernetes', icon: <Server size={12} /> },
            { key: 'exportImport', label: 'Export / Import', icon: <ExternalLink size={12} /> },
            { key: 'teamRoles', label: 'Team roles', icon: <Users size={12} /> },
          ].map(({ key, label, icon }) => {
            const limits = normalizeLimits(plan?.limits)
            const enabled = limits[key as keyof typeof limits] as boolean
            return (
              <div key={key} className={`${styles.featureGridItem} ${!enabled ? styles.featureGridItemOff : ''}`}>
                <span className={styles.featureGridIcon}>{icon}</span>
                <span>{label}</span>
                <span className={`${styles.featureGridStatus} ${enabled ? styles.featureGridOn : styles.featureGridOff}`}>
                  {enabled ? 'Active' : 'Upgrade'}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

// ── Fallback plans (shown before API responds) ────────────────────────────────
const FALLBACK_PLANS: PlanConfig[] = [
  {
    plan: 'free', displayName: 'Free', description: 'For solo developers',
    priceMonthlyUSD: 0, priceYearlyUSD: 0, stripePriceMonthly: '', stripePriceYearly: '',
    highlighted: false, badge: '',
    limits: { invocationsPerMonth: 100, workspaces: 1, membersPerWorkspace: 1, collectionsPerWorkspace: 5, environmentsPerWorkspace: 2, historyRetentionDays: 7, protoUpload: true, k8sIntegration: false, exportImport: false, teamRoles: false, loadTesting: false, aiAssistant: false }
  },
  {
    plan: 'basic', displayName: 'Basic', description: 'For small teams',
    priceMonthlyUSD: 1200, priceYearlyUSD: 9600, stripePriceMonthly: '', stripePriceYearly: '',
    highlighted: false, badge: '',
    limits: { invocationsPerMonth: 2000, workspaces: 3, membersPerWorkspace: 5, collectionsPerWorkspace: -1, environmentsPerWorkspace: 10, historyRetentionDays: 30, protoUpload: true, k8sIntegration: false, exportImport: true, teamRoles: true, loadTesting: true, aiAssistant: false }
  },
  {
    plan: 'professional', displayName: 'Professional', description: 'Unlimited power',
    priceMonthlyUSD: 3900, priceYearlyUSD: 31200, stripePriceMonthly: '', stripePriceYearly: '',
    highlighted: true, badge: 'Most Popular',
    limits: { invocationsPerMonth: -1, workspaces: -1, membersPerWorkspace: -1, collectionsPerWorkspace: -1, environmentsPerWorkspace: -1, historyRetentionDays: 90, protoUpload: true, k8sIntegration: true, exportImport: true, teamRoles: true, loadTesting: true, aiAssistant: true }
  },
  {
    plan: 'enterprise', displayName: 'Enterprise', description: 'Custom SLA & support',
    priceMonthlyUSD: 0, priceYearlyUSD: 0, stripePriceMonthly: '', stripePriceYearly: '',
    highlighted: false, badge: 'Custom Pricing',
    limits: { invocationsPerMonth: -1, workspaces: -1, membersPerWorkspace: -1, collectionsPerWorkspace: -1, environmentsPerWorkspace: -1, historyRetentionDays: -1, protoUpload: true, k8sIntegration: true, exportImport: true, teamRoles: true, loadTesting: true, aiAssistant: true }
  },
]
