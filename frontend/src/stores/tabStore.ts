import { create } from 'zustand'
import type { SavedRequest } from './collectionStore'
// lazy import to avoid circular deps
function getActiveConnId(): string | null {
  try { return (window as any).__connectionStore?.getState?.()?.activeConnectionId ?? null } catch { return null }
}

export type TabType = 'new' | 'collection' | 'history'

export interface RequestTab {
  id: string                          // unique tab id
  type: TabType
  title: string
  isDirty: boolean                    // unsaved changes indicator

  // Request state
  service: string
  method: string
  requestBody: string
  metadata: Record<string, string>
  connAddress: string

  // Linked entities
  savedRequestId?: string             // if opened from collection
  collectionId?: string
  connectionId?: string | null
  envId?: string | null

  // Response state
  response: unknown | null
  responseStatus: string | null
  responseDuration: number | null
  responseHeaders: Record<string, string>
  responseTrailers: Record<string, string>
  streamMessages: unknown[]
  isLoading: boolean
  isStreaming: boolean
}

function newTab(overrides: Partial<RequestTab> = {}): RequestTab {
  return {
    id: Math.random().toString(36).slice(2),
    type: 'new',
    title: 'New Request',
    isDirty: false,
    service: '',
    method: '',
    requestBody: '{}',
    metadata: {},
    connAddress: '',
    response: null,
    responseStatus: null,
    responseDuration: null,
    responseHeaders: {},
    responseTrailers: {},
    streamMessages: [],
    isLoading: false,
    isStreaming: false,
    ...overrides,
  }
}

interface TabStore {
  tabs: RequestTab[]
  activeTabId: string | null

  // Tab management
  openNewTab: () => void
  openSavedRequest: (req: SavedRequest) => void
  openFromMethod: (service: string, method: string, body?: string, connectionId?: string | null) => void
  closeTab: (id: string) => void
  setActiveTab: (id: string) => void

  // Tab state updates (all keyed by tabId)
  updateTab: (id: string, patch: Partial<RequestTab>) => void
  setTabBody: (id: string, body: string) => void
  setTabMetadata: (id: string, metadata: Record<string, string>) => void
  setTabResponse: (id: string, response: unknown, status: string, duration: number, headers: Record<string, string>, trailers: Record<string, string>) => void
  setTabLoading: (id: string, loading: boolean) => void
  setTabStreaming: (id: string, streaming: boolean) => void
  addTabStreamMessage: (id: string, msg: unknown) => void
  clearTabStream: (id: string) => void
  markTabClean: (id: string) => void
  clearAllTabs: () => void
}

export const useTabStore = create<TabStore>((set, get) => {
  const initialTab = newTab()

  return {
    tabs: [initialTab],
    activeTabId: initialTab.id,

    openNewTab: () => {
      const tab = newTab()
      set(s => ({ tabs: [...s.tabs, tab], activeTabId: tab.id }))
    },

    openSavedRequest: (req: SavedRequest) => {
      const { tabs } = get()
      // If already open, just activate
      const existing = tabs.find(t => t.savedRequestId === req.id)
      if (existing) {
        set({ activeTabId: existing.id })
        return
      }
      const tab = newTab({
        type: 'collection',
        title: req.name,
        service: req.service,
        method: req.method,
        requestBody: req.body || '{}',
        metadata: req.metadata || {},
        connAddress: req.connAddress,
        savedRequestId: req.id,
        collectionId: req.collectionId,
        envId: req.envId,
      })
      set(s => ({ tabs: [...s.tabs, tab], activeTabId: tab.id }))
    },

    openFromMethod: (service: string, method: string, body?: string, connectionId?: string | null) => {
      const tab = newTab({
        type: 'new',
        title: method,
        service,
        method,
        requestBody: body || '{}',
        connectionId: connectionId ?? getActiveConnId(),
      })
      set(s => ({ tabs: [...s.tabs, tab], activeTabId: tab.id }))
    },

    closeTab: (id: string) => {
      set(s => {
        const idx = s.tabs.findIndex(t => t.id === id)
        const filtered = s.tabs.filter(t => t.id !== id)
        if (filtered.length === 0) {
          const fresh = newTab()
          return { tabs: [fresh], activeTabId: fresh.id }
        }
        let newActive = s.activeTabId
        if (s.activeTabId === id) {
          // activate neighbor
          newActive = filtered[Math.min(idx, filtered.length - 1)].id
        }
        return { tabs: filtered, activeTabId: newActive }
      })
    },

    setActiveTab: (id) => set({ activeTabId: id }),


    updateTab: (id, patch) => set(s => ({
      tabs: s.tabs.map(t => t.id === id ? { ...t, ...patch } : t)
    })),

    setTabBody: (id, body) => set(s => ({
      tabs: s.tabs.map(t => t.id === id ? { ...t, requestBody: body, isDirty: true } : t)
    })),

    setTabMetadata: (id, metadata) => set(s => ({
      tabs: s.tabs.map(t => t.id === id ? { ...t, metadata, isDirty: true } : t)
    })),

    setTabResponse: (id, response, responseStatus, responseDuration, responseHeaders, responseTrailers) =>
      set(s => ({
        tabs: s.tabs.map(t => t.id === id
          ? { ...t, response, responseStatus, responseDuration, responseHeaders, responseTrailers }
          : t)
      })),

    setTabLoading: (id, isLoading) => set(s => ({
      tabs: s.tabs.map(t => t.id === id ? { ...t, isLoading } : t)
    })),

    setTabStreaming: (id, isStreaming) => set(s => ({
      tabs: s.tabs.map(t => t.id === id ? { ...t, isStreaming } : t)
    })),

    addTabStreamMessage: (id, msg) => set(s => ({
      tabs: s.tabs.map(t => t.id === id ? { ...t, streamMessages: [...t.streamMessages, msg] } : t)
    })),

    clearTabStream: (id) => set(s => ({
      tabs: s.tabs.map(t => t.id === id ? { ...t, streamMessages: [] } : t)
    })),

    markTabClean: (id) => set(s => ({
      tabs: s.tabs.map(t => t.id === id ? { ...t, isDirty: false } : t)
    })),

    clearAllTabs: () => {
      const fresh = newTab()
      set({ tabs: [fresh], activeTabId: fresh.id })
    },
  }
})
