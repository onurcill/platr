import { useState, useEffect, useRef, useCallback } from 'react'
import {
  FolderOpen, Layers, Cpu, Clock, ChevronLeft,
  Plus, Trash2, Copy, Eye, EyeOff, Check, Edit2, AlertCircle, Zap,
  GripHorizontal,
} from 'lucide-react'
import { CollectionPanel } from '../CollectionPanel'
import { ServiceExplorer } from '../ServiceExplorer'
import { HistoryPanel } from '../HistoryPanel'
import { useEnvironmentStore, useAuthStore } from '../../stores'
import { api } from '../../api/client'
import type { Environment, EnvVar } from '../../stores/environmentStore'
import styles from './Sidebar.module.css'

type SideTab = 'collections' | 'services' | 'environments' | 'history'

const COLORS = ['#818cf8','#34d399','#60a5fa','#f472b6','#fb923c','#a78bfa','#fbbf24','#f87171']

const TABS: { id: SideTab; icon: React.ReactNode; label: string }[] = [
  { id: 'collections',  icon: <FolderOpen size={17} />, label: 'Collections'  },
  { id: 'services',     icon: <Cpu        size={17} />, label: 'Services'     },
  { id: 'environments', icon: <Layers     size={17} />, label: 'Environments' },
  { id: 'history',      icon: <Clock      size={17} />, label: 'History'      },
]

interface SidebarProps {
  panelWidth?: number  // controlled from App; undefined = self-managed (legacy)
}

