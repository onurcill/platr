import { useEffect, useRef, useState } from 'react'
import {
  Users, Plus, X, Crown, Edit2, Eye, Trash2,
  Link, Check, ChevronDown, LogOut, Building2,
  Loader, Mail, AlertCircle
} from 'lucide-react'
import { useAuthStore, apiFetch } from '../../stores'
import { RolesPanel } from '../RolesPanel'
import { usePermission, ASSIGNABLE_ROLES } from '../../hooks/usePermission'
import type { WorkspaceMember, WorkspaceInvite } from '../../stores'
import styles from './WorkspacePanel.module.css'

// ─── Toast ────────────────────────────────────────────────────────────────────
interface Toast { id: number; type: 'success' | 'error'; message: string }

function useToast() {
  const [toasts, setToasts] = useState<Toast[]>([])
  const counter = useRef(0)

  function add(type: Toast['type'], message: string) {
    const id = ++counter.current
    setToasts(t => [...t, { id, type, message }])
    setTimeout(() => setToasts(t => t.filter(x => x.id !== id)), 3500)
  }

  return { toasts, success: (m: string) => add('success', m), error: (m: string) => add('error', m) }
}

export function WorkspacePanel({ onClose }: { onClose: () => void }) {
  const { user, workspaces, activeWorkspaceId, setActiveWorkspace, upsertWorkspace, removeWorkspace, logout } = useAuthStore()
  const activeWs = workspaces.find(w => w.id === activeWorkspaceId) ?? workspaces[0]

  const [tab, setTab] = useState<'members' | 'invites' | 'roles' | 'settings'>('members')
  const [members, setMembers] = useState<WorkspaceMember[]>([])
  const [invites, setInvites] = useState<WorkspaceInvite[]>([])
  const [loading, setLoading] = useState(false)

  // Invite state
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<'editor' | 'viewer'>('editor')
  const [inviteSending, setInviteSending] = useState(false)

  // Copy link state
  const [copiedToken, setCopiedToken] = useState<string | null>(null)

  // Workspace create
  const [newWsName, setNewWsName] = useState('')
  const [creatingWs, setCreatingWs] = useState(false)
  const [wsName, setWsName] = useState(activeWs?.name ?? '')
  const [wsSaving, setWsSaving] = useState(false)
  const [wsSaved, setWsSaved] = useState(false)

  const toast = useToast()
  const { myRole, can, isOwner } = usePermission()
  const canManage = can('member:invite')

  useEffect(() => {
    if (!activeWs) return
    loadMembers()
    setWsName(activeWs.name)
  }, [activeWs?.id])

  useEffect(() => {
    if (!activeWs || tab !== 'invites') return
    loadInvites()
  }, [tab, activeWs?.id])

  async function loadMembers() {
    if (!activeWs) return
    setLoading(true)
    try {
      const res = await apiFetch(`/api/workspaces/${activeWs.id}/members`)
      if (!res.ok) return
      const data = await res.json()
      setMembers(data)
      upsertWorkspace({ ...activeWs, members: data })
    } finally {
      setLoading(false)
    }
  }

  async function loadInvites() {
    if (!activeWs) return
    const res = await apiFetch(`/api/workspaces/${activeWs.id}/invites`)
    if (!res.ok) return
    const data = await res.json()
    setInvites(Array.isArray(data) ? data : [])
  }

  async function sendInvite() {
    const email = inviteEmail.trim()
    if (!email || !activeWs || inviteSending) return

    setInviteSending(true)
    // Clear input immediately for quick re-use feel
    setInviteEmail('')

    try {
      const res = await apiFetch(`/api/workspaces/${activeWs.id}/invites`, {
        method: 'POST',
        body: JSON.stringify({ email, role: inviteRole }),
      })
      const data = await res.json()

      if (!res.ok) {
        setInviteEmail(email) // restore on error
        toast.error(data.error || 'Failed to send invite')
        return
      }

      // Add to invites list
      setInvites(inv => [{ ...data.invite ?? data, inviteUrl: data.inviteUrl }, ...inv])

      if (data.emailSent) {
        toast.success(`Invite email sent to ${email}`)
      } else {
        toast.success(`Invite created for ${email} — copy the link below`)
        setTab('invites')
      }
    } catch {
      setInviteEmail(email)
      toast.error('Network error — please try again')
    } finally {
      setInviteSending(false)
    }
  }

  async function removeMember(userId: string) {
    if (!activeWs) return
    // Optimistic
    setMembers(m => m.filter(x => x.userId !== userId))
    try {
      const res = await apiFetch(`/api/workspaces/${activeWs.id}/members/${userId}`, { method: 'DELETE' })
      if (!res.ok) {
        await loadMembers() // rollback
        toast.error('Failed to remove member')
      }
    } catch {
      await loadMembers()
      toast.error('Network error')
    }
  }

  async function updateRole(userId: string, role: string) {
    if (!activeWs) return
    // Optimistic
    const prev = members.find(m => m.userId === userId)?.role
    setMembers(m => m.map(x => x.userId === userId ? { ...x, role } : x))
    try {
      const res = await apiFetch(`/api/workspaces/${activeWs.id}/members/${userId}`, {
        method: 'PUT',
        body: JSON.stringify({ role }),
      })
      if (!res.ok) {
        setMembers(m => m.map(x => x.userId === userId ? { ...x, role: prev! } : x))
        toast.error('Failed to update role')
      }
    } catch {
      setMembers(m => m.map(x => x.userId === userId ? { ...x, role: prev! } : x))
      toast.error('Network error')
    }
  }

  async function createWorkspace() {
    if (!newWsName.trim()) return
    try {
      const res = await apiFetch('/api/workspaces', {
        method: 'POST',
        body: JSON.stringify({ name: newWsName.trim(), description: '' }),
      })
      const ws = await res.json()
      upsertWorkspace(ws)
      setActiveWorkspace(ws.id)
      setCreatingWs(false)
      setNewWsName('')
    } catch {
      toast.error('Failed to create workspace')
    }
  }

  async function updateWorkspace() {
    if (!activeWs || !wsName.trim()) return
    setWsSaving(true)
    try {
      const res = await apiFetch(`/api/workspaces/${activeWs.id}`, {
        method: 'PUT',
        body: JSON.stringify({ name: wsName.trim(), description: activeWs.description }),
      })
      const updated = await res.json()
      upsertWorkspace(updated)
      setWsSaved(true)
      setTimeout(() => setWsSaved(false), 2000)
    } catch {
      toast.error('Failed to save')
    } finally {
      setWsSaving(false)
    }
  }

  async function deleteWorkspace() {
    if (!activeWs || !isOwner) return
    if (!confirm(`Delete "${activeWs.name}"? This will permanently remove all collections, environments and history.`)) return
    try {
      await apiFetch(`/api/workspaces/${activeWs.id}`, { method: 'DELETE' })
      removeWorkspace(activeWs.id)
      onClose()
    } catch {
      toast.error('Failed to delete workspace')
    }
  }

  function copyInviteLink(inviteUrl: string, token: string) {
    navigator.clipboard.writeText(window.location.origin + inviteUrl)
    setCopiedToken(token)
    setTimeout(() => setCopiedToken(null), 2000)
  }

  function roleIcon(role: string) {
    if (role === 'owner') return <Crown size={11} className={styles.roleOwner} />
    if (role === 'admin') return <Crown size={11} className={styles.roleAdmin} />
    if (role === 'editor') return <Edit2 size={11} className={styles.roleEditor} />
    return <Eye size={11} className={styles.roleViewer} />
  }

  return (
    <div className={styles.overlay} onClick={e => e.target === e.currentTarget && onClose()}>
      <div className={styles.panel}>

        {/* Toasts */}
        {toast.toasts.length > 0 && (
          <div className={styles.toastStack}>
            {toast.toasts.map(t => (
              <div
                key={t.id}
                className={`${styles.inviteToast} ${t.type === 'success' ? styles.inviteToastSuccess : styles.inviteToastError}`}
              >
                {t.type === 'success' ? <Check size={13} /> : <AlertCircle size={13} />}
                <span>{t.message}</span>
              </div>
            ))}
          </div>
        )}

        {/* Header */}
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <Building2 size={16} className={styles.headerIcon} />
            <span>Workspace</span>
          </div>
          <button className={styles.closeBtn} onClick={onClose}><X size={15} /></button>
        </div>

        {/* Workspace switcher */}
        <div className={styles.wsSwitcher}>
          {workspaces.map(ws => (
            <button
              key={ws.id}
              className={`${styles.wsItem} ${ws.id === activeWorkspaceId ? styles.wsItemActive : ''}`}
              onClick={() => setActiveWorkspace(ws.id)}
            >
              <div className={styles.wsAvatar}>{ws.name[0]?.toUpperCase()}</div>
              <div className={styles.wsInfo}>
                <span className={styles.wsName}>{ws.name}</span>
                {ws.id === activeWorkspaceId && <span className={styles.wsActive}>Active</span>}
              </div>
              {ws.id === activeWorkspaceId && <Check size={13} className={styles.wsCheck} />}
            </button>
          ))}

          {creatingWs ? (
            <div className={styles.wsCreate}>
              <input
                className={styles.wsInput}
                value={newWsName}
                onChange={e => setNewWsName(e.target.value)}
                placeholder="Workspace name"
                autoFocus
                onKeyDown={e => {
                  if (e.key === 'Enter') createWorkspace()
                  if (e.key === 'Escape') setCreatingWs(false)
                }}
              />
              <div className={styles.wsCreateBtns}>
                <button className={styles.cancelSmall} onClick={() => setCreatingWs(false)}>Cancel</button>
                <button className={styles.createSmall} onClick={createWorkspace} disabled={!newWsName.trim()}>Create</button>
              </div>
            </div>
          ) : (
            <button className={styles.addWsBtn} onClick={() => setCreatingWs(true)}>
              <Plus size={13} /> New workspace
            </button>
          )}
        </div>

        {activeWs && (
          <>
            {/* Tabs */}
            <div className={styles.tabs}>
              {(['members', 'invites', 'roles', 'settings'] as const).map(t => (
                <button
                  key={t}
                  className={`${styles.tab} ${tab === t ? styles.tabActive : ''}`}
                  onClick={() => setTab(t)}
                >
                  {t.charAt(0).toUpperCase() + t.slice(1)}
                </button>
              ))}
            </div>

            {/* Members tab */}
            {tab === 'members' && (
              <div className={styles.tabContent}>
                {canManage && (
                  <div className={styles.inviteRow}>
                    <input
                      className={styles.inviteInput}
                      value={inviteEmail}
                      onChange={e => setInviteEmail(e.target.value)}
                      placeholder="colleague@company.com"
                      disabled={inviteSending}
                      onKeyDown={e => e.key === 'Enter' && sendInvite()}
                    />
                    <select
                      className={styles.roleSelect}
                      value={inviteRole}
                      onChange={e => setInviteRole(e.target.value as any)}
                      disabled={inviteSending}
                    >
                      <option value="editor">Editor</option>
                      <option value="viewer">Viewer</option>
                    </select>
                    <button
                      className={`${styles.inviteBtn} ${inviteSending ? styles.inviteBtnSending : ''}`}
                      onClick={sendInvite}
                      disabled={!inviteEmail.trim() || inviteSending}
                    >
                      {inviteSending ? (
                        <><Loader size={12} className={styles.spin} /> Sending…</>
                      ) : (
                        <><Mail size={12} /> Invite</>
                      )}
                    </button>
                  </div>
                )}

                <div className={styles.memberList}>
                  {loading && <div className={styles.empty}>Loading…</div>}
                  {members.map(m => (
                    <div key={m.id} className={styles.memberRow}>
                      <div className={styles.memberAvatar}>{m.userName?.[0]?.toUpperCase() ?? '?'}</div>
                      <div className={styles.memberInfo}>
                        <span className={styles.memberName}>
                          {m.userName}
                          {m.userId === user?.id && <span className={styles.youBadge}>you</span>}
                        </span>
                        <span className={styles.memberEmail}>{m.userEmail}</span>
                      </div>
                      <div className={styles.memberRole}>
                        {roleIcon(m.role)}
                        {canManage && m.role !== 'owner' ? (
                          <select
                            className={styles.roleSelectInline}
                            value={m.role}
                            onChange={e => updateRole(m.userId, e.target.value)}
                          >
                            {ASSIGNABLE_ROLES.map(r => (
                              <option key={r} value={r}>{r.charAt(0).toUpperCase() + r.slice(1)}</option>
                            ))}
                          </select>
                        ) : (
                          <span className={styles.roleLabel}>{m.role}</span>
                        )}
                      </div>
                      {can('member:remove') && m.role !== 'owner' && m.userId !== user?.id && (
                        <button
                          className={styles.removeMemberBtn}
                          onClick={() => removeMember(m.userId)}
                          title="Remove member"
                        >
                          <X size={12} />
                        </button>
                      )}
                      {m.userId === user?.id && m.role !== 'owner' && (
                        <button
                          className={styles.removeMemberBtn}
                          onClick={() => removeMember(m.userId)}
                          title="Leave workspace"
                        >
                          <LogOut size={12} />
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Invites tab */}
            {tab === 'invites' && (
              <div className={styles.tabContent}>
                {invites.length === 0 && (
                  <div className={styles.empty}>No pending invites</div>
                )}
                {invites.map(inv => (
                  <div key={inv.id} className={`${styles.inviteItem} ${inv.used ? styles.inviteUsed : ''}`}>
                    <div className={styles.inviteStatusDot} style={{
                      background: inv.used ? 'var(--text-muted)' : 'var(--ok)'
                    }} />
                    <div className={styles.inviteInfo}>
                      <span className={styles.inviteEmail}>{inv.email}</span>
                      <span className={styles.inviteMeta}>
                        <span className={styles.inviteRoleBadge}>{inv.role}</span>
                        {inv.used
                          ? ' · Accepted'
                          : ` · Expires ${new Date(inv.expiresAt).toLocaleDateString()}`
                        }
                      </span>
                    </div>
                    {!inv.used && (
                      <button
                        className={styles.copyLinkBtn}
                        onClick={() => copyInviteLink(inv.inviteUrl ?? `/invite/${inv.token}`, inv.token)}
                        title="Copy invite link"
                      >
                        {copiedToken === inv.token ? <Check size={13} /> : <Link size={13} />}
                      </button>
                    )}
                  </div>
                ))}
              </div>
            )}

            {/* Roles tab */}
            {tab === 'roles' && <RolesPanel />}

            {/* Settings tab */}
            {tab === 'settings' && (
              <div className={styles.tabContent}>
                <label className={styles.settingsLabel}>Workspace name</label>
                <div className={styles.settingsRow}>
                  <input
                    className={styles.settingsInput}
                    value={wsName}
                    onChange={e => setWsName(e.target.value)}
                    disabled={!isOwner}
                    onKeyDown={e => e.key === 'Enter' && isOwner && updateWorkspace()}
                  />
                  {isOwner && (
                    <button
                      className={styles.saveSettingsBtn}
                      onClick={updateWorkspace}
                      disabled={wsSaving || !wsName.trim()}
                    >
                      {wsSaved ? <><Check size={12} /> Saved</> : wsSaving ? <Loader size={12} className={styles.spin} /> : 'Save'}
                    </button>
                  )}
                </div>

                <div className={styles.settingsMeta}>
                  <span>Owner: {members.find(m => m.role === 'owner')?.userName ?? '—'}</span>
                  <span>{members.length} member{members.length !== 1 ? 's' : ''}</span>
                  <span>Your role: <strong>{myRole}</strong></span>
                </div>

                {myRole === 'owner' && (
                  <button className={styles.deleteWsBtn} onClick={deleteWorkspace}>
                    <Trash2 size={13} /> Delete workspace
                  </button>
                )}

                <div className={styles.logoutSection}>
                  <button className={styles.logoutBtn} onClick={logout}>
                    <LogOut size={13} /> Sign out
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
