import { useState, useEffect, useRef, useCallback } from 'react'
import { ConnectionBar } from './components/ConnectionBar'
import { Sidebar } from './components/Sidebar'
import { TabBar } from './components/TabBar'
import { RequestBuilder } from './components/RequestBuilder'
import { ResponseViewer } from './components/ResponseViewer'
import { AuthScreen } from './components/AuthScreen'
import { InviteAccept } from './components/InviteAccept'
import { useAuthStore, apiFetch, useConnectionStore, useBillingStore } from './stores'
import { QuotaUpgradePrompt } from './components/QuotaUpgradePrompt'
import { useTabStore } from './stores/tabStore'
import styles from './App.module.css'

function getInviteToken(): string | null {
  const m = window.location.pathname.match(/^\/invite\/([a-f0-9]+)$/)
  return m ? m[1] : null
}

// Sidebar default / min / max widths (px) — 50px rail is always shown
const SIDEBAR_DEFAULT = 256
const SIDEBAR_MIN     = 160
const SIDEBAR_MAX     = 480

export default function App() {
  const token = useAuthStore(s => s.token)
  const { setAuth } = useAuthStore()
  const activeWorkspaceId = useAuthStore(s => s.activeWorkspaceId)
  const [authChecked, setAuthChecked] = useState(false)
  const [inviteToken, setInviteToken] = useState<string | null>(getInviteToken)
  const prevWsId = useRef<string | null>(null)

  // Sidebar panel width (excludes 50px rail)
  const [panelWidth, setPanelWidth]  = useState(SIDEBAR_DEFAULT)
  const [collapsed, setCollapsed]    = useState(false)
  const dragStartX = useRef(0)
  const dragStartW = useRef(0)

  const onResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    dragStartX.current = e.clientX
    dragStartW.current = panelWidth

    const onMove = (ev: MouseEvent) => {
      const delta = ev.clientX - dragStartX.current
      const next  = Math.max(SIDEBAR_MIN, Math.min(SIDEBAR_MAX, dragStartW.current + delta))
      setPanelWidth(next)
      if (next < SIDEBAR_MIN + 20) setCollapsed(true)
      else setCollapsed(false)
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [panelWidth])

  useEffect(() => {
    if (!token) { setAuthChecked(true); return }
    apiFetch('/api/auth/me')
      .then(r => r.json())
      .then(data => {
        if (data.user) {
          setAuth(token, data.user, data.workspaces ?? [])
          import('./api/client').then(({ api }) =>
            api.billing.getSubscription().then(b =>
              useBillingStore.getState().setSubscriptionData(b.subscription, b.usage, b.plan, b.plans)
            ).catch(() => {})
          )
        } else {
          useAuthStore.getState().logout()
        }
      })
      .catch(() => useAuthStore.getState().logout())
      .finally(() => setAuthChecked(true))
  }, [])

  useEffect(() => {
    if (token && inviteToken) {
      window.history.replaceState(null, '', '/')
      setInviteToken(null)
    }
  }, [token])

  useEffect(() => {
    if (!authChecked) return
    if (prevWsId.current && prevWsId.current !== activeWorkspaceId) {
      useTabStore.getState().clearAllTabs()
    }
    prevWsId.current = activeWorkspaceId
  }, [activeWorkspaceId, authChecked])

  if (!authChecked) {
    return (
      <div style={{ display:'flex', alignItems:'center', justifyContent:'center', height:'100vh', background:'var(--bg-base)', color:'var(--text-muted)', fontSize:13, gap:10 }}>
        <div style={{ width:14, height:14, border:'2px solid var(--accent)', borderTopColor:'transparent', borderRadius:'50%', animation:'spin 0.7s linear infinite' }} />
        Loading\u2026
      </div>
    )
  }

  if (inviteToken) {
    return (
      <InviteAccept
        token={inviteToken}
        onDone={() => { window.history.replaceState(null, '', '/'); setInviteToken(null) }}
      />
    )
  }

  if (!token) return <AuthScreen />

  return (
    <>
      <div className={styles.app}>
        <ConnectionBar />
        <div className={styles.workspace}>

          {/* Sidebar gets the panel width injected */}
          <Sidebar panelWidth={collapsed ? 0 : panelWidth} />

          {/* Drag handle between sidebar and content */}
          <div
            className={styles.sidebarResizer}
            onMouseDown={onResizeStart}
            title="Drag to resize sidebar"
          />

          {/* Main content */}
          <div className={styles.center}>
            <TabBar />
            <div className={styles.requestPane}><RequestBuilder /></div>
            <div className={styles.divider} />
            <div className={styles.responsePane}><ResponseViewer /></div>
          </div>

        </div>
      </div>
      <QuotaUpgradePrompt />
    </>
  )
}
