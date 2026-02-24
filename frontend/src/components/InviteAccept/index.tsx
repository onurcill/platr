import { useEffect, useState } from 'react'
import { Zap, CheckCircle, XCircle, Loader } from 'lucide-react'
import { useAuthStore, apiFetch } from '../../stores'
import { AuthScreen } from '../AuthScreen'
import styles from './InviteAccept.module.css'

interface Props {
  token: string
  onDone: () => void
}

export function InviteAccept({ token, onDone }: Props) {
  const { isAuthenticated, upsertWorkspace, setActiveWorkspace, token: authToken } = useAuthStore()
  const [state, setState] = useState<'loading' | 'needsAuth' | 'accepting' | 'success' | 'error'>('loading')
  const [inviteInfo, setInviteInfo] = useState<{ email: string; role: string; workspace?: string } | null>(null)
  const [error, setError] = useState('')

  useEffect(() => { checkInvite() }, [])

  // When user logs in (authToken changes), re-try accepting
  useEffect(() => {
    if (isAuthenticated() && state === 'needsAuth') {
      acceptInvite()
    }
  }, [authToken])

  async function checkInvite() {
    try {
      const res  = await apiFetch(`/api/invites/${token}/accept`, { method: 'POST' })
      const data = await res.json()

      if (!res.ok) {
        setState('error')
        setError(data.error || 'Invalid or expired invite')
        return
      }

      if (data.requiresAuth) {
        // Not logged in — show auth form inline (don't navigate away, keep token)
        setInviteInfo({ email: data.invite?.email ?? '', role: data.invite?.role ?? '' })
        setState('needsAuth')
        return
      }

      // Logged in and accepted
      if (data.workspace) {
        upsertWorkspace(data.workspace)
        setActiveWorkspace(data.workspace.id)
        setInviteInfo({ email: '', role: data.role, workspace: data.workspace.name })
      }
      setState('success')
      setTimeout(onDone, 1500)
    } catch {
      setState('error')
      setError('Connection failed')
    }
  }

  async function acceptInvite() {
    setState('accepting')
    await checkInvite()
  }

  // Show AuthScreen inline — pendingInviteToken keeps the token alive
  // After login/register, AuthScreen calls setAuth which triggers the useEffect above
  if (state === 'needsAuth') {
    return <AuthScreen pendingInviteToken={token} />
  }

  return (
    <div className={styles.screen}>
      <div className={styles.grid} />
      <div className={styles.card}>
        <div className={styles.logo}>
          <div className={styles.logoIcon}><Zap size={20} /></div>
          <span className={styles.logoText}>gRPC Inspector</span>
        </div>

        {state === 'loading' && (
          <div className={styles.stateBox}>
            <Loader size={32} className={styles.spinning} />
            <p>Checking invite…</p>
          </div>
        )}

        {state === 'accepting' && (
          <div className={styles.stateBox}>
            <Loader size={32} className={styles.spinning} />
            <p>Joining workspace…</p>
          </div>
        )}

        {state === 'success' && (
          <div className={styles.stateBox}>
            <CheckCircle size={40} className={styles.successIcon} />
            <p className={styles.successText}>You've joined <strong>{inviteInfo?.workspace}</strong>!</p>
            <p className={styles.successSub}>Redirecting…</p>
          </div>
        )}

        {state === 'error' && (
          <div className={styles.stateBox}>
            <XCircle size={40} className={styles.errorIcon} />
            <p className={styles.errorText}>{error}</p>
            <button className={styles.goAuthBtn} onClick={onDone}>Go to app →</button>
          </div>
        )}
      </div>
    </div>
  )
}
