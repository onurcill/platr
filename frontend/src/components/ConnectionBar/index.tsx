import { useState, useEffect } from 'react'
import { PlugZap, Trash2, ChevronDown, Plus, Sun, Moon, Wifi, WifiOff, FileCode, Building2, X, Settings, Star, Zap } from 'lucide-react'
import { useConnectionStore, useThemeStore, useAuthStore } from '../../stores'
import { api } from '../../api/client'
import { ProtoUpload } from '../ProtoUpload'
import { WorkspacePanel } from '../WorkspacePanel'
import { BillingPanel } from '../BillingPanel'
import { useEnvironmentStore, useBillingStore } from '../../stores'
import { usePermission } from '../../hooks/usePermission'
import { K8sDiscovery } from '../K8sDiscovery'
import styles from './ConnectionBar.module.css'

export function ConnectionBar() {
  const { connections, activeConnectionId, addConnection, removeConnection, setActiveConnection } = useConnectionStore()
  const { theme, toggleTheme } = useThemeStore()

  const [address, setAddress] = useState('localhost:50051')
  const [name, setName] = useState('')
  const user = useAuthStore(s => s.user)
  const { can } = usePermission()
  const activeWorkspaceId = useAuthStore(s => s.activeWorkspaceId)
  const workspaces = useAuthStore(s => s.workspaces)
  const activeWs = workspaces.find(w => w.id === activeWorkspaceId) ?? workspaces[0] ?? null
  const [showWorkspace, setShowWorkspace] = useState(false)
  const [tls, setTls] = useState(false)
  const [insecure, setInsecure] = useState(true) // skip cert verification by default
  const [connecting, setConnecting] = useState(false)
  const [showMeta, setShowMeta] = useState(false)
  const [metaRows, setMetaRows] = useState<{key:string,value:string}[]>([{key:'authorization',value:'Bearer '}])
  const [error, setError] = useState<string | null>(null)
  const [showDropdown, setShowDropdown] = useState(false)
  const [connSearch, setConnSearch] = useState('')
  const [favorites, setFavorites] = useState<Set<string>>(new Set())
  const [showProtoUpload, setShowProtoUpload] = useState(false)
  const [showK8s, setShowK8s] = useState(false)
  const [showBillingPanel, setShowBillingPanel] = useState(false)
  const { setEnvironments } = useEnvironmentStore()
  const { currentPlan, usage, invocationsPercent, canUseFeature } = useBillingStore()
  const canK8s = canUseFeature('k8sIntegration')

  // Load environments when workspace changes
  useEffect(() => {
    if (activeWorkspaceId) {
      api.environments.list(activeWorkspaceId)
        .then(data => setEnvironments(data))
        .catch(() => {})
    }
  }, [activeWorkspaceId])

  const activeConn = connections.find((c) => c.id === activeConnectionId)

  async function connect() {
    if (!address.trim()) return
    setConnecting(true)
    setError(null)
    try {
      const meta: Record<string,string> = {}
      metaRows.forEach(r => { if (r.key.trim()) meta[r.key.trim()] = r.value })
      const conn = await api.connections.create({
        address: address.trim(),
        name: name.trim() || address.trim(),
        tls,
        insecure,
        metadata: meta,
      })
      addConnection(conn)
      setActiveConnection(conn.id)
      setAddress('')
      setName('')
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Connection failed')
    } finally {
      setConnecting(false)
    }
  }

  async function disconnect(id: string) {
    await api.connections.delete(id)
    removeConnection(id)
    if (activeConnectionId === id) setActiveConnection(null)
  }

  return (
    <header className={styles.bar}>
      {/* Logo */}
      <div className={styles.logo}>
        <div className={styles.logoMark}>
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
            <path d="M10 2L17.32 6.5V15.5L10 20L2.68 15.5V6.5L10 2Z" fill="var(--accent)" opacity="0.15"/>
            <path d="M10 2L17.32 6.5V15.5L10 20L2.68 15.5V6.5L10 2Z" stroke="var(--accent)" strokeWidth="1.2" fill="none"/>
            <circle cx="10" cy="11" r="2.5" fill="var(--accent)"/>
            <line x1="10" y1="5" x2="10" y2="8.5" stroke="var(--accent)" strokeWidth="1.2" strokeLinecap="round"/>
          </svg>
        </div>
        <span className={styles.logoText}>Platr</span>
      </div>

      <div className={styles.sep} />

      {/* Connection input */}
      <div className={styles.inputGroup}>
        <div className={styles.tlsToggle} onClick={() => {
          if (!tls) { setTls(true); setInsecure(true) }
          else if (tls && insecure) { setTls(true); setInsecure(false) }
          else { setTls(false); setInsecure(true) }
        }} title={!tls ? 'PLAIN' : insecure ? 'TLS (skip verify)' : 'TLS (strict)'}>
          {!tls ? <span className={styles.tlsOff}>PLAIN</span>
            : insecure ? <span className={styles.tlsSkip}>TLS⚠</span>
            : <span className={styles.tlsOn}>TLS✓</span>}
        </div>
        <input
          className={styles.addressInput}
          value={address}
          onChange={(e) => setAddress(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && connect()}
          placeholder="host:port"
          spellCheck={false}
        />
        <input
          className={styles.nameInput}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="label"
          spellCheck={false}
        />
        <button
          className={`${styles.metaToggle} ${showMeta ? styles.metaToggleActive : ''}`}
          onClick={() => setShowMeta(v => !v)}
          title="Request metadata"
          type="button"
        >
          <Settings size={12} />
        </button>
        <button
          className={styles.connectBtn}
          onClick={connect}
          disabled={connecting || !address.trim()}
        >
          {connecting
            ? <span className={styles.spinner} />
            : <><PlugZap size={13} /> Connect</>
          }
        </button>
      </div>

      {/* Active connection switcher */}
      {connections.length > 0 && (
        <div className={styles.switcher} onClick={() => setShowDropdown(!showDropdown)}>
          <div className={styles.activeConn}>
            <span className={activeConn ? styles.dotGreen : styles.dotGray} />
            <span className={styles.activeLabel}>
              {activeConn ? activeConn.name : 'No connection'}
            </span>
            <ChevronDown size={11} className={showDropdown ? styles.chevronOpen : ''} />
          </div>
          {showDropdown && (
            <div className={styles.dropdown} onClick={e => e.stopPropagation()}>
              {connections.length > 3 && (
                <div className={styles.dropdownSearch}>
                  <input
                    className={styles.dropdownSearchInput}
                    placeholder="Search…"
                    value={connSearch}
                    onChange={e => setConnSearch(e.target.value)}
                    autoFocus
                  />
                </div>
              )}
              {[...connections.filter(c => favorites.has(c.id)), ...connections.filter(c => !favorites.has(c.id))]
                .filter(c => !connSearch || c.name.toLowerCase().includes(connSearch.toLowerCase()) || c.address.toLowerCase().includes(connSearch.toLowerCase()))
                .map((c) => (
                <div
                  key={c.id}
                  className={`${styles.dropdownItem} ${c.id === activeConnectionId ? styles.dropdownActive : ''}`}
                  onClick={() => { setActiveConnection(c.id); setShowDropdown(false); setConnSearch('') }}
                >
                  <div className={styles.dropdownItemInfo}>
                    {c.state === 'READY' || c.state === 'IDLE'
                      ? <Wifi size={12} className={styles.iconGreen} />
                      : <WifiOff size={12} className={styles.iconMuted} />
                    }
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className={styles.dropdownName}>{c.name}</div>
                      <div className={styles.dropdownAddr}>{c.address}</div>
                    </div>
                  </div>
                  <button className={`${styles.favBtn} ${favorites.has(c.id) ? styles.favBtnActive : ''}`}
                    onClick={(e) => { e.stopPropagation(); setFavorites(prev => { const n = new Set(prev); n.has(c.id) ? n.delete(c.id) : n.add(c.id); return n }) }}>
                    <Star size={11} />
                  </button>
                  <button className={styles.deleteBtn}
                    onClick={(e) => { e.stopPropagation(); disconnect(c.id) }}>
                    <Trash2 size={11} />
                  </button>
                </div>
              ))}
              <div className={styles.dropdownDivider} />
              <div className={styles.dropdownItem}
                onClick={() => { setShowDropdown(false); setActiveConnection(null) }}>
                <Plus size={12} /><span>New connection</span>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Meta panel */}
      {showMeta && (
        <div className={styles.metaPanel}>
          <span className={styles.metaPanelLabel}>Default Metadata</span>
          {metaRows.map((row, i) => (
            <div key={i} className={styles.metaRow}>
              <input className={styles.metaKeyInput} value={row.key} placeholder="key" spellCheck={false}
                onChange={e => setMetaRows(rows => rows.map((r,j) => j===i ? {...r, key: e.target.value} : r))} />
              <input className={styles.metaValInput} value={row.value} placeholder="value" spellCheck={false}
                onChange={e => setMetaRows(rows => rows.map((r,j) => j===i ? {...r, value: e.target.value} : r))} />
              <button className={styles.removeMetaBtn} onClick={() => setMetaRows(rows => rows.filter((_,j) => j!==i))}><X size={11}/></button>
            </div>
          ))}
          <button className={styles.addMetaBtn} onClick={() => setMetaRows(r => [...r, {key:'',value:''}])}><Plus size={11}/> Add header</button>
        </div>
      )}

      {error && <div className={styles.errorMsg}>{error}</div>}

      {/* Right group */}
      <div className={styles.rightGroup}>
        {/* Proto */}
        {can('proto:upload') && (
          <button className={styles.protoBtn} onClick={() => setShowProtoUpload(true)} title="Upload .proto">
            <FileCode size={13} /><span>Proto</span>
          </button>
        )}

        {/* K8s — Professional+ only */}
        <button
          className={`${styles.protoBtn} ${showK8s ? styles.protoBtnActive : ''} ${!canK8s ? styles.protoBtnLocked : ''}`}
          onClick={() => canK8s ? setShowK8s(s => !s) : setShowBillingPanel(true)}
          title={canK8s ? "Kubernetes port-forward" : "K8s integration requires Professional plan"}>
          <span style={{ fontSize: 14, lineHeight: 1 }}>⎈</span>
          <span>K8s{!canK8s && ' 🔒'}</span>
        </button>

        {/* Billing plan badge */}
        <button
          className={styles.billingBtn}
          onClick={() => setShowBillingPanel(true)}
          title={`${currentPlan?.displayName ?? 'Free'} plan`}
        >
          <Zap size={11} className={styles.billingIcon} />
          <span className={styles.billingLabel}>{currentPlan?.displayName ?? 'Free'}</span>
          {usage && usage.invocationsLimit !== -1 && invocationsPercent() > 75 && (
            <span className={styles.billingWarn}>!</span>
          )}
        </button>

        <div className={styles.sep} />

        {/* Workspace + user */}
        <button className={styles.workspaceBtn} onClick={() => setShowWorkspace(true)}>
          <Building2 size={13} />
          <span className={styles.wsName}>{activeWs?.name ?? 'Workspace'}</span>
        </button>

        <div className={styles.avatar}>{user?.name?.[0]?.toUpperCase() ?? '?'}</div>

        {/* Theme */}
        <button className={styles.themeBtn} onClick={toggleTheme} title="Toggle theme">
          {theme === 'dark' ? <Sun size={14} /> : <Moon size={14} />}
        </button>
      </div>

      {/* Modals */}
      {showProtoUpload && <ProtoUpload onClose={() => setShowProtoUpload(false)} />}
      {showWorkspace && <WorkspacePanel onClose={() => setShowWorkspace(false)} />}
      {showBillingPanel && <BillingPanel onClose={() => setShowBillingPanel(false)} />}

      {showK8s && (
        <div className={styles.k8sOverlay} onClick={() => setShowK8s(false)}>
          <div className={styles.k8sPanel} onClick={e => e.stopPropagation()}>
            <div className={styles.k8sPanelHeader}>
              <span className={styles.k8sPanelTitle}>⎈ Kubernetes</span>
              <button className={styles.k8sPanelClose} onClick={() => setShowK8s(false)}><X size={14} /></button>
            </div>
            <K8sDiscovery onConnected={() => setShowK8s(false)} />
          </div>
        </div>
      )}
    </header>
  )
}
