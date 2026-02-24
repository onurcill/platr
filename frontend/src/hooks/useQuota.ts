/**
 * useQuota — intercepts 402 Payment Required responses and surfaces
 * an upgrade prompt globally. Any component can call `triggerUpgrade()`
 * to open the BillingPanel.
 */
import { create } from 'zustand'

interface QuotaState {
  showBilling: boolean
  quotaMessage: string | null
  quotaCode: string | null
  openBilling: (message?: string, code?: string) => void
  closeBilling: () => void
}

export const useQuotaStore = create<QuotaState>()((set) => ({
  showBilling: false,
  quotaMessage: null,
  quotaCode: null,
  openBilling: (message, code) =>
    set({ showBilling: true, quotaMessage: message ?? null, quotaCode: code ?? null }),
  closeBilling: () =>
    set({ showBilling: false, quotaMessage: null, quotaCode: null }),
}))

/**
 * parseQuotaError checks if an API response is a 402 quota error and
 * triggers the billing panel. Returns true if it was a quota error.
 */
export async function handleQuotaResponse(res: Response): Promise<boolean> {
  if (res.status !== 402) return false
  try {
    const data = await res.clone().json()
    useQuotaStore.getState().openBilling(
      data.error || 'Plan limit reached',
      data.quota?.code
    )
  } catch {
    useQuotaStore.getState().openBilling('Plan limit reached')
  }
  return true
}

export function useQuota() {
  const { openBilling } = useQuotaStore()
  return { triggerUpgrade: openBilling }
}
