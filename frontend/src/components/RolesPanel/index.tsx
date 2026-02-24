import { useEffect, useState } from 'react'
import { Check, X, Shield } from 'lucide-react'
import { apiFetch } from '../../stores'
import styles from './RolesPanel.module.css'

interface RoleData {
  id: string
  name: string
  displayName: string
  description: string
  rank: number
  isSystem: boolean
  permissions: string[]
}

// All possible actions grouped by category
const ACTION_GROUPS: { label: string; actions: { key: string; label: string }[] }[] = [
  {
    label: 'Workspace',
    actions: [
      { key: 'workspace:update', label: 'Edit workspace settings' },
      { key: 'workspace:delete', label: 'Delete workspace' },
    ],
  },
  {
    label: 'Members',
    actions: [
      { key: 'member:invite',   label: 'Invite members' },
      { key: 'member:remove',   label: 'Remove members' },
      { key: 'member:set_role', label: 'Change member roles' },
    ],
  },
  {
    label: 'Collections',
    actions: [
      { key: 'collection:read',   label: 'View collections' },
      { key: 'collection:create', label: 'Create collections' },
      { key: 'collection:update', label: 'Edit collections' },
      { key: 'collection:delete', label: 'Delete collections' },
    ],
  },
  {
    label: 'Requests',
    actions: [
      { key: 'request:create', label: 'Save requests' },
      { key: 'request:update', label: 'Edit saved requests' },
      { key: 'request:delete', label: 'Delete saved requests' },
      { key: 'invoke',         label: 'Send requests (invoke)' },
    ],
  },
  {
    label: 'Environment & Proto',
    actions: [
      { key: 'environment:read',   label: 'View environments' },
      { key: 'environment:manage', label: 'Manage environments' },
      { key: 'proto:upload',       label: 'Upload proto files' },
    ],
  },
  {
    label: 'History',
    actions: [
      { key: 'history:read', label: 'View request history' },
    ],
  },
]

const ROLE_COLORS: Record<string, string> = {
  owner:  '#fbbf24',
  admin:  '#60a5fa',
  editor: '#4ade80',
  viewer: '#a78bfa',
  guest:  '#94a3b8',
}

export function RolesPanel() {
  const [roles, setRoles] = useState<RoleData[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    apiFetch('/api/roles')
      .then(r => r.json())
      .then(data => { setRoles(data); setLoading(false) })
      .catch(() => setLoading(false))
  }, [])

  if (loading) return <div className={styles.loading}>Loading roles…</div>

  return (
    <div className={styles.wrap}>
      {/* Role cards */}
      <div className={styles.roleCards}>
        {roles.map(role => (
          <div key={role.id} className={styles.roleCard}>
            <div className={styles.roleCardDot} style={{ background: ROLE_COLORS[role.name] ?? '#64748b' }} />
            <div className={styles.roleCardInfo}>
              <span className={styles.roleCardName}>{role.displayName}</span>
              <span className={styles.roleCardDesc}>{role.description}</span>
            </div>
          </div>
        ))}
      </div>

      {/* Permission matrix table */}
      <div className={styles.tableWrap}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th className={styles.actionCol}>Permission</th>
              {roles.map(role => (
                <th key={role.id} className={styles.roleCol}>
                  <div className={styles.roleHeader}>
                    <div className={styles.roleHeaderDot} style={{ background: ROLE_COLORS[role.name] ?? '#64748b' }} />
                    <span>{role.displayName}</span>
                  </div>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {ACTION_GROUPS.map(group => (
              <>
                <tr key={group.label} className={styles.groupRow}>
                  <td colSpan={roles.length + 1} className={styles.groupLabel}>
                    <Shield size={11} />
                    {group.label}
                  </td>
                </tr>
                {group.actions.map(action => (
                  <tr key={action.key} className={styles.actionRow}>
                    <td className={styles.actionLabel}>{action.label}</td>
                    {roles.map(role => (
                      <td key={role.id} className={styles.permCell}>
                        {role.permissions.includes(action.key)
                          ? <Check size={13} className={styles.permYes} />
                          : <X    size={13} className={styles.permNo}  />
                        }
                      </td>
                    ))}
                  </tr>
                ))}
              </>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
