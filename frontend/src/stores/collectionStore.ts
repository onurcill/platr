import { create } from 'zustand'

export interface SavedRequest {
  id: string
  collectionId: string
  name: string
  description: string
  service: string
  method: string
  body: string
  metadata: Record<string, string>
  connAddress: string
  envId?: string | null
  sortOrder: number
  createdAt: string
  updatedAt: string
}

export interface Collection {
  id: string
  name: string
  description: string
  color: string
  sortOrder: number
  requests: SavedRequest[]
  createdAt: string
  updatedAt: string
}

interface CollectionStore {
  collections: Collection[]
  setCollections: (cols: Collection[]) => void
  upsertCollection: (col: Collection) => void
  removeCollection: (id: string) => void
  upsertRequest: (req: SavedRequest) => void
  removeRequest: (id: string) => void
}

export const useCollectionStore = create<CollectionStore>((set) => ({
  collections: [],

  setCollections: (collections) => set({ collections }),

  upsertCollection: (col) => set((s) => {
    const idx = s.collections.findIndex(c => c.id === col.id)
    if (idx >= 0) {
      const updated = [...s.collections]
      updated[idx] = col
      return { collections: updated }
    }
    return { collections: [...s.collections, col] }
  }),

  removeCollection: (id) => set((s) => ({
    collections: s.collections.filter(c => c.id !== id)
  })),

  upsertRequest: (req) => set((s) => ({
    collections: s.collections.map(col => {
      if (col.id !== req.collectionId) return col
      const idx = col.requests.findIndex(r => r.id === req.id)
      if (idx >= 0) {
        const reqs = [...col.requests]
        reqs[idx] = req
        return { ...col, requests: reqs }
      }
      return { ...col, requests: [...col.requests, req] }
    })
  })),

  removeRequest: (id) => set((s) => ({
    collections: s.collections.map(col => ({
      ...col,
      requests: col.requests.filter(r => r.id !== id)
    }))
  })),
}))
