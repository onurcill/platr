import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type PlanName = 'free' | 'basic' | 'professional' | 'enterprise'
export type SubStatus = 'active' | 'trialing' | 'past_due' | 'canceled' | 'incomplete'

export interface PlanLimits {
  invocationsPerMonth: number   // -1 = unlimited
  workspaces: number
  membersPerWorkspace: number
  collectionsPerWorkspace: number
  environmentsPerWorkspace: number
  historyRetentionDays: number
  protoUpload: boolean
  k8sIntegration: boolean
  exportImport: boolean
  teamRoles: boolean
  loadTesting: boolean
  aiAssistant: boolean
}

export interface PlanConfig {
  plan: PlanName
  displayName: string
  description: string
  priceMonthlyUSD: number  // cents
  priceYearlyUSD: number
  stripePriceMonthly: string
  stripePriceYearly: string
  limits: PlanLimits
  highlighted: boolean
  badge: string
}

export interface Subscription {
  id: string
  userId: string
  plan: PlanName
  status: SubStatus
  stripeCustomerId?: string
  stripeSubscriptionId?: string
  currentPeriodStart: string
  currentPeriodEnd: string
  cancelAtPeriodEnd: boolean
  createdAt: string
  updatedAt: string
}

export interface UsageSummary {
  userId: string
  periodStart: string
  periodEnd: string
  invocationsUsed: number
  invocationsLimit: number  // -1 = unlimited
  workspacesCount: number
  workspacesLimit: number
}

interface BillingStore {
  subscription: Subscription | null
  usage: UsageSummary | null
  plans: PlanConfig[]
  currentPlan: PlanConfig | null
  loading: boolean
  lastFetched: number | null

  setSubscriptionData: (sub: Subscription, usage: UsageSummary, plan: PlanConfig, plans: PlanConfig[]) => void
  setPlans: (plans: PlanConfig[]) => void
  setLoading: (v: boolean) => void
  clear: () => void

  // Computed helpers
  isActive: () => boolean
  isPaid: () => boolean
  canUseFeature: (feature: keyof PlanLimits) => boolean
  invocationsPercent: () => number
}

export const useBillingStore = create<BillingStore>()(
  persist(
    (set, get) => ({
      subscription: null,
      usage: null,
      plans: [],
      currentPlan: null,
      loading: false,
      lastFetched: null,

      setSubscriptionData: (sub, usage, plan, plans) =>
        set({ subscription: sub, usage, currentPlan: plan, plans, lastFetched: Date.now() }),

      setPlans: (plans) => set({ plans }),
      setLoading: (v) => set({ loading: v }),
      clear: () => set({ subscription: null, usage: null, currentPlan: null, lastFetched: null }),

      isActive: () => {
        const s = get().subscription
        return s?.status === 'active' || s?.status === 'trialing'
      },

      isPaid: () => {
        const s = get().subscription
        return !!(s && s.plan !== 'free' && (s.status === 'active' || s.status === 'trialing'))
      },

      canUseFeature: (feature) => {
        const plan = get().currentPlan
        if (!plan) return false
        const val = plan.limits[feature]
        if (typeof val === 'boolean') return val
        if (typeof val === 'number') return val === -1 || val > 0
        return true
      },

      invocationsPercent: () => {
        const u = get().usage
        if (!u || u.invocationsLimit === -1) return 0
        return Math.min(100, Math.round((u.invocationsUsed / u.invocationsLimit) * 100))
      },
    }),
    {
      name: 'grpc-inspector-billing',
      partialize: (s) => ({
        subscription: s.subscription,
        usage: s.usage,
        currentPlan: s.currentPlan,
        plans: s.plans,
        lastFetched: s.lastFetched,
      }),
    }
  )
)
