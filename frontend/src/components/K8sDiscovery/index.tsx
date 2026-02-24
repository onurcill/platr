import { useEffect, useRef, useState } from 'react'
import {
  Plug, Loader, X, Upload, Trash2, RefreshCw,
  Wifi, Plus, ChevronRight, ChevronDown
} from 'lucide-react'
import { apiFetch, useConnectionStore, useEnvironmentStore } from '../../stores'
import type { Connection } from '../../types'
import styles from './K8sDiscovery.module.css'

interface KubeconfigEntry {
  id: string; name: string; contexts: string[]; currentContext: string
}
interface K8sSvc {
  name: string; namespace: string; ports: { name: string; port: number }[]
}
interface ActiveFwd {
  id: string; namespace: string; service: string
  localPort: number; remotePort: number; connectionId: string
}

export function K8sDiscovery({ onConnected }: { onConnected?: () => void } = {}) {
  const { addConnection, setActiveConnection } = useConnectionStore()
  const { environments, activeEnvId } = useEnvironmentStore()

  const [configs, setConfigs]           = useState<KubeconfigEntry[]>([])
  const [activeConfigId, setActiveConfigId] = useState('')
  const [activeContext, setActiveContext]   = useState('')
  const [namespaces, setNamespaces]         = useState<string[]>([])
  const [activeNs, setActiveNs]             = useState('default')
  const [services, setServices]             = useState<K8sSvc[]>([])
  const [forwards, setForwards]             = useState<ActiveFwd[]>([])
  const [expandedSvc, setExpandedSvc]       = useState<string | null>(null)

  const [loading, setLoading]     = useState(false)
  const [uploading, setUploading] = useState(false)
  const [connecting, setConnecting] = useState<string | null>(null)
  const [error, setError]         = useState<string | null>(null)
  const [uploadName, setUploadName] = useState('')

  const [svcTls, setSvcTls]         = useState<Record<string, boolean>>({})
  const [svcInsecure, setSvcInsecure] = useState<Record<string, boolean>>({})
  const [svcMeta, setSvcMeta]       = useState<Record<string, { key: string; value: string }[]>>({})
  const [svcPort, setSvcPort]       = useState<Record<string, number>>({})

  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => { loadConfigs() }, [])
  useEffect(() => { if (activeConfigId) loadNamespaces() }, [activeConfigId, activeContext])
  useEffect(() => { if (activeConfigId && activeNs) loadServices() }, [activeConfigId, activeContext, activeNs])

  async function loadConfigs() {
    try {
      const r = await apiFetch('/api/k8s/kubeconfigs')
      const d = await r.json()
      const list: KubeconfigEntry[] = d.configs || []
      setConfigs(list)
      if (list.length > 0) {
        setActiveConfigId(list[0].id)
        setActiveContext(list[0].currentContext || list[0].contexts?.[0] || '')
      }
    } catch {}
  }

  async function loadNamespaces() {
    setError(null)
    try {
      const params = new URLSearchParams({ configId: activeConfigId })
      if (activeContext) params.set('context', activeContext)
      const r = await apiFetch(`/api/k8s/namespaces?${params}`)
      const d = await r.json()
      if (!r.ok) throw new Error(d.error || 'Failed to list namespaces')
      const ns: string[] = d.namespaces || []
      setNamespaces(ns)
      if (ns.length > 0 && !ns.includes(activeNs)) setActiveNs(ns[0])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to list namespaces')
    }
  }

  async function loadServices() {
    setLoading(true); setError(null)
    try {
      const [sr, fr, cr] = await Promise.all([
        apiFetch(`/api/k8s/services?${new URLSearchParams({ configId: activeConfigId, ...(activeContext ? { context: activeContext } : {}), namespace: activeNs })}`).then(r => r.json()),
        apiFetch('/api/k8s/forwards').then(r => r.json()),
        apiFetch('/api/connections').then(r => r.json()),
      ])
      setServices(sr.services || [])

      const fwds: ActiveFwd[] = fr.forwards || []
      setForwards(fwds)

      // Restore any forward-backed connections that aren't already in the store.
      // This handles the case where the K8s modal is re-opened after a forward
      // was created — the connection exists on the backend but may have been
      // cleared from the in-memory store (e.g. after a page refresh).
      const knownIds = new Set((cr as Connection[]).map((c: Connection) => c.id));
      (cr as Connection[]).forEach((c: Connection) => addConnection(c))

      fwds.forEach(f => {
        if (f.connectionId && !knownIds.has(f.connectionId)) {
          // Fallback: synthesise a minimal connection entry so it appears in the bar.
          // The next reflect/invoke call will use the real localPort address.
          addConnection({
            id:        f.connectionId,
            name:      `${f.service}:${f.remotePort}`,
            address:   `localhost:${f.localPort}`,
            tls:       false,
            state:     'READY',
            metadata:  {},
            createdAt: new Date().toISOString(),
          })
        }
      })
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed')
    } finally { setLoading(false) }
  }

  async function uploadConfig(file: File) {
    setUploading(true); setError(null)
    const form = new FormData()
    form.append('file', file)
    form.append('name', uploadName || file.name.replace(/\.[^.]+$/, ''))
    try {
      const r = await apiFetch('/api/k8s/kubeconfigs', { method: 'POST', body: form })
      const d = await r.json()
      if (!r.ok) throw new Error(d.error)
      setConfigs(p => [...p, d])
      setActiveConfigId(d.id)
      setActiveContext(d.currentContext || d.contexts?.[0] || '')
      setUploadName('')
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Upload failed')
    } finally { setUploading(false) }
  }

  async function deleteConfig(id: string) {
    await apiFetch(`/api/k8s/kubeconfigs/${id}`, { method: 'DELETE' })
    const rem = configs.filter(c => c.id !== id)
    setConfigs(rem)
    if (activeConfigId === id) { setActiveConfigId(rem[0]?.id || ''); setActiveContext(rem[0]?.currentContext || rem[0]?.contexts?.[0] || '') }
  }

  async function stopForward(id: string) {
    await apiFetch(`/api/k8s/forwards/${id}`, { method: 'DELETE' })
    setForwards(p => p.filter(f => f.id !== id))
  }

  function getMeta(key: string) { return svcMeta[key] || [{ key: 'authorization', value: 'Bearer ' }] }
  function getTls(key: string)  { return svcTls[key] ?? false }
  function getIns(key: string)  { return svcInsecure[key] ?? true }
  function cycleTls(key: string) {
    const t = getTls(key), i = getIns(key)
    if (!t)      { setSvcTls(p => ({...p,[key]:true}));  setSvcInsecure(p => ({...p,[key]:true})) }
    else if (i)  { setSvcInsecure(p => ({...p,[key]:false})) }
    else         { setSvcTls(p => ({...p,[key]:false})); setSvcInsecure(p => ({...p,[key]:true})) }
  }
  function tlsLabel(key: string) { return !getTls(key) ? 'PLAIN' : getIns(key) ? 'TLS⚠' : 'TLS✓' }
  function tlsClass(key: string) { return !getTls(key) ? styles.tlsOff : getIns(key) ? styles.tlsSkip : styles.tlsOn }

  async function connect(svc: K8sSvc, port: number) {
    const key = `${svc.namespace}/${svc.name}`
    setConnecting(key); setError(null)
    try {
      const meta: Record<string, string> = {}
      environments.find(e => e.id === activeEnvId)?.variables?.forEach((v: any) => {
        if (v.key?.trim()) meta[v.key.trim()] = v.value ?? ''
      })
      getMeta(key).forEach(r => { if (r.key.trim()) meta[r.key.trim()] = r.value })

      const r = await apiFetch('/api/k8s/connect', {
        method: 'POST',
        body: JSON.stringify({
          configId: activeConfigId, context: activeContext,
          namespace: svc.namespace, service: svc.name, port,
          tls: getTls(key), insecure: getIns(key), metadata: meta,
          name: `${svc.name}:${port}`,
        }),
      })
      const d = await r.json()
      if (!r.ok) throw new Error(d.error)
      addConnection(d.connection)
      setActiveConnection(d.connection.id)
      setForwards(p => [...p.filter(f => f.id !== d.forward?.id), {
        id: d.forward?.id, namespace: svc.namespace, service: svc.name,
        localPort: d.forward?.localPort, remotePort: port, connectionId: d.connection.id,
      }])
      setExpandedSvc(null)
      onConnected?.()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Connect failed')
    } finally { setConnecting(null) }
  }

  const activeConfig = configs.find(c => c.id === activeConfigId)
  const connectedKeys = new Set(forwards.map(f => `${f.namespace}/${f.service ?? ''}`))

  return (
    <div className={styles.panel}>
      {error && (
        <div className={styles.error}>
          <span>{error}</span>
          <button onClick={() => setError(null)}><X size={10} /></button>
        </div>
      )}

      {/* Upload row */}
      <div className={styles.section}>
        <div className={styles.uploadRow}>
          <input className={styles.uploadName} placeholder="Config label (optional)"
            value={uploadName} onChange={e => setUploadName(e.target.value)} />
          <input ref={fileRef} type="file" style={{ display: 'none' }}
            onChange={e => { const f = e.target.files?.[0]; if (f) uploadConfig(f); e.target.value = '' }} />
          <button className={styles.uploadBtn} onClick={() => fileRef.current?.click()} disabled={uploading}>
            {uploading ? <Loader size={11} className={styles.spin} /> : <Upload size={11} />}
            {uploading ? 'Parsing…' : 'Upload kubeconfig'}
          </button>
        </div>

        {/* Config list */}
        {configs.map(c => (
          <div key={c.id} className={`${styles.configItem} ${c.id === activeConfigId ? styles.configActive : ''}`}
            onClick={() => { setActiveConfigId(c.id); setActiveContext(c.currentContext || c.contexts?.[0] || '') }}>
            <span className={styles.configName}>{c.name}</span>
            <span className={styles.configCtx}>{c.currentContext}</span>
            <button className={styles.configDel} onClick={e => { e.stopPropagation(); deleteConfig(c.id) }}>
              <Trash2 size={10} />
            </button>
          </div>
        ))}
      </div>

      {/* Context + namespace selectors */}
      {activeConfig && (
        <div className={styles.section}>
          <div className={styles.selectors}>
            {activeConfig.contexts.length > 1 && (
              <select className={styles.select} value={activeContext} onChange={e => setActiveContext(e.target.value)}>
                {activeConfig.contexts.map(ctx => <option key={ctx} value={ctx}>{ctx}</option>)}
              </select>
            )}
            {namespaces.length > 0 && (
              <select className={styles.select} value={activeNs} onChange={e => setActiveNs(e.target.value)}>
                {namespaces.map(ns => <option key={ns} value={ns}>{ns}</option>)}
              </select>
            )}
            <button className={styles.refreshBtn} onClick={loadServices} title="Refresh">
              <RefreshCw size={11} className={loading ? styles.spin : ''} />
            </button>
          </div>
        </div>
      )}

      {/* Active forwards */}
      {forwards.length > 0 && (
        <div className={styles.section}>
          <div className={styles.sectionLabel}>Active</div>
          {forwards.map(f => (
            <div key={f.id} className={styles.fwdRow}
              onClick={() => { if (f.connectionId) setActiveConnection(f.connectionId) }}
              title="Click to activate this connection">
              <Wifi size={10} className={styles.fwdIcon} />
              <span className={styles.fwdName}>{f.service}</span>
              <span className={styles.fwdPort}>:{f.localPort}→{f.remotePort}</span>
              <button className={styles.fwdStop} onClick={e => { e.stopPropagation(); stopForward(f.id) }}><X size={10} /></button>
            </div>
          ))}
        </div>
      )}

      {/* Service list */}
      <div className={styles.serviceList}>
        {!activeConfigId ? (
          <div className={styles.empty}>Upload a kubeconfig to get started</div>
        ) : loading && services.length === 0 ? (
          <div className={styles.loadingRow}><Loader size={12} className={styles.spin} /> Loading services…</div>
        ) : services.length === 0 && !loading ? (
          <div className={styles.empty}>No services in {activeNs}</div>
        ) : services.map(svc => {
          const key = `${svc.namespace}/${svc.name}`
          const open = expandedSvc === key
          const isConn = connectedKeys.has(key)
          const isConn2 = connecting === key
          const grpcPort = svc.ports?.find(p => p.name?.toLowerCase().includes('grpc') || p.port === 50051 || p.port === 9090)
          const defaultPort = grpcPort?.port ?? svc.ports?.[0]?.port ?? 50051
          const metaRows = getMeta(key)
          const selPort = svcPort[key] ?? defaultPort
          const setSelPort = (p: number) => setSvcPort(prev => ({...prev, [key]: p}))

          return (
            <div key={key} className={styles.svcGroup}>
              <div className={`${styles.svcRow} ${isConn ? styles.svcConnected : ''}`}
                onClick={() => setExpandedSvc(open ? null : key)}>
                {open ? <ChevronDown size={10} className={styles.chevron} /> : <ChevronRight size={10} className={styles.chevron} />}
                <span className={styles.svcName}>{svc.name}</span>
                {isConn && <Wifi size={9} className={styles.connectedDot} />}
                {isConn2 && <Loader size={9} className={styles.spin} />}
              </div>

              {open && (
                <div className={styles.svcPanel}>
                  <div className={styles.portRow}>
                    <select className={styles.portSelect} value={selPort}
                      onChange={e => setSelPort(Number(e.target.value))}>
                      {(svc.ports || []).map(p => (
                        <option key={p.port} value={p.port}>{p.name ? `${p.name}:${p.port}` : p.port}</option>
                      ))}
                    </select>
                    <button className={`${styles.tlsBadge} ${tlsClass(key)}`} onClick={() => cycleTls(key)}>
                      {tlsLabel(key)}
                    </button>
                  </div>

                  <div className={styles.metaLabel}>Metadata</div>
                  {metaRows.map((row, i) => (
                    <div key={i} className={styles.metaRow}>
                      <input className={styles.metaInput} value={row.key} placeholder="key" spellCheck={false}
                        onChange={e => setSvcMeta(p => ({...p,[key]:metaRows.map((r,j)=>j===i?{...r,key:e.target.value}:r)}))} />
                      <input className={styles.metaInput} value={row.value} placeholder="value" spellCheck={false}
                        onChange={e => setSvcMeta(p => ({...p,[key]:metaRows.map((r,j)=>j===i?{...r,value:e.target.value}:r)}))} />
                      <button className={styles.metaRemove}
                        onClick={() => setSvcMeta(p => ({...p,[key]:metaRows.filter((_,j)=>j!==i)}))}>
                        <X size={9} />
                      </button>
                    </div>
                  ))}
                  <button className={styles.metaAdd}
                    onClick={() => setSvcMeta(p => ({...p,[key]:[...metaRows,{key:'',value:''}]}))}>
                    <Plus size={9} /> Add header
                  </button>

                  <button className={styles.connectBtn} onClick={() => connect(svc, selPort)} disabled={isConn2}>
                    {isConn2
                      ? <><Loader size={10} className={styles.spin} />Forwarding…</>
                      : <><Plug size={10} />Port-forward & Connect</>
                    }
                  </button>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
