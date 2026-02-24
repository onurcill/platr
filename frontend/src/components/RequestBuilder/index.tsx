import { useState, useEffect } from 'react'
import Editor from '@monaco-editor/react'
import { Send, Square, Plus, Trash2, Save, Sparkles, BarChart2, X } from 'lucide-react'
import {
  useConnectionStore, useServiceStore, useThemeStore,
  useHistoryStore, useEnvironmentStore, useCollectionStore,
  useTabStore,
} from '../../stores'
import { apiFetch } from '../../stores/authStore'
import { usePermission } from '../../hooks/usePermission'
import type { RequestTab } from '../../stores'
import { api, createStream } from '../../api/client'
import { AiAssistant } from '../AiAssistant'
import { LoadTestPanel } from '../LoadTestPanel'
import styles from './RequestBuilder.module.css'

// ── Outer shell — subscribes to store, passes active tab down ─────────────────
export function RequestBuilder() {
  const tabs      = useTabStore(s => s.tabs)
  const activeTabId = useTabStore(s => s.activeTabId)
  const tab = tabs.find(t => t.id === activeTabId) ?? null

  const { selectedMethod } = useServiceStore()
  const { theme } = useThemeStore()

  if (!tab) return null

  // key=tab.id → every tab switch completely remounts TabContent
  // so all useState (metaRows, bulkMode, bulkText, panelTab) reset cleanly
  return (
    <TabContent
      key={tab.id}
      tab={tab}
      selectedMethod={selectedMethod}
      monacoTheme={theme === 'dark' ? 'vs-dark' : 'vs'}
    />
  )
}

// ── Inner content — fully remounts on tab change ──────────────────────────────
// Build a JSON template object from a MessageSchema
function schemaToTemplate(schema: any, depth = 0): any {
  if (!schema || depth > 4) return {}
  const out: Record<string, any> = {}
  for (const [key, field] of Object.entries<any>(schema.fields || {})) {
    const type: string = field.type || ''
    let val: any = ''
    if (field.repeated) {
      val = []
    } else if (type === 'bool') {
      val = false
    } else if (type === 'string') {
      val = ''
    } else if (['int32','int64','uint32','uint64','float','double'].includes(type)) {
      val = 0
    } else if (type === 'bytes') {
      val = ''
    } else if (type.startsWith('enum:')) {
      val = 0
    } else if (type.startsWith('message:') && field.nested) {
      val = schemaToTemplate(field.nested, depth + 1)
    }
    out[key] = val
  }
  return out
}

