import { useState } from 'react'
import { Zap, Mail, Lock, User, Eye, EyeOff, AlertCircle, Users } from 'lucide-react'
import { useAuthStore, apiFetch } from '../../stores'
import styles from './AuthScreen.module.css'

type Mode = 'login' | 'register'

interface Props {
  // If set, after login/register we auto-accept this invite
  pendingInviteToken?: string | null
}

export function AuthScreen({ pendingInviteToken }: Props) {
  const { setAuth, upsertWorkspace, setActiveWorkspace } = useAuthStore()
  const [mode, setMode] = useState<Mode>('register')
  const [email, setEmail]       = useState('')
  const [name, setName]         = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading]   = useState(false)
  const [error, setError]       = useState('')

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const endpoint = mode === 'login' ? '/api/auth/login' : '/api/auth/register'
      const body     = mode === 'login' ? { email, password } : { email, name, password }

      const res  = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const data = await res.json()

      if (!res.ok) {
        setError(data.error || 'Something went wrong')
        return
      }

      const workspaces = data.workspaces ?? (data.workspace ? [data.workspace] : [])

      if (pendingInviteToken) {
        const invRes = await fetch(`/api/invites/${pendingInviteToken}/accept`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${data.token}` },
        })
        if (invRes.ok) {
          const invData = await invRes.json()
          if (invData.workspace) {
            // Personal workspace stays in list, invite workspace becomes active
            const allWorkspaces = [...workspaces, invData.workspace]
            setAuth(data.token, data.user, allWorkspaces, invData.workspace.id)
            return
          }
        }
        // Invite failed — still log in normally
      }

      setAuth(data.token, data.user, workspaces)
    } catch {
      setError('Connection failed — is the backend running?')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={styles.screen}>
      <div className={styles.grid} />
      <div className={styles.card}>
        <div className={styles.logo}>
          <div className={styles.logoIcon}><Zap size={20} /></div>
          <span className={styles.logoText}>gRPC Inspector</span>
        </div>

        {/* Invite banner */}
        {pendingInviteToken && (
          <div className={styles.inviteBanner}>
            <Users size={13} />
            You have a pending workspace invite. Sign in or create an account to accept it.
          </div>
        )}

        <h1 className={styles.heading}>
          {mode === 'login' ? 'Welcome back' : 'Create account'}
        </h1>
        <p className={styles.subheading}>
          {mode === 'login'
            ? 'Sign in to access your workspaces'
            : 'Start collaborating with your team'}
        </p>

        <form className={styles.form} onSubmit={handleSubmit}>
          {mode === 'register' && (
            <div className={styles.field}>
              <label className={styles.label}>Full name</label>
              <div className={styles.inputWrap}>
                <User size={15} className={styles.inputIcon} />
                <input className={styles.input} type="text" value={name}
                  onChange={e => setName(e.target.value)} placeholder="John Doe" required autoFocus />
              </div>
            </div>
          )}

          <div className={styles.field}>
            <label className={styles.label}>Email</label>
            <div className={styles.inputWrap}>
              <Mail size={15} className={styles.inputIcon} />
              <input className={styles.input} type="email" value={email}
                onChange={e => setEmail(e.target.value)} placeholder="you@company.com"
                required autoFocus={mode === 'login'} />
            </div>
          </div>

          <div className={styles.field}>
            <label className={styles.label}>Password</label>
            <div className={styles.inputWrap}>
              <Lock size={15} className={styles.inputIcon} />
              <input className={styles.input} type={showPassword ? 'text' : 'password'}
                value={password} onChange={e => setPassword(e.target.value)}
                placeholder={mode === 'register' ? 'At least 8 characters' : '••••••••'} required />
              <button type="button" className={styles.eyeBtn}
                onClick={() => setShowPassword(v => !v)} tabIndex={-1}>
                {showPassword ? <EyeOff size={14} /> : <Eye size={14} />}
              </button>
            </div>
          </div>

          {error && (
            <div className={styles.error}><AlertCircle size={13} />{error}</div>
          )}

          <button className={styles.submitBtn} type="submit" disabled={loading}>
            {loading ? <span className={styles.spinner} /> : null}
            {loading ? 'Please wait…' : mode === 'login'
              ? (pendingInviteToken ? 'Sign in & Accept Invite' : 'Sign in')
              : (pendingInviteToken ? 'Create Account & Accept Invite' : 'Create account')}
          </button>
        </form>

        <div className={styles.toggle}>
          {mode === 'login' ? (
            <>Don't have an account?{' '}
              <button onClick={() => { setMode('register'); setError('') }}>Sign up</button>
            </>
          ) : (
            <>Already have an account?{' '}
              <button onClick={() => { setMode('login'); setError('') }}>Sign in</button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
