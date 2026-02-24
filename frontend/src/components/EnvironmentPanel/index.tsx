import { useEffect, useRef, useState } from 'react'
import {
  X, Plus, Trash2, Copy,
  Eye, EyeOff, Check, Layers, Edit2, AlertCircle
} from 'lucide-react'
import { useEnvironmentStore } from '../../stores'
import { useAuthStore } from '../../stores/authStore'
import { api } from '../../api/client'
import type { Environment, EnvVar } from '../../stores/environmentStore'
import styles from './EnvironmentPanel.module.css'

interface EnvironmentPanelProps {
  onClose: () => void
}

const COLORS = [
  '#818cf8', '#34d399', '#60a5fa', '#f472b6',
  '#fb923c', '#a78bfa', '#fbbf24', '#f87171',
]

export function EnvironmentPanel({ onClose }: EnvironmentPanelProps) {
  const { environments, activeEnvId, setEnvironments, setActiveEnv, upsertEnvironment, removeEnvironment } = useEnvironmentStore()
  const activeWorkspaceId = useAuthStore(s => s.activeWorkspaceId)
  const [selectedId, setSelectedId] = useState<string | null>(activeEnvId ?? environments[0]?.id ?? null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [newColor, setNewColor] = useState(COLORS[0])

  const selectedEnv = environments.find(e => e.id === selectedId) ?? null

  useEffect(() => { fetchEnvironments() }, [activeWorkspaceId])

  async function fetchEnvironments() {
    if (!activeWorkspaceId) return
    try {
      const data = await api.environments.list(activeWorkspaceId)
      setEnvironments(data)
      if (data.length > 0 && !selectedId) setSelectedId(data[0].id)
    } catch {
      setError('Failed to load environments')
    }
  }

  async function createEnvironment() {
    if (!newName.trim() || !activeWorkspaceId) return
    setLoading(true)
    try {
      const env = await api.environments.create(activeWorkspaceId, { name: newName.trim(), color: newColor })
      upsertEnvironment(env)
      setSelectedId(env.id)
      setCreating(false)
      setNewName('')
    } catch {
      setError('Failed to create environment')
    } finally {
      setLoading(false)
    }
  }

  async function deleteEnvironment(id: string) {
    try {
      await api.environments.delete(id)
      removeEnvironment(id)
      if (selectedId === id) setSelectedId(environments.find(e => e.id !== id)?.id ?? null)
    } catch {
      setError('Failed to delete environment')
    }
  }

  async function duplicateEnvironment(id: string) {
    try {
      const env = await api.environments.duplicate(id)
      upsertEnvironment(env)
      setSelectedId(env.id)
    } catch {
      setError('Failed to duplicate environment')
    }
  }

  return (
    <div className={styles.overlay} onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className={styles.modal}>

        {/* Header */}
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <Layers size={16} className={styles.headerIcon} />
            <span className={styles.headerTitle}>Environments</span>
            {activeWorkspaceId && (
              <span className={styles.headerWs}>{activeWorkspaceId.slice(0, 8)}</span>
            )}
          </div>
          <button className={styles.closeBtn} onClick={onClose}><X size={15} /></button>
        </div>

        <div className={styles.layout}>
          {/* Sidebar */}
          <div className={styles.sidebar}>
            <div className={styles.sidebarHeader}>
              <span className={styles.sidebarLabel}>ENVIRONMENTS</span>
              <button className={styles.addBtn} onClick={() => setCreating(true)} title="New environment">
                <Plus size={13} />
              </button>
            </div>

            <div
              className={`${styles.envItem} ${activeEnvId === null ? styles.envItemActive : ''}`}
              onClick={() => setActiveEnv(null)}
            >
              <div className={styles.envDot} style={{ background: 'var(--text-muted)' }} />
              <span className={styles.envName}>No environment</span>
              {activeEnvId === null && <Check size={11} className={styles.activeCheck} />}
            </div>

            {environments.map(env => (
              <div
                key={env.id}
                className={`${styles.envItem} ${selectedId === env.id ? styles.envItemSelected : ''}`}
                onClick={() => setSelectedId(env.id)}
              >
                <div className={styles.envDot} style={{ background: env.color }} />
                <span className={styles.envName}>{env.name}</span>
                <div className={styles.envActions}>
                  {activeEnvId === env.id && <Check size={11} className={styles.activeCheck} />}
                  <button
                    className={styles.iconBtn}
                    onClick={(e) => { e.stopPropagation(); duplicateEnvironment(env.id) }}
                    title="Duplicate"
                  >
                    <Copy size={11} />
                  </button>
                  <button
                    className={styles.iconBtn}
                    onClick={(e) => { e.stopPropagation(); deleteEnvironment(env.id) }}
                    title="Delete"
                  >
                    <Trash2 size={11} />
                  </button>
                </div>
              </div>
            ))}

            {creating && (
              <div className={styles.createForm}>
                <input
                  className={styles.createInput}
                  value={newName}
                  onChange={e => setNewName(e.target.value)}
                  onKeyDown={e => {
                    if (e.key === 'Enter') createEnvironment()
                    if (e.key === 'Escape') setCreating(false)
                  }}
                  placeholder="Environment name"
                  autoFocus
                />
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
                  <button className={styles.cancelBtn} onClick={() => setCreating(false)}>Cancel</button>
                  <button
                    className={styles.saveBtn}
                    onClick={createEnvironment}
                    disabled={!newName.trim() || loading || !activeWorkspaceId}
                  >
                    Create
                  </button>
                </div>
              </div>
            )}
          </div>

          {/* Editor */}
          <div className={styles.editor}>
            {!activeWorkspaceId ? (
              <div className={styles.emptyEditor}>
                <AlertCircle size={28} className={styles.emptyIcon} />
                <p>No workspace selected</p>
                <p className={styles.emptyHint}>Select a workspace to manage environments</p>
              </div>
            ) : selectedEnv ? (
              <EnvironmentEditor
                env={selectedEnv}
                isActive={activeEnvId === selectedEnv.id}
                onActivate={() => setActiveEnv(selectedEnv.id)}
                onDeactivate={() => setActiveEnv(null)}
                onUpdate={(updated) => upsertEnvironment(updated)}
              />
            ) : (
              <div className={styles.emptyEditor}>
                <Layers size={32} className={styles.emptyIcon} />
                <p>Select or create an environment</p>
                <p className={styles.emptyHint}>
                  Use <code>{'{{variable}}'}</code> in request body, metadata, and addresses
                </p>
              </div>
            )}
          </div>
        </div>

        {error && (
          <div className={styles.errorBar}>
            <AlertCircle size={13} />
            <span>{error}</span>
            <button onClick={() => setError(null)}><X size={11} /></button>
          </div>
        )}
      </div>
    </div>
  )
}

// ── Environment Editor ────────────────────────────────────────────────────────

interface EnvEditorProps {
  env: Environment
  isActive: boolean
  onActivate: () => void
  onDeactivate: () => void
  onUpdate: (env: Environment) => void
}

function EnvironmentEditor({ env, isActive, onActivate, onDeactivate, onUpdate }: EnvEditorProps) {
  const [rows, setRows] = useState<EnvVar[]>(env.variables?.length ? env.variables : [emptyVar(env.id)])
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [editingName, setEditingName] = useState(false)
  const [name, setName] = useState(env.name)
  const [color, setColor] = useState(env.color)
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    setRows(env.variables?.length ? [...env.variables] : [emptyVar(env.id)])
    setName(env.name)
    setColor(env.color)
  }, [env.id])

  async function save() {
    setSaving(true)
    try {
      const updated = await api.environments.update(env.id, {
        name,
        color,
        variables: rows.filter(r => r.key.trim() !== ''),
      })
      onUpdate(updated)
      setSaved(true)
      if (saveTimer.current) clearTimeout(saveTimer.current)
      saveTimer.current = setTimeout(() => setSaved(false), 2000)
    } catch {
      // show error inline
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className={styles.envEditor}>
      <div className={styles.envEditorHeader}>
        <div className={styles.envEditorName}>
          <div className={styles.envDotLg} style={{ background: color }} />
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
            <span className={styles.envTitle} onClick={() => setEditingName(true)}>
              {name} <Edit2 size={11} className={styles.editIcon} />
            </span>
          )}
        </div>
        <div className={styles.envEditorActions}>
          <div className={styles.colorPicker}>
            {COLORS.map(c => (
              <button
                key={c}
                className={`${styles.colorBtn} ${color === c ? styles.colorBtnActive : ''}`}
                style={{ background: c }}
                onClick={() => setColor(c)}
              />
            ))}
          </div>
          {isActive ? (
            <button className={styles.deactivateBtn} onClick={onDeactivate}>Deactivate</button>
          ) : (
            <button className={styles.activateBtn} onClick={onActivate}>
              <Check size={12} /> Set Active
            </button>
          )}
        </div>
      </div>

      <div className={styles.varTable}>
        <div className={styles.varTableHeader}>
          <span className={styles.varCol}>VARIABLE</span>
          <span className={styles.varCol}>VALUE</span>
          <span className={styles.varColSm}>SECRET</span>
          <span className={styles.varColSm} />
        </div>

        {rows.map((row, idx) => (
          <VarRow
            key={row.id || idx}
            row={row}
            onChange={(f, v) => setRows(r => r.map((x, i) => i === idx ? { ...x, [f]: v } : x))}
            onRemove={() => setRows(r => r.filter((_, i) => i !== idx))}
          />
        ))}

        <button className={styles.addVarBtn} onClick={() => setRows(r => [...r, emptyVar(env.id)])}>
          <Plus size={12} /> Add variable
        </button>
      </div>

      <div className={styles.saveRow}>
        <span className={styles.saveHint}>
          Use <code>{'{{variable_name}}'}</code> in request body and metadata
        </span>
        <button className={styles.saveBtn} onClick={save} disabled={saving}>
          {saved ? <><Check size={12} /> Saved</> : saving ? 'Saving…' : 'Save'}
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
  const [showValue, setShowValue] = useState(!row.secret)

  return (
    <div className={styles.varRow}>
      <input
        className={styles.varInput}
        value={row.key}
        onChange={e => onChange('key', e.target.value)}
        placeholder="VARIABLE_NAME"
        spellCheck={false}
      />
      <input
        className={styles.varInput}
        type={row.secret && !showValue ? 'password' : 'text'}
        value={row.value}
        onChange={e => onChange('value', e.target.value)}
        placeholder="value"
        spellCheck={false}
      />
      <button
        className={`${styles.secretBtn} ${row.secret ? styles.secretBtnOn : ''}`}
        onClick={() => { onChange('secret', !row.secret); setShowValue(!row.secret) }}
        title={row.secret ? 'Secret (hidden)' : 'Not secret'}
      >
        {row.secret ? <EyeOff size={11} /> : <Eye size={11} />}
      </button>
      <button className={styles.removeBtn} onClick={onRemove}>
        <Trash2 size={11} />
      </button>
    </div>
  )
}

function emptyVar(envId: string): EnvVar {
  return { id: crypto.randomUUID(), envId, key: '', value: '', secret: false }
}
