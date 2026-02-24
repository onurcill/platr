import { create } from 'zustand'
import type { Connection, ServiceDescriptor, MethodDescriptor, HistoryEntry } from '../types'

// ── Connection store ─────────────────────────────────────────

interface ConnectionStore {
  connections: Connection[]
  activeConnectionId: string | null
  setConnections: (c: Connection[]) => void
  addConnection: (c: Connection) => void
  removeConnection: (id: string) => void
  setActiveConnection: (id: string | null) => void
}

export const useConnectionStore = create<ConnectionStore>((set) => ({

  connections: [],
  activeConnectionId: null,
  setConnections: (connections) => set({ connections }),
  addConnection: (c) => set((s) => ({ connections: s.connections.find(x => x.id === c.id) ? s.connections.map(x => x.id === c.id ? c : x) : [...s.connections, c] })),
  removeConnection: (id) => set((s) => ({ connections: s.connections.filter((c) => c.id !== id) })),
  setActiveConnection: (id) => set({ activeConnectionId: id }),
}))

// ── Service explorer store ───────────────────────────────────

interface ServiceStore {
  // keyed by connectionId -> serviceName -> descriptor
  services: Record<string, Record<string, ServiceDescriptor>>
  serviceNames: Record<string, string[]>
  selectedService: string | null
  selectedMethod: MethodDescriptor | null
  expandedServices: Set<string>
  setServiceNames: (connId: string, names: string[]) => void
  setService: (connId: string, name: string, desc: ServiceDescriptor) => void
  selectMethod: (method: MethodDescriptor | null) => void
  selectService: (name: string | null) => void
  toggleService: (name: string) => void
}

export const useServiceStore = create<ServiceStore>((set) => ({
  services: {},
  serviceNames: {},
  selectedService: null,
  selectedMethod: null,
  expandedServices: new Set(),
  setServiceNames: (connId, names) =>
    set((s) => ({ serviceNames: { ...s.serviceNames, [connId]: names } })),
  setService: (connId, name, desc) =>
    set((s) => ({
      services: {
        ...s.services,
        [connId]: { ...(s.services[connId] || {}), [name]: desc },
      },
    })),
  selectMethod: (method) => set({ selectedMethod: method }),
  selectService: (name) => set({ selectedService: name }),
  toggleService: (name) =>
    set((s) => {
      const next = new Set(s.expandedServices)
      next.has(name) ? next.delete(name) : next.add(name)
      return { expandedServices: next }
    }),
}))

// ── Request store ─────────────────────────────────────────────

interface RequestStore {
  requestBody: string
  metadata: string
  response: unknown | null
  responseStatus: string | null
  responseDuration: number | null
  responseHeaders: Record<string, string>
  responseTrailers: Record<string, string>
  isLoading: boolean
  streamMessages: unknown[]
  isStreaming: boolean
  setRequestBody: (v: string) => void
  setMetadata: (v: string) => void
  setResponse: (r: unknown, status: string, duration: number, headers: Record<string, string>, trailers: Record<string, string>) => void
  setLoading: (v: boolean) => void
  addStreamMessage: (msg: unknown) => void
  clearStream: () => void
  setStreaming: (v: boolean) => void
}

export const useRequestStore = create<RequestStore>((set) => ({
  requestBody: '{}',
  metadata: '{}',
  response: null,
  responseStatus: null,
  responseDuration: null,
  responseHeaders: {},
  responseTrailers: {},
  isLoading: false,
  streamMessages: [],
  isStreaming: false,
  setRequestBody: (v) => set({ requestBody: v }),
  setMetadata: (v) => set({ metadata: v }),
  setResponse: (response, responseStatus, responseDuration, responseHeaders, responseTrailers) =>
    set({ response, responseStatus, responseDuration, responseHeaders, responseTrailers }),
  setLoading: (isLoading) => set({ isLoading }),
  addStreamMessage: (msg) => set((s) => ({ streamMessages: [...s.streamMessages, msg] })),
  clearStream: () => set({ streamMessages: [] }),
  setStreaming: (isStreaming) => set({ isStreaming }),
}))

// ── History store ─────────────────────────────────────────────

interface HistoryStore {
  entries: HistoryEntry[]
  setEntries: (e: HistoryEntry[]) => void
  addEntry: (e: HistoryEntry) => void
  removeEntry: (id: string) => void
  clear: () => void
}

export const useHistoryStore = create<HistoryStore>((set) => ({
  entries: [],
  setEntries: (entries) => set({ entries }),
  addEntry: (e) => set((s) => ({ entries: [e, ...s.entries] })),
  removeEntry: (id) => set((s) => ({ entries: s.entries.filter((e) => e.id !== id) })),
  clear: () => set({ entries: [] }),
}))

// ── Theme store ───────────────────────────────────────────────

interface ThemeStore {
  theme: 'dark' | 'light'
  toggleTheme: () => void
}

export const useThemeStore = create<ThemeStore>((set) => ({
  theme: 'dark',
  toggleTheme: () =>
    set((s) => {
      const next = s.theme === 'dark' ? 'light' : 'dark'
      document.documentElement.setAttribute('data-theme', next)
      return { theme: next }
    }),
}))
export { useEnvironmentStore } from './environmentStore'
export type { Environment, EnvVar } from './environmentStore'
export { useCollectionStore } from './collectionStore'
export type { Collection, SavedRequest } from './collectionStore'

export { useTabStore } from './tabStore'
export type { RequestTab, TabType } from './tabStore'

export { useAuthStore, apiFetch } from './authStore'
export type { User, Workspace, WorkspaceMember, WorkspaceInvite } from './authStore'

// Expose connection store for tabStore (avoids circular imports)
if (typeof window !== 'undefined') {
  (window as any).__connectionStore = useConnectionStore
}
export { useBillingStore } from './billingStore'
export type { PlanConfig, Subscription, UsageSummary, PlanName } from './billingStore'
