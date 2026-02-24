import { useEffect, useState } from 'react'
import { Trash2, Clock, CheckCircle, XCircle, RefreshCw, ChevronRight } from 'lucide-react'
import { useHistoryStore, useServiceStore, useTabStore } from '../../stores'
import { useAuthStore } from '../../stores/authStore'
import { usePermission } from '../../hooks/usePermission'
import { api } from '../../api/client'
import type { HistoryEntry, MethodDescriptor } from '../../types'
import styles from './HistoryPanel.module.css'

export function HistoryPanel() {
  const { can } = usePermission()
  const { entries, setEntries, removeEntry, clear } = useHistoryStore()
  const activeWorkspaceId = useAuthStore(s => s.activeWorkspaceId)
  const { services } = useServiceStore()
  const openFromMethod = useTabStore(s => s.openFromMethod)
  const [loading, setLoading] = useState(false)
  const [expanded, setExpanded] = useState<string | null>(null)

  async function load() {
    if (!activeWorkspaceId) return
    setLoading(true)
    try {
      const data = await api.history.list(activeWorkspaceId)
      setEntries(data)
    } catch {
      // silently fail — workspace may not be set yet
    } finally {
      setLoading(false)
    }
  }

  async function deleteEntry(id: string) {
    if (!activeWorkspaceId) return
    await api.history.delete(id, activeWorkspaceId)
    removeEntry(id)
  }

  async function clearAll() {
    if (!activeWorkspaceId) return
    await api.history.clear(activeWorkspaceId)
    clear()
  }

  function reuse(entry: HistoryEntry) {
    const body = typeof entry.requestBody === 'string'
      ? entry.requestBody
      : JSON.stringify(entry.requestBody, null, 2)
    openFromMethod(entry.service, entry.method, body)
  }

  // Reload when workspace changes
  useEffect(() => { load() }, [activeWorkspaceId])

  if (!can('history:read')) {
    return (
      <div className={styles.panel}>
        <div className={styles.restricted}>
          <Clock size={20} className={styles.restrictedIcon} />
          <span>History not available for your role</span>
        </div>
      </div>
    )
  }

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <span className={styles.headerTitle}>History</span>
        <div className={styles.headerActions}>
          <button className={styles.iconBtn} onClick={load} title="Refresh" disabled={loading}>
            <RefreshCw size={12} className={loading ? styles.spinning : ''} />
          </button>
          {entries.length > 0 && (
            <button className={styles.clearBtn} onClick={clearAll} title="Clear all">
              <Trash2 size={12} /> Clear
            </button>
          )}
        </div>
      </div>

      <div className={styles.list}>
        {entries.length === 0 && (
          <div className={styles.empty}>
            {activeWorkspaceId ? 'No history yet' : 'Select a workspace to view history'}
          </div>
        )}
        {entries.map((entry) => {
          const isOk = entry.status === 'OK'
          const isExpanded = expanded === entry.id
          const shortSvc = entry.service.split('.').pop() || entry.service

          return (
            <div key={entry.id} className={styles.entryGroup}>
              <div className={styles.entry} onClick={() => setExpanded(isExpanded ? null : entry.id)}>
                <div className={styles.entryLeft}>
                  {isOk
                    ? <CheckCircle size={12} className={styles.iconOk} />
                    : <XCircle size={12} className={styles.iconErr} />
                  }
                  <div className={styles.entryInfo}>
                    <span className={styles.method}>{entry.method}</span>
                    <span className={styles.service}>{shortSvc}</span>
                  </div>
                </div>
                <div className={styles.entryRight}>
                  <span className={styles.duration}>{entry.durationMs}ms</span>
                  <span className={styles.time}>{formatTime(entry.createdAt ?? entry.timestamp)}</span>
                  <ChevronRight size={11} className={`${styles.chevron} ${isExpanded ? styles.chevronOpen : ''}`} />
                </div>
              </div>

              {isExpanded && (
                <div className={styles.entryDetail}>
                  <div className={styles.detailMeta}>
                    {entry.connAddress && (
                      <span className={styles.detailAddr}>{entry.connAddress}</span>
                    )}
                    {entry.userName && (
                      <span className={styles.detailUser}>{entry.userName}</span>
                    )}
                  </div>
                  <div className={styles.detailActions}>
                    <button className={styles.reuseBtn} onClick={() => reuse(entry)}>
                      ↑ Reuse request
                    </button>
                    <button className={styles.deleteBtn} onClick={() => deleteEntry(entry.id)}>
                      <Trash2 size={11} />
                    </button>
                  </div>
                  <div className={styles.detailBlock}>
                    <span className={styles.detailLabel}>Request</span>
                    <pre className={styles.detailPre}>{String(formatBody(entry.requestBody))}</pre>
                  </div>
                  {!!entry.responseBody && (
                    <div className={styles.detailBlock}>
                      <span className={styles.detailLabel}>Response</span>
                      <pre className={styles.detailPre}>{String(formatBody(entry.responseBody))}</pre>
                    </div>
                  )}
                  {entry.status !== 'OK' && (
                    <div className={styles.detailBlock}>
                      <span className={styles.detailLabelErr}>Status</span>
                      <pre className={styles.detailPreErr}>{entry.status}</pre>
                    </div>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function formatBody(body: unknown): string {
  if (!body) return ''
  if (typeof body === 'string') {
    try { return JSON.stringify(JSON.parse(body), null, 2) } catch { return body }
  }
  return JSON.stringify(body, null, 2)
}

function formatTime(ts: string | undefined) {
  if (!ts) return ''
  const d = new Date(ts)
  return d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
