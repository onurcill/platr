import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export interface User {
  id: string
  email: string
  name: string
  role: string
}

export interface Workspace {
  id: string
  name: string
  description: string
  ownerId: string
  members: WorkspaceMember[]
}

export interface WorkspaceMember {
  id: string
  workspaceId: string
  userId: string
  userEmail: string
  userName: string
  role: string
  joinedAt: string
}

export interface WorkspaceInvite {
  id: string
  workspaceId: string
  email: string
  role: string
  token: string
  inviteUrl: string
  used: boolean
  expiresAt: string
}

interface AuthStore {
  token: string | null
  user: User | null
  workspaces: Workspace[]
  activeWorkspaceId: string | null
  myRoles: Record<string, string> // workspaceId -> role

  setAuth: (token: string, user: User, workspaces: Workspace[], preferWorkspaceId?: string) => void
  logout: () => void
  setWorkspaces: (ws: Workspace[]) => void
  setActiveWorkspace: (id: string) => void
  upsertWorkspace: (ws: Workspace) => void
  removeWorkspace: (id: string) => void
  getActiveWorkspace: () => Workspace | null
  isAuthenticated: () => boolean
}

export const useAuthStore = create<AuthStore>()(
  persist(
    (set, get) => ({
      token: null,
      user: null,
      workspaces: [],
      activeWorkspaceId: null,
      myRoles: {},

      setAuth: (token, user, workspaces, preferWorkspaceId?) => {
        set(s => {
          const existingId = s.activeWorkspaceId
          const stillValid = workspaces.some(w => w.id === existingId)
          // preferWorkspaceId wins > existing > first workspace
          const activeId = (preferWorkspaceId && workspaces.some(w => w.id === preferWorkspaceId))
            ? preferWorkspaceId
            : stillValid ? existingId : (workspaces[0]?.id ?? null)
          // Build myRoles map from members arrays
          const myRoles: Record<string, string> = {}
          workspaces.forEach(ws => {
            const me = ws.members?.find(m => m.userId === user.id)
            if (me) myRoles[ws.id] = me.role
          })
          return { token, user, workspaces, activeWorkspaceId: activeId, myRoles }
        })
      },

      logout: () => set({ token: null, user: null, workspaces: [], activeWorkspaceId: null }),

      setWorkspaces: (workspaces) => set(s => ({
        workspaces,
        activeWorkspaceId: s.activeWorkspaceId ?? workspaces[0]?.id ?? null,
      })),

      setActiveWorkspace: (id) => set({ activeWorkspaceId: id }),

      upsertWorkspace: (ws) => set(s => {
        const idx = s.workspaces.findIndex(w => w.id === ws.id)
        const workspaces = idx >= 0
          ? s.workspaces.map((w, i) => i === idx ? ws : w)
          : [...s.workspaces, ws]
        // Refresh myRoles if members updated
        const myRoles = { ...s.myRoles }
        if (ws.members && s.user) {
          const me = ws.members.find(m => m.userId === s.user!.id)
          if (me) myRoles[ws.id] = me.role
        }
        return { workspaces, myRoles }
      }),

      removeWorkspace: (id) => set(s => ({
        workspaces: s.workspaces.filter(w => w.id !== id),
        activeWorkspaceId: s.activeWorkspaceId === id
          ? s.workspaces.find(w => w.id !== id)?.id ?? null
          : s.activeWorkspaceId,
      })),

      getActiveWorkspace: () => {
        const { workspaces, activeWorkspaceId } = get()
        return workspaces.find(w => w.id === activeWorkspaceId) ?? workspaces[0] ?? null
      },

      isAuthenticated: () => !!get().token,
    }),
    {
      name: 'grpc-inspector-auth',
      partialize: (s) => ({ token: s.token, user: s.user, activeWorkspaceId: s.activeWorkspaceId }),
    }
  )
)

// Axios-like fetch wrapper that injects auth header
export async function apiFetch(url: string, options: RequestInit = {}): Promise<Response> {
  // Try store first, then localStorage directly as fallback for rehydration timing
  let token: string | null = useAuthStore.getState().token
  if (!token) {
    try {
      const raw = localStorage.getItem('grpc-inspector-auth')
      token = JSON.parse(raw || '{}')?.state?.token ?? null
    } catch { token = null }
  }
  const isFormData = options.body instanceof FormData
  if (!token) console.warn('[apiFetch] No token for', url)

  // Build headers: for FormData, strip Content-Type entirely (browser sets boundary)
  const baseHeaders: Record<string, string> = {}
  if (!isFormData) baseHeaders['Content-Type'] = 'application/json'
  if (token) baseHeaders['Authorization'] = `Bearer ${token}`

  // Merge caller headers but never let Content-Type override FormData requests
  const callerHeaders = options.headers as Record<string, string> | undefined
  if (callerHeaders) {
    for (const [k, v] of Object.entries(callerHeaders)) {
      if (isFormData && k.toLowerCase() === 'content-type') continue
      baseHeaders[k] = v
    }
  }

  return fetch(url, { ...options, headers: baseHeaders })
}
