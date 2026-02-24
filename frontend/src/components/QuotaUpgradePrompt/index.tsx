import { Zap, X, AlertTriangle } from 'lucide-react'
import { useQuotaStore } from '../../hooks/useQuota'
import { useBillingStore } from '../../stores/billingStore'
import { BillingPanel } from '../BillingPanel'
import styles from './QuotaUpgradePrompt.module.css'

/**
 * QuotaUpgradePrompt — rendered at the app root.
 * Shows a toast when a quota limit is hit, with a button to open BillingPanel.
 */
export function QuotaUpgradePrompt() {
  const { showBilling, quotaMessage, closeBilling, openBilling } = useQuotaStore()
  const currentPlan = useBillingStore(s => s.currentPlan)

  if (!showBilling && !quotaMessage) return null

  // When showBilling is true (user clicked upgrade), show the full panel
  if (showBilling) {
    return <BillingPanel onClose={closeBilling} />
  }

  // Otherwise show the toast
  return (
    <div className={styles.toast}>
      <AlertTriangle size={14} className={styles.toastIcon} />
      <div className={styles.toastBody}>
        <div className={styles.toastTitle}>Plan limit reached</div>
        <div className={styles.toastMsg}>{quotaMessage}</div>
      </div>
      <button
        className={styles.upgradeBtn}
        onClick={() => openBilling(quotaMessage ?? undefined)}
      >
        <Zap size={12} />
        Upgrade
      </button>
      <button className={styles.dismissBtn} onClick={closeBilling}>
        <X size={12} />
      </button>
    </div>
  )
}