function TabContent({ tab, selectedMethod, monacoTheme }: {
  tab: RequestTab
  selectedMethod: any
  monacoTheme: string
}) {
  const { activeConnectionId } = useConnectionStore()
  const connId = tab.connectionId || activeConnectionId
  const { activeEnvId, resolveVariables } = useEnvironmentStore()
  const { collections, upsertRequest } = useCollectionStore()
  const { addEntry } = useHistoryStore()

  const { can } = usePermission()
  const setTabBody          = useTabStore(s => s.setTabBody)
  const setTabMetadata      = useTabStore(s => s.setTabMetadata)
  const setTabResponse      = useTabStore(s => s.setTabResponse)
  const setTabLoading       = useTabStore(s => s.setTabLoading)
  const setTabStreaming      = useTabStore(s => s.setTabStreaming)
  const addTabStreamMessage = useTabStore(s => s.addTabStreamMessage)
  const clearTabStream      = useTabStore(s => s.clearTabStream)
  const markTabClean        = useTabStore(s => s.markTabClean)

  // ── Local state — resets on every tab mount ──
  const initRows = Object.entries(tab.metadata || {}).map(([key, value]) => ({ key, value }))
  const [panelTab,    setPanelTab]    = useState<'body' | 'metadata'>('body')
  const [metaRows,    setMetaRowsRaw] = useState<{ key: string; value: string }[]>(
    initRows.length > 0 ? initRows : [{ key: '', value: '' }]
  )
  const [bulkMode,    setBulkMode]    = useState(false)
  const [bulkText,    setBulkText]    = useState('')
  const [streamCtrl,  setStreamCtrl]  = useState<{ send: (p: unknown) => void; close: () => void } | null>(null)
  const [showSave,    setShowSave]    = useState(false)
  const [rightPanel,  setRightPanel]  = useState<'ai' | 'loadtest' | null>(null)

  const service   = tab.service || ''
  const method    = tab.method  || ''
  const hasMethod = !!method
  const isStream  = selectedMethod?.clientStreaming || selectedMethod?.serverStreaming


  // Auto-fill body from schema when method changes and body is empty/default
  useEffect(() => {
    if (!selectedMethod?.inputSchema) return
    const current = tab.requestBody?.trim()
    if (current && current !== '{}' && current !== '') return // don't overwrite user content
    const template = schemaToTemplate(selectedMethod.inputSchema)
    if (template !== '{}') {
      setTabBody(tab.id, JSON.stringify(template, null, 2))
    }
  }, [selectedMethod?.fullName])

  // ── Helpers ──
  function setMetaRows(rows: { key: string; value: string }[]) {
    setMetaRowsRaw(rows)
    const md: Record<string, string> = {}
    rows.forEach(({ key, value }) => { if (key.trim()) md[key.trim()] = value })
    setTabMetadata(tab.id, md)
  }

  function buildMetadata(): Record<string, string> {
    if (bulkMode) {
      const md: Record<string, string> = {}
      bulkText.split('\n').forEach(line => {
        const i = line.indexOf(':')
        if (i > 0) md[line.slice(0, i).trim()] = resolveVariables(line.slice(i + 1).trim())
      })
      return md
    }
    const md: Record<string, string> = {}
    metaRows.forEach(({ key, value }) => { if (key.trim()) md[key.trim()] = resolveVariables(value) })
    return md
  }

  function rowsToBulk(rows: { key: string; value: string }[]) {
    return rows.filter(r => r.key.trim()).map(r => `${r.key}: ${r.value}`).join('\n')
  }

  function bulkToRows(text: string) {
    const rows = text.split('\n').map(line => {
      const i = line.indexOf(':')
      if (i < 0) return { key: line.trim(), value: '' }
      return { key: line.slice(0, i).trim(), value: line.slice(i + 1).trim() }
    }).filter(r => r.key)
    return rows.length > 0 ? rows : [{ key: '', value: '' }]
  }

  function toggleBulk() {
    if (!bulkMode) {
      setBulkText(rowsToBulk(metaRows))
    } else {
      setMetaRows(bulkToRows(bulkText))
    }
    setBulkMode(b => !b)
  }

  // ── Send ──
  async function sendUnary() {
    if (!connId || !method) return
    setTabLoading(tab.id, true)
    clearTabStream(tab.id)
    try {
      let payload = {}
      try { payload = JSON.parse(resolveVariables(tab.requestBody)) } catch { payload = {} }
      const res = await api.invoke.unary(connId, {
        service, method, payload, metadata: buildMetadata(),
      }, activeEnvId)
      setTabResponse(tab.id, res.payload, res.status, res.durationMs, res.headers, res.trailers)
      addEntry({
        id: res.historyId, connectionId: connId,
        workspaceId: '', userId: '', connAddress: '',
        service, method, requestBody: payload,
        responseBody: res.payload, status: res.status,
        durationMs: res.durationMs,
        createdAt: new Date().toISOString(),
        timestamp: new Date().toISOString(),
      })
    } catch (e) {
      setTabResponse(tab.id, null, e instanceof Error ? e.message : 'Error', 0, {}, {})
    } finally {
      setTabLoading(tab.id, false)
    }
  }

  function startStream() {
    if (!connId || !method) return
    clearTabStream(tab.id)
    setTabStreaming(tab.id, true)
    const ctrl = createStream(
      connId, { service, method, metadata: buildMetadata() },
      (msg) => addTabStreamMessage(tab.id, msg),
      (trailers) => { setTabStreaming(tab.id, false); if (trailers) addTabStreamMessage(tab.id, { _trailers: trailers }) },
      (err) => { setTabStreaming(tab.id, false); addTabStreamMessage(tab.id, { _error: err }) },
    )
    setStreamCtrl(ctrl)
    if (selectedMethod && !selectedMethod.clientStreaming) {
      let payload = {}
      try { payload = JSON.parse(resolveVariables(tab.requestBody)) } catch { payload = {} }
      ctrl.send(payload)
    }
  }

  function stopStream() { streamCtrl?.close(); setStreamCtrl(null); setTabStreaming(tab.id, false) }
  function sendStreamMessage() {
    if (!streamCtrl) return
    let payload = {}
    try { payload = JSON.parse(tab.requestBody) } catch { payload = {} }
    streamCtrl.send(payload)
  }

  const activeMetaCount = bulkMode
    ? bulkText.split('\n').filter(l => l.includes(':')).length
    : metaRows.filter(r => r.key.trim()).length

  // ── Render ──
  return (
    <div className={styles.panel}>
      {!hasMethod ? (
        <div className={styles.empty}>
          <p>Select a method from Services or open a saved request</p>
        </div>
      ) : (
        <div className={styles.splitLayout}>
          {/* ── Left: request editor ── */}
          <div className={styles.editorPane}>
          {/* Method header */}
          <div className={styles.methodHeader}>
            <div className={styles.methodPath}>
              <span className={styles.methodService}>{service.split('.').pop()}</span>
              <span className={styles.methodSep}>/</span>
              <span className={styles.methodName}>{method}</span>
            </div>
            {selectedMethod && (
              <div className={styles.methodMeta}>
                <span className={styles.inputType}>{selectedMethod.inputType?.split('.').pop()}</span>
                <span className={styles.arrow}>→</span>
                <span className={styles.outputType}>{selectedMethod.outputType?.split('.').pop()}</span>
              </div>
            )}
            {/* Right panel toggles */}
            <div className={styles.rightPanelBtns}>
              <button
                className={`${styles.rpBtn} ${rightPanel === 'ai' ? styles.rpBtnActive : ''}`}
                onClick={() => setRightPanel(p => p === 'ai' ? null : 'ai')}
                title="AI Assistant"
              >
                <Sparkles size={12} />
                <span>AI</span>
              </button>
              <button
                className={`${styles.rpBtn} ${rightPanel === 'loadtest' ? styles.rpBtnActive : ''}`}
                onClick={() => setRightPanel(p => p === 'loadtest' ? null : 'loadtest')}
                title="Load Test"
              >
                <BarChart2 size={12} />
                <span>Load Test</span>
              </button>
            </div>
          </div>

          {/* Body / Metadata tabs */}
          <div className={styles.tabs}>
            <button className={`${styles.tab} ${panelTab === 'body' ? styles.tabActive : ''}`} onClick={() => setPanelTab('body')}>
              Body
            </button>
            <button className={`${styles.tab} ${panelTab === 'metadata' ? styles.tabActive : ''}`} onClick={() => setPanelTab('metadata')}>
              Metadata
              {activeMetaCount > 0 && <span className={styles.tabBadge}>{activeMetaCount}</span>}
            </button>
          </div>

          {/* Editor area */}
          <div className={styles.editorWrap}>
            {panelTab === 'body' ? (
              <Editor
                height="100%"
                language="json"
                value={tab.requestBody}
                onChange={(v) => setTabBody(tab.id, v || '{}')}
                theme={monacoTheme}
                options={{
                  minimap: { enabled: false }, fontSize: 13,
                  fontFamily: 'JetBrains Mono', lineNumbers: 'on',
                  scrollBeyondLastLine: false, wordWrap: 'on',
                  renderLineHighlight: 'line', padding: { top: 12, bottom: 12 },
                  folding: true, formatOnPaste: true,
                }}
              />
            ) : (
              <div className={styles.metaEditor}>
                {/* Toolbar */}
                <div className={styles.metaToolbar}>
                  <span className={styles.metaToolbarHint}>
                    {bulkMode ? 'key: value  per line' : `${activeMetaCount} headers`}
                  </span>
                  <button
                    className={`${styles.bulkToggle} ${bulkMode ? styles.bulkToggleActive : ''}`}
                    onClick={toggleBulk}
                  >
                    {bulkMode ? 'Table' : 'Bulk Edit'}
                  </button>
                </div>

                {bulkMode ? (
                  <textarea
                    className={styles.bulkArea}
                    value={bulkText}
                    onChange={e => setBulkText(e.target.value)}
                    placeholder={'Authorization: Bearer token\nX-Tenant-Id: my-tenant'}
                    spellCheck={false}
                    autoFocus
                  />
                ) : (
                  <div className={styles.metaRowsArea}>
                    {metaRows.map((row, i) => (
                      <div key={i} className={styles.metaRow}>
                        <input
                          className={styles.metaKey} value={row.key} placeholder="key"
                          onChange={(e) => { const n = [...metaRows]; n[i] = { ...n[i], key: e.target.value }; setMetaRows(n) }}
                        />
                        <span className={styles.metaColon}>:</span>
                        <input
                          className={styles.metaVal} value={row.value} placeholder="value"
                          onChange={(e) => { const n = [...metaRows]; n[i] = { ...n[i], value: e.target.value }; setMetaRows(n) }}
                        />
                        <button className={styles.metaDelete} onClick={() => setMetaRows(metaRows.filter((_, j) => j !== i))}>
                          <Trash2 size={12} />
                        </button>
                      </div>
                    ))}
                    <button className={styles.metaAdd} onClick={() => setMetaRows([...metaRows, { key: '', value: '' }])}>
                      <Plus size={12} /> Add header
                    </button>
                  </div>
                )}
              </div>
            )}
          </div>

          {/* Send bar */}
          <div className={styles.sendBar}>
            {can('request:create') && <button
              className={`${styles.saveBtn}${tab.savedRequestId && tab.isDirty ? ` ${styles.saveBtnDirty}` : ''}`}
              onClick={async () => {
                if (tab.savedRequestId) {
                  // Update existing saved request in-place
                  const res = await apiFetch(`/api/collections/requests/${tab.savedRequestId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                      service, method,
                      body: tab.requestBody,
                      metadata: buildMetadata(),
                      envId: activeEnvId ?? undefined,
                    }),
                  })
                  if (res.ok) { upsertRequest(await res.json()); markTabClean(tab.id) }
                } else {
                  setShowSave(true)
                }
              }}
              title={tab.savedRequestId ? 'Update saved request' : 'Save to collection'}
            >
              <Save size={13} />
            </button>}
            {!isStream ? (
              <button className={styles.sendBtn} onClick={sendUnary} disabled={tab.isLoading || !connId || !can('invoke')}>
                {tab.isLoading ? <span className={styles.spinner} /> : <Send size={13} />}
                {tab.isLoading ? 'Sending…' : 'Send'}
              </button>
            ) : !tab.isStreaming ? (
              <button className={styles.sendBtn} onClick={startStream} disabled={!connId || !can('invoke')}>
                <Send size={13} /> Start Stream
              </button>
            ) : (
              <>
                {selectedMethod?.clientStreaming && (
                  <button className={styles.sendMsgBtn} onClick={sendStreamMessage}><Send size={13} /> Send Message</button>
                )}
                <button className={styles.stopBtn} onClick={stopStream}><Square size={13} /> Stop</button>
              </>
            )}
          </div>
          </div>{/* end editorPane */}

          {/* ── Right: AI / Load Test panel ── */}
          {rightPanel && (
            <div className={styles.rightPanel}>
              {rightPanel === 'ai' && (
                <AiAssistant
                  method={selectedMethod ?? null}
                  service={service}
                  currentPayload={tab.requestBody}
                  currentResponse={tab.response}
                  lastStatus={tab.responseStatus ?? ''}
                  lastDurationMs={tab.responseDuration ?? 0}
                  onApplyPayload={(p) => setTabBody(tab.id, p)}
                />
              )}
              {rightPanel === 'loadtest' && connId && (
                <LoadTestPanel
                  connId={connId}
                  service={service}
                  method={method}
                  payload={tab.requestBody}
                  metadata={buildMetadata()}
                  onClose={() => setRightPanel(null)}
                />
              )}
            </div>
          )}
        </div>
      )}

      {/* Save modal — only shown for new (unsaved) requests */}
      {showSave && (
        <SaveToCollectionModal
          collections={collections}
          tabService={service} tabMethod={method}
          requestBody={tab.requestBody} activeEnvId={activeEnvId}
          onSave={async (collectionId, name, description) => {
            const res = await apiFetch(`/api/collections/${collectionId}/requests`, {
              method: 'POST',
              body: JSON.stringify({ name, description, service, method, body: tab.requestBody, metadata: buildMetadata(), connAddress: '', envId: activeEnvId }),
            })
            const saved = await res.json()
            upsertRequest(saved); markTabClean(tab.id); setShowSave(false)
          }}
          onClose={() => setShowSave(false)}
        />
      )}
    </div>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function schemaToExample(fields: Record<string, { type: string; repeated: boolean; nested?: { fields: Record<string, unknown> } }>): Record<string, unknown> {
  const obj: Record<string, unknown> = {}
  for (const [k, f] of Object.entries(fields)) {
    let val: unknown = defaultForType(f.type)
    if (f.nested) val = schemaToExample(f.nested.fields as any)
    obj[k] = f.repeated ? [val] : val
  }
  return obj
}

function defaultForType(type: string): unknown {
  if (type === 'string') return ''
  if (type === 'bool') return false
  if (type.startsWith('int') || type.startsWith('uint') || type === 'float' || type === 'double') return 0
  return null
}

// ── Save Modal ────────────────────────────────────────────────────────────────

const SAVE_COLORS = ['#4ade80','#60a5fa','#f472b6','#fb923c','#a78bfa','#34d399','#fbbf24','#f87171']

function SaveToCollectionModal({ collections, tabService, tabMethod, requestBody, activeEnvId, onSave, onClose }: {
  collections: any[]; tabService: string; tabMethod: string; requestBody: string
  activeEnvId: string | null; onSave: (col: string, name: string, desc: string) => void; onClose: () => void
}) {
  const { upsertCollection } = useCollectionStore()
  const [name,         setName]         = useState(tabMethod || 'New Request')
  const [description,  setDescription]  = useState('')
  const [collectionId, setCollectionId] = useState(collections[0]?.id ?? '')
  const [creatingNew,  setCreatingNew]  = useState(collections.length === 0)
  const [newColName,   setNewColName]   = useState('')
  const [newColColor,  setNewColColor]  = useState(SAVE_COLORS[0])

  async function handleSave() {
    let targetId = collectionId
    if (creatingNew) {
      if (!newColName.trim()) return
      const res = await apiFetch('/api/collections', {
        method: 'POST',
        body: JSON.stringify({ name: newColName.trim(), color: newColColor, description: '' }),
      })
      const col = await res.json(); upsertCollection(col); targetId = col.id
    }
    onSave(targetId, name, description)
  }

  return (
    <div className={styles.modalOverlay} onClick={e => e.target === e.currentTarget && onClose()}>
      <div className={styles.modal}>
        <div className={styles.modalHeader}>
          <Save size={14} className={styles.modalIcon} />
          <span>Save to Collection</span>
          <button className={styles.modalClose} onClick={onClose}>✕</button>
        </div>
        <div className={styles.modalBody}>
          {tabMethod && (
            <div className={styles.requestPreview}>
              <span className={styles.previewBadge}>UNARY</span>
              <span className={styles.previewMethod}>{tabService}/{tabMethod}</span>
            </div>
          )}
          <label className={styles.fieldLabel}>Request name</label>
          <input className={styles.fieldInput} value={name} onChange={e => setName(e.target.value)} autoFocus />
          <label className={styles.fieldLabel}>Description</label>
          <input className={styles.fieldInput} value={description} onChange={e => setDescription(e.target.value)} placeholder="optional" />
          <label className={styles.fieldLabel}>Collection</label>
          {!creatingNew && collections.length > 0 ? (
            <>
              <select className={styles.fieldSelect} value={collectionId} onChange={e => setCollectionId(e.target.value)}>
                {collections.map((c: any) => <option key={c.id} value={c.id}>{c.name}</option>)}
              </select>
              <button className={styles.newColLink} onClick={() => setCreatingNew(true)}>+ New collection</button>
            </>
          ) : (
            <>
              <input className={styles.fieldInput} value={newColName} onChange={e => setNewColName(e.target.value)} placeholder="Collection name" />
              <div className={styles.colorRowModal}>
                {SAVE_COLORS.map(c => (
                  <button key={c} onClick={() => setNewColColor(c)} style={{ background: c, width: 16, height: 16, borderRadius: '50%', border: newColColor === c ? '2px solid white' : '2px solid transparent' }} />
                ))}
              </div>
              {collections.length > 0 && <button className={styles.newColLink} onClick={() => setCreatingNew(false)}>← Back</button>}
            </>
          )}
        </div>
        <div className={styles.modalFooter}>
          <button className={styles.cancelModalBtn} onClick={onClose}>Cancel</button>
          <button className={styles.saveFinalBtn} onClick={handleSave} disabled={!name.trim() || (!collectionId && !newColName.trim())}>
            <Save size={12} /> Save
          </button>
        </div>
      </div>
    </div>
  )
}
