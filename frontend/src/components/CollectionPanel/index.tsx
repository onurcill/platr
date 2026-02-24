import { useEffect, useRef, useState } from 'react'
import {
  FolderOpen, Plus, Trash2, Edit2, MoreHorizontal,
  ChevronRight, ChevronDown, Download, Upload, Save, X
} from 'lucide-react'
import { useCollectionStore, useTabStore, useEnvironmentStore, useAuthStore, apiFetch } from '../../stores'
import { usePermission } from '../../hooks/usePermission'
import type { Collection, SavedRequest } from '../../stores'
import styles from './CollectionPanel.module.css'

const COLORS = ['#4ade80','#60a5fa','#f472b6','#fb923c','#a78bfa','#34d399','#fbbf24','#f87171']

export function CollectionPanel() {
  const { collections, setCollections, upsertCollection, removeCollection, upsertRequest, removeRequest } = useCollectionStore()
  const { openSavedRequest, activeTabId } = useTabStore()
  const { activeEnvId } = useEnvironmentStore()
  const { can } = usePermission()
  const activeWorkspaceId = useAuthStore(s => s.activeWorkspaceId)
  const workspaces = useAuthStore(s => s.workspaces)
  const activeWs = workspaces.find(w => w.id === activeWorkspaceId) ?? workspaces[0] ?? null

  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(false)
  const [contextMenu, setContextMenu] = useState<{ type: 'collection' | 'request'; id: string; collectionId?: string; x: number; y: number } | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editingName, setEditingName] = useState('')
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [newColor, setNewColor] = useState(COLORS[0])
  const importRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (activeWorkspaceId) {
      // Migrate any workspace_id=NULL collections to this workspace (one-time, idempotent)
      apiFetch(`/api/collections/migrate-orphans?workspaceId=${activeWorkspaceId}`, { method: 'POST' })
        .then(() => fetchCollections())
        .catch(() => fetchCollections())
    } else {
      fetchCollections()
    }
  }, [activeWorkspaceId])

  useEffect(() => {
    const close = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest('[data-ctx-menu]')) return
      setContextMenu(null)
    }
    window.addEventListener('mousedown', close)
    return () => window.removeEventListener('mousedown', close)
  }, [])

  async function fetchCollections() {
    setLoading(true)
    try {
      const wsId = activeWorkspaceId ?? ''
      const res = await apiFetch(`/api/collections?workspaceId=${wsId}`)
      const data: Collection[] = await res.json()
      setCollections(data)
    } finally {
      setLoading(false)
    }
  }

  async function createCollection() {
    if (!newName.trim()) return
    const wsId = activeWorkspaceId ?? ''
    const res = await apiFetch(`/api/collections?workspaceId=${wsId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: newName.trim(), color: newColor, description: '' }),
    })
    const col: Collection = await res.json()
    upsertCollection(col)
    setExpanded(s => new Set([...s, col.id]))
    setCreating(false)
    setNewName('')
  }

  async function deleteCollection(id: string) {
    await apiFetch(`/api/collections/${id}`, { method: 'DELETE' })
    removeCollection(id)
    setContextMenu(null)
  }

  async function deleteRequest(id: string) {
    await apiFetch(`/api/collections/requests/${id}`, { method: 'DELETE' })
    removeRequest(id)
    setContextMenu(null)
  }

  async function renameItem(id: string, name: string, isCollection: boolean, collectionId?: string) {
    if (isCollection) {
      const col = collections.find(c => c.id === id)!
      const res = await apiFetch(`/api/collections/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, description: col.description, color: col.color }),
      })
      upsertCollection(await res.json())
    } else if (collectionId) {
      const req = collections.flatMap(c => c.requests).find(r => r.id === id)!
      await apiFetch(`/api/collections/requests/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...req, name }),
      })
      upsertRequest({ ...req, name })
    }
    setEditingId(null)
  }

  async function exportCollections() {
    const res = await apiFetch('/api/collections/export')
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a'); a.href = url; a.download = 'collections.json'; a.click()
    URL.revokeObjectURL(url)
  }

  async function importCollections(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]; if (!file) return
    try {
      const json = JSON.parse(await file.text())
      await apiFetch('/api/collections/import', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(json) })
      fetchCollections()
    } catch {}
    e.target.value = ''
  }

  return (
    <div className={styles.panel}>
      {/* Toolbar */}
      <div className={styles.toolbar}>
        <span className={styles.toolbarTitle}>Collections</span>
        <div className={styles.toolbarActions}>
          <button className={styles.toolbarBtn} onClick={exportCollections} title="Export"><Download size={13} /></button>
          <button className={styles.toolbarBtn} onClick={() => importRef.current?.click()} title="Import"><Upload size={13} /></button>
          <button className={styles.toolbarBtn} onClick={() => setCreating(true)} title="New collection"><Plus size={13} /></button>
          <input ref={importRef} type="file" accept=".json" style={{ display: 'none' }} onChange={importCollections} />
        </div>
      </div>

      {/* New collection form */}
      {creating && (
        <div className={styles.createForm}>
          <input
            className={styles.createInput} value={newName}
            onChange={e => setNewName(e.target.value)} autoFocus
            onKeyDown={e => { if (e.key === 'Enter') createCollection(); if (e.key === 'Escape') setCreating(false) }}
            placeholder="Collection name"
          />
          <div className={styles.colorRow}>
            {COLORS.map(c => (
              <button key={c} className={`${styles.colorDot} ${newColor === c ? styles.colorDotActive : ''}`}
                style={{ background: c }} onClick={() => setNewColor(c)} />
            ))}
          </div>
          <div className={styles.createBtns}>
            <button className={styles.cancelBtn} onClick={() => setCreating(false)}>Cancel</button>
            <button className={styles.confirmBtn} onClick={createCollection}>Create</button>
          </div>
        </div>
      )}

      {/* List */}
      <div className={styles.list}>
        {loading && <div className={styles.empty}><span>Loading…</span></div>}

        {!loading && collections.length === 0 && (
          <div className={styles.empty}>
            <FolderOpen size={28} className={styles.emptyIcon} />
            <span>No collections yet</span>
            <button className={styles.emptyAction} onClick={() => setCreating(true)}>
              <Plus size={12} /> New Collection
            </button>
          </div>
        )}

        {collections.map(col => (
          <div key={col.id} className={styles.collectionGroup}>
            {/* Collection header */}
            <div
              className={styles.collectionHeader}
              onClick={() => setExpanded(s => { const n = new Set(s); n.has(col.id) ? n.delete(col.id) : n.add(col.id); return n })}
              onContextMenu={e => { e.preventDefault(); setContextMenu({ type: 'collection', id: col.id, x: e.clientX, y: e.clientY }) }}
            >
              <span className={styles.expandIcon}>
                {expanded.has(col.id) ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
              </span>
              <span className={styles.collectionDot} style={{ background: col.color }} />

              {editingId === col.id ? (
                <input
                  className={styles.renameInput} value={editingName}
                  onChange={e => setEditingName(e.target.value)}
                  onBlur={() => renameItem(col.id, editingName, true)}
                  onKeyDown={e => { if (e.key === 'Enter') renameItem(col.id, editingName, true); if (e.key === 'Escape') setEditingId(null) }}
                  autoFocus onClick={e => e.stopPropagation()}
                />
              ) : (
                <span className={styles.collectionName}>{col.name}</span>
              )}

              <span className={styles.requestCount}>{col.requests.length}</span>
              <button className={styles.moreBtn}
                onClick={e => { e.stopPropagation(); setContextMenu({ type: 'collection', id: col.id, x: e.clientX, y: e.clientY }) }}
              >
                <MoreHorizontal size={13} />
              </button>
            </div>

            {/* Requests */}
            {expanded.has(col.id) && (
              <div className={styles.requestList}>
                {col.requests.length === 0 && (
                  <div className={styles.noRequests}>No saved requests</div>
                )}
                {col.requests.map(req => (
                  <div
                    key={req.id}
                    className={styles.requestRow}
                    onMouseDown={() => openSavedRequest(req)}
                    onContextMenu={e => { e.preventDefault(); e.stopPropagation(); setContextMenu({ type: 'request', id: req.id, collectionId: col.id, x: e.clientX, y: e.clientY }) }}
                  >
                    <span className={styles.methodBadge}>UNARY</span>
                    {editingId === req.id ? (
                      <input
                        className={styles.renameInput} value={editingName}
                        onChange={e => setEditingName(e.target.value)}
                        onBlur={() => renameItem(req.id, editingName, false, col.id)}
                        onKeyDown={e => { if (e.key === 'Enter') renameItem(req.id, editingName, false, col.id); if (e.key === 'Escape') setEditingId(null) }}
                        autoFocus onMouseDown={e => e.stopPropagation()}
                      />
                    ) : (
                      <span className={styles.requestName}>{req.name}</span>
                    )}
                    <span className={styles.methodName}>{req.method}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Context menu */}
      {contextMenu && (
        <div data-ctx-menu className={styles.contextMenu} style={{ top: contextMenu.y, left: contextMenu.x }}>
          {contextMenu.type === 'collection' ? (
            <>
              <button className={styles.ctxItem} onClick={() => {
                const col = collections.find(c => c.id === contextMenu.id)!
                setEditingId(col.id); setEditingName(col.name); setContextMenu(null)
              }}><Edit2 size={12} /> Rename</button>
              <div className={styles.ctxDivider} />
              <button className={`${styles.ctxItem} ${styles.ctxDanger}`} onClick={() => deleteCollection(contextMenu.id)}>
                <Trash2 size={12} /> Delete collection
              </button>
            </>
          ) : (
            <>
              <button className={styles.ctxItem} onClick={() => {
                const req = collections.flatMap(c => c.requests).find(r => r.id === contextMenu.id)
                if (req) openSavedRequest(req)
                setContextMenu(null)
              }}><Plus size={12} /> Open in new tab</button>
              <button className={styles.ctxItem} onClick={() => {
                const req = collections.flatMap(c => c.requests).find(r => r.id === contextMenu.id)
                if (req) { setEditingId(req.id); setEditingName(req.name) }
                setContextMenu(null)
              }}><Edit2 size={12} /> Rename</button>
              <div className={styles.ctxDivider} />
              <button className={`${styles.ctxItem} ${styles.ctxDanger}`} onClick={() => deleteRequest(contextMenu.id)}>
                <Trash2 size={12} /> Delete
              </button>
            </>
          )}
        </div>
      )}
    </div>
  )
}
