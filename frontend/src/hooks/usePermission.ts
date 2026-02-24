import { useAuthStore } from '../stores/authStore'

// Must match backend auth/permission.go
type Role = 'owner' | 'admin' | 'editor' | 'viewer' | 'guest' | ''

const roleRank: Record<string, number> = {
  guest: 0,
  viewer: 1,
  editor: 2,
  admin: 3,
  owner: 4,
}

const permissionMatrix: Record<string, Role> = {
  'workspace:update':   'owner',
  'workspace:delete':   'owner',
  'member:invite':      'admin',
  'member:remove':      'admin',
  'member:set_role':    'admin',
  'collection:create':  'editor',
  'collection:update':  'editor',
  'collection:delete':  'editor',
  'collection:read':    'guest',
  'request:create':     'editor',
  'request:update':     'editor',
  'request:delete':     'editor',
  'invoke':             'viewer',
  'environment:manage': 'editor',
  'environment:read':   'viewer',
  'proto:upload':       'editor',
  'history:read':       'viewer',
}

export type Action = keyof typeof permissionMatrix

function can(role: string, action: Action): boolean {
  const required = permissionMatrix[action]
  if (!required) return false
  return (roleRank[role] ?? -1) >= (roleRank[required] ?? 99)
}

export function usePermission() {
  const activeWorkspaceId = useAuthStore(s => s.activeWorkspaceId)
  const myRoles           = useAuthStore(s => s.myRoles)
  const workspaces        = useAuthStore(s => s.workspaces)
  const user              = useAuthStore(s => s.user)

  // Primary source: myRoles map (populated from me/login response)
  let myRole: string = (activeWorkspaceId ? myRoles[activeWorkspaceId] : '') ?? ''

  // Fallback: scan members array if myRoles not populated yet
  if (!myRole && activeWorkspaceId && user) {
    const activeWs = workspaces.find(w => w.id === activeWorkspaceId)
    const me = activeWs?.members?.find(m => m.userId === user.id)
    if (me) myRole = me.role
  }

  return {
    myRole,
    can: (action: Action) => can(myRole, action),
    canAny: (...actions: Action[]) => actions.some(a => can(myRole, a)),
    isOwner: myRole === 'owner',
    isAdmin: roleRank[myRole] >= roleRank['admin'],
    isEditor: can(myRole, 'collection:create'),
    isViewer: can(myRole, 'invoke'),
    roleLabel: myRole ? myRole.charAt(0).toUpperCase() + myRole.slice(1) : '',
  }
}

export const ROLE_DESCRIPTIONS: Record<string, string> = {
  owner:  'Full control. Can delete workspace.',
  admin:  'Can manage members and invites.',
  editor: 'Can create collections and upload protos.',
  viewer: 'Can send requests and view history.',
  guest:  'Read-only access to collections.',
}

export const ASSIGNABLE_ROLES = ['admin', 'editor', 'viewer', 'guest']