export function Sidebar({ panelWidth }: SidebarProps = {}) {
  const [active, setActive]       = useState<SideTab>('collections')
  const [collapsed, setCollapsed] = useState(false)

  // When parent controls width, derive collapsed from it
  const isCollapsed = panelWidth !== undefined ? panelWidth < 20 : collapsed

  function toggle(tab: SideTab) {
    if (!isCollapsed && active === tab) setCollapsed(true)
    else { setActive(tab); setCollapsed(false) }
  }

  return (
    <div className={`${styles.sidebar} ${isCollapsed ? styles.collapsed : ''}`}>

      {/* ── Icon rail ── */}
      <nav className={styles.rail}>
        {TABS.map(t => (
          <button
            key={t.id}
            className={`${styles.railBtn} ${!isCollapsed && active === t.id ? styles.railBtnActive : ''}`}
            onClick={() => toggle(t.id)}
            title={t.label}
          >
            {t.icon}
            <span className={styles.railLabel}>{t.label}</span>
          </button>
        ))}
        <button
          className={`${styles.railBtn} ${styles.collapseBtn}`}
          onClick={() => setCollapsed(c => !c)}
          title={isCollapsed ? 'Expand' : 'Collapse'}
        >
          <ChevronLeft size={15} className={`${styles.collapseIcon} ${isCollapsed ? styles.flipped : ''}`} />
        </button>
      </nav>

      {/* ── Panel (width controlled by parent drag or internal state) ── */}
      {!isCollapsed && (
        <div
          className={styles.panel}
          style={panelWidth !== undefined ? { width: panelWidth } : undefined}
        >
          {active === 'collections'  && <CollectionPanel />}
          {active === 'services'     && <ServiceExplorer />}
          {active === 'environments' && <EnvPanel />}
          {active === 'history'      && <HistoryPanel />}
        </div>
      )}
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// Environment panel — clean single-list design
// Click env row  →  open its editor below
// Click ⚡ icon  →  set as active (independent from editing)
// ─────────────────────────────────────────────────────────────────────────────

function EnvPanel() {
  const { environments, activeEnvId, setEnvironments, setActiveEnv,
          upsertEnvironment, removeEnvironment } = useEnvironmentStore()
  const activeWorkspaceId = useAuthStore(s => s.activeWorkspaceId)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName]   = useState('')
  const [newColor, setNewColor] = useState(COLORS[0])
  const [loading, setLoading]   = useState(false)
  const [error, setError]       = useState<string | null>(null)

  // Auto-select first env or active env
  useEffect(() => {
    const target = activeEnvId ?? environments[0]?.id ?? null
    setSelectedId(target)
  }, [activeWorkspaceId])

  useEffect(() => { fetchEnvs() }, [activeWorkspaceId])

  async function fetchEnvs() {
    if (!activeWorkspaceId) return
    try {
      const data = await api.environments.list(activeWorkspaceId)
      setEnvironments(data)
      if (!selectedId && data.length > 0) setSelectedId(activeEnvId ?? data[0].id)
    } catch { setError('Failed to load') }
  }

  async function createEnv() {
    if (!newName.trim() || !activeWorkspaceId) return
    setLoading(true)
    try {
      const env = await api.environments.create(activeWorkspaceId, { name: newName.trim(), color: newColor })
      upsertEnvironment(env)
      setSelectedId(env.id)
      setCreating(false); setNewName('')
    } catch { setError('Failed to create') }
    finally { setLoading(false) }
  }

  async function deleteEnv(id: string) {
    try {
      await api.environments.delete(id)
      removeEnvironment(id)
      if (selectedId === id) setSelectedId(environments.find(e => e.id !== id)?.id ?? null)
      if (activeEnvId === id) setActiveEnv(null)
    } catch { setError('Failed to delete') }
  }

  async function dupEnv(id: string) {
    try {
      const env = await api.environments.duplicate(id)
      upsertEnvironment(env); setSelectedId(env.id)
    } catch { setError('Failed to duplicate') }
  }

  const selectedEnv = environments.find(e => e.id === selectedId) ?? null

  // ── Resizable split ──────────────────────────────────────────
  const [listHeight, setListHeight] = useState(180) // px
  const dragStartY   = useRef<number>(0)
  const dragStartH   = useRef<number>(0)
  const isDragging   = useRef(false)

  const onDragStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    isDragging.current  = true
    dragStartY.current  = e.clientY
    dragStartH.current  = listHeight

    const onMove = (ev: MouseEvent) => {
      if (!isDragging.current) return
      const delta = ev.clientY - dragStartY.current
      setListHeight(Math.max(80, Math.min(420, dragStartH.current + delta)))
    }
    const onUp = () => {
      isDragging.current = false
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [listHeight])

  return (
    <div className={styles.envPanel}>

      {/* ── List section ── */}
      <div className={styles.envListSection} style={{ height: listHeight }}>

        {/* List header */}
        <div className={styles.envListHeader}>
          {!creating && (
            <button className={styles.newEnvBtn} onClick={() => setCreating(true)}>
              <Plus size={12} /> New
            </button>
          )}
        </div>

        {/* Env rows */}
        <div className={styles.envList}>

          {environments.length === 0 && !creating && (
            <div className={styles.envEmpty}>
              <Layers size={20} className={styles.envEmptyIcon} />
              <p>No environments yet</p>
              <p className={styles.envEmptyHint}>Use <code>{'{{variable}}'}</code> in requests</p>
            </div>
          )}

          {environments.map(env => {
            const isActive   = activeEnvId === env.id
            const isSelected = selectedId === env.id
            return (
              <div
                key={env.id}
                className={`${styles.envRow} ${isSelected ? styles.envRowSelected : ''}`}
                onClick={() => setSelectedId(isSelected ? null : env.id)}
              >
                <div className={styles.envRowDot} style={{ background: env.color }} />
                <span className={styles.envRowName}>{env.name}</span>

                {isActive && (
                  <span className={styles.activeBadge} title="Active environment">
                    <Zap size={9} />
                  </span>
                )}

                <div className={styles.envRowActions} onClick={e => e.stopPropagation()}>
                  <button
                    className={`${styles.envAction} ${isActive ? styles.envActionActive : ''}`}
                    onClick={() => isActive ? setActiveEnv(null) : setActiveEnv(env.id)}
                    title={isActive ? 'Deactivate' : 'Set as active'}
                  >
                    {isActive ? <Check size={11} /> : <Zap size={11} />}
                  </button>
                  <button className={styles.envAction} onClick={() => dupEnv(env.id)} title="Duplicate">
                    <Copy size={11} />
                  </button>
                  <button className={`${styles.envAction} ${styles.envActionDelete}`} onClick={() => deleteEnv(env.id)} title="Delete">
                    <Trash2 size={11} />
                  </button>
                </div>
              </div>
            )
          })}

          {/* Inline create form */}
          {creating && (
            <div className={styles.createForm}>
              <div className={styles.createRow}>
                <div className={styles.createDotPreview} style={{ background: newColor }} />
                <input
                  className={styles.createInput}
                  value={newName}
                  onChange={e => setNewName(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter') createEnv(); if (e.key === 'Escape') { setCreating(false); setNewName('') } }}
                  placeholder="Environment name"
                  autoFocus
                />
              </div>
              <div className={styles.colorPicker}>
                {COLORS.map(c => (
                  <button
                    key={c}
                    className={`${styles.colorBtn} ${newColor === c ? styles.colorBtnActive : ''}`}
                    style={{ background: c }}
                    onClick={() => setNewColor(c)}
                  />
                ))}
              </div>
              <div className={styles.createActions}>
                <button className={styles.cancelBtn} onClick={() => { setCreating(false); setNewName('') }}>Cancel</button>
                <button className={styles.confirmBtn} onClick={createEnv} disabled={!newName.trim() || loading}>
                  Create
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* ── Drag handle ── */}
      {selectedEnv && (
        <div className={styles.dragHandle} onMouseDown={onDragStart}>
          <GripHorizontal size={14} className={styles.dragIcon} />
        </div>
      )}

      {/* ── Editor ── */}
      {selectedEnv && (
        <div className={styles.envEditorWrap}>
          <EnvEditor
            key={selectedEnv.id}
            env={selectedEnv}
            isActive={activeEnvId === selectedEnv.id}
            onUpdate={upsertEnvironment}
          />
        </div>
      )}

      {/* Error */}
      {error && (
        <div className={styles.envError}>
          <AlertCircle size={11} /> {error}
          <button onClick={() => setError(null)}>×</button>
        </div>
      )}
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// Variable editor — shows inside selected env
// ─────────────────────────────────────────────────────────────────────────────

function EnvEditor({ env, isActive, onUpdate }: {
  env: Environment
  isActive: boolean
  onUpdate: (env: Environment) => void
}) {
  const [rows, setRows]           = useState<EnvVar[]>(env.variables?.length ? [...env.variables] : [emptyVar(env.id)])
  const [name, setName]           = useState(env.name)
  const [color, setColor]         = useState(env.color)
  const [editingName, setEditingName] = useState(false)
  const [saving, setSaving]       = useState(false)
  const [saved, setSaved]         = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  async function save() {
    setSaving(true)
    try {
      const updated = await api.environments.update(env.id, {
        name, color, variables: rows.filter(r => r.key.trim()),
      })
      onUpdate(updated)
      setSaved(true)
      if (timer.current) clearTimeout(timer.current)
      timer.current = setTimeout(() => setSaved(false), 2000)
    } finally { setSaving(false) }
  }

  return (
    <div className={styles.envEditor}>

      {/* Editor header — name + color strip */}
      <div className={styles.editorHeader}>
        <div className={styles.editorHeaderLeft}>
          {/* Color dot — click cycles color */}
          <div
            className={styles.editorDot}
            style={{ background: color }}
            onClick={() => {
              const i = COLORS.indexOf(color)
              setColor(COLORS[(i + 1) % COLORS.length])
            }}
            title="Click to change color"
          />

          {editingName ? (
            <input
              className={styles.nameInput}
              value={name}
              onChange={e => setName(e.target.value)}
              onBlur={() => setEditingName(false)}
              onKeyDown={e => e.key === 'Enter' && setEditingName(false)}
              autoFocus
            />
          ) : (
            <span className={styles.editorName} onClick={() => setEditingName(true)}>
              {name}
              <Edit2 size={10} className={styles.editHint} />
            </span>
          )}
        </div>

        {isActive && <span className={styles.activeTag}><Zap size={9} /> Active</span>}
      </div>

      {/* Variables */}
      <div className={styles.varList}>
        <div className={styles.varListHead}>
          <span>Key</span><span>Value</span><span />
        </div>
        {rows.map((row, i) => (
          <VarRow
            key={row.id || i}
            row={row}
            onChange={(f, v) => setRows(r => r.map((x, j) => j === i ? { ...x, [f]: v } : x))}
            onRemove={() => setRows(r => r.filter((_, j) => j !== i))}
          />
        ))}
        <button className={styles.addVarBtn} onClick={() => setRows(r => [...r, emptyVar(env.id)])}>
          <Plus size={11} /> Add variable
        </button>
      </div>

      <div className={styles.editorFooter}>
        <code className={styles.usageHint}>{'{{variable}}'}</code>
        <button className={styles.saveBtn} onClick={save} disabled={saving}>
          {saved ? <><Check size={11} /> Saved</> : saving ? 'Saving…' : 'Save'}
        </button>
      </div>
    </div>
  )
}

function VarRow({ row, onChange, onRemove }: {
  row: EnvVar
  onChange: (f: keyof EnvVar, v: string | boolean) => void
  onRemove: () => void
}) {
  const [show, setShow] = useState(!row.secret)
  return (
    <div className={styles.varRow}>
      <input
        className={styles.varKey}
        value={row.key}
        onChange={e => onChange('key', e.target.value)}
        placeholder="KEY"
        spellCheck={false}
      />
      <input
        className={styles.varVal}
        type={row.secret && !show ? 'password' : 'text'}
        value={row.value}
        onChange={e => onChange('value', e.target.value)}
        placeholder="value"
        spellCheck={false}
      />
      <div className={styles.varBtns}>
        <button
          className={`${styles.varBtn} ${row.secret ? styles.varBtnSecret : ''}`}
          onClick={() => { onChange('secret', !row.secret); setShow(!row.secret) }}
          title={row.secret ? 'Secret' : 'Visible'}
        >
          {row.secret ? <EyeOff size={10} /> : <Eye size={10} />}
        </button>
        <button className={`${styles.varBtn} ${styles.varBtnDel}`} onClick={onRemove}>
          <Trash2 size={10} />
        </button>
      </div>
    </div>
  )
}

function emptyVar(envId: string): EnvVar {
  return { id: crypto.randomUUID(), envId, key: '', value: '', secret: false }
}
