import { useRef, useState } from 'react'
import { Upload, X, FileCode, CheckCircle, AlertCircle, ChevronRight, Zap, Radio, ArrowRightLeft, Plug, PlugZap } from 'lucide-react'
import { apiFetch, useAuthStore, useConnectionStore, useServiceStore, useTabStore } from '../../stores'
import { api } from '../../api/client'
import type { ServiceDescriptor, MethodDescriptor } from '../../types'
import styles from './ProtoUpload.module.css'

interface ProtoParseResult {
  files: string[]
  services: ServiceDescriptor[]
}

interface ProtoUploadProps {
  onClose: () => void
}

export function ProtoUpload({ onClose }: ProtoUploadProps) {
  const { connections, activeConnectionId, addConnection, setActiveConnection } = useConnectionStore()
  const { setService, setServiceNames } = useServiceStore()

  const [step, setStep] = useState<'upload' | 'result'>('upload')
  const [dragging, setDragging] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [connecting, setConnecting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<ProtoParseResult | null>(null)
  const [selectedFiles, setSelectedFiles] = useState<File[]>([])

  // Connection fields
  const [address, setAddress] = useState('localhost:50051')
  const [connName, setConnName] = useState('')
  const [tls, setTls] = useState(false)
  // Or pick existing
  const [targetConnId, setTargetConnId] = useState<string>(activeConnectionId || '')
  const [mode, setMode] = useState<'new' | 'existing'>(connections.length > 0 ? 'existing' : 'new')

  const fileRef = useRef<HTMLInputElement>(null)

  async function upload(files: File[]) {
    if (files.length === 0) return
    setUploading(true)
    setError(null)
    setSelectedFiles(files)

    try {
      // Read token directly - bypass any store timing issues
      const token = useAuthStore.getState().token ||
        (() => { try { return JSON.parse(localStorage.getItem('grpc-inspector-auth') || '{}')?.state?.token } catch { return null } })()

      let data: ProtoParseResult
      if (files.length === 1) {
        const form = new FormData()
        form.append('file', files[0])
        if (activeConnectionId) form.append('connId', activeConnectionId)
        const res = await fetch('/api/proto/upload', {
          method: 'POST',
          headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
          body: form,
        })
        const json = await res.json()
        if (!res.ok) throw new Error(json.error || 'Upload failed')
        data = json
      } else {
        const form = new FormData()
        files.forEach(f => form.append('files[]', f))
        if (activeConnectionId) form.append('connId', activeConnectionId)
        const res = await fetch('/api/proto/upload-multi', {
          method: 'POST',
          headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}) },
          body: form,
        })
        const json = await res.json()
        if (!res.ok) throw new Error(json.error || 'Upload failed')
        data = json
      }
      setResult(data)
      setStep('result')
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Upload failed')
    } finally {
      setUploading(false)
    }
  }

  function handleFiles(files: FileList | File[]) {
    const arr = Array.from(files).filter(f => /\.(proto|bin|pb|desc)$/i.test(f.name))
    if (arr.length === 0) { setError('Please select .proto files'); return }
    upload(arr)
  }

  function applyServices(connId: string) {
    if (!result) return
    const names = result.services.map(s => s.name)
    setServiceNames(connId, names)
    result.services.forEach(svc => setService(connId, svc.name, svc))
    setActiveConnection(connId)
  }

  async function connectAndApply() {
    if (!result) return
    setConnecting(true)
    setError(null)
    try {
      const conn = await api.connections.create({
        address: address.trim(),
        name: connName.trim() || address.trim(),
        tls,
      })
      addConnection(conn)
      // Re-register proto descriptors with the new connection ID
      if (selectedFiles.length > 0) {
        const form = new FormData()
        form.append('connId', conn.id)
        if (selectedFiles.length === 1) {
          form.append('file', selectedFiles[0])
          await apiFetch('/api/proto/upload', { method: 'POST', body: form })
        } else {
          selectedFiles.forEach(f => form.append('files[]', f))
          await apiFetch('/api/proto/upload-multi', { method: 'POST', body: form })
        }
      }
      applyServices(conn.id)
      onClose()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Connection failed')
    } finally {
      setConnecting(false)
    }
  }

  function applyToExisting() {
    if (!result || !targetConnId) return
    applyServices(targetConnId)
    onClose()
  }

  function methodIcon(m: MethodDescriptor) {
    if (m.clientStreaming && m.serverStreaming) return <ArrowRightLeft size={11} className={styles.iconBidi} />
    if (m.serverStreaming) return <Radio size={11} className={styles.iconStream} />
    if (m.clientStreaming) return <Radio size={11} className={styles.iconClient} />
    return <Zap size={11} className={styles.iconUnary} />
  }

  return (
    <div className={styles.overlay} onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className={styles.modal}>

        {/* Header */}
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <FileCode size={16} className={styles.headerIcon} />
            <span className={styles.headerTitle}>Upload Proto File</span>
          </div>
          <button className={styles.closeBtn} onClick={onClose}><X size={15} /></button>
        </div>

        <div className={styles.body}>
          {/* ── Step 1: Upload ── */}
          {step === 'upload' && (
            <>
              <div className={styles.infoBox}>
                <p>
                  Servisin <code>.proto</code> dosyasını direkt yükle — <code>protoc</code> gerekmez.
                  Birden fazla dosya seçebilirsin (import'lar dahil).
                </p>
              </div>

              <div
                className={`${styles.dropzone} ${dragging ? styles.dropzoneDragging : ''} ${uploading ? styles.dropzoneLoading : ''}`}
                onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
                onDragLeave={() => setDragging(false)}
                onDrop={(e) => { e.preventDefault(); setDragging(false); if (e.dataTransfer.files.length) handleFiles(e.dataTransfer.files) }}
                onClick={() => !uploading && fileRef.current?.click()}
              >
                <input ref={fileRef} type="file" multiple accept=".proto,.bin,.pb,.desc"
                  style={{ display: 'none' }}
                  onChange={(e) => { if (e.target.files?.length) handleFiles(e.target.files); e.target.value = '' }}
                />
                {uploading ? (
                  <><span className={styles.spinner} /><span className={styles.dropzoneText}>Parsing…</span></>
                ) : (
                  <>
                    <Upload size={24} className={styles.dropzoneIcon} />
                    <span className={styles.dropzoneText}>Drop <code>.proto</code> file(s) or click to browse</span>
                    <span className={styles.dropzoneHint}>Birden fazla dosya seçilebilir (import'lar dahil)</span>
                  </>
                )}
              </div>

              {error && <div className={styles.errorBox}><AlertCircle size={13} /><span>{error}</span></div>}
            </>
          )}

          {/* ── Step 2: Result + Connect ── */}
          {step === 'result' && result && (
            <>
              <div className={styles.successBox}>
                <CheckCircle size={13} className={styles.successIcon} />
                <span>
                  <b>{selectedFiles.map(f => f.name).join(', ')}</b>
                  {' — '}{result.services.length} service,{' '}
                  {result.services.reduce((a, s) => a + s.methods.length, 0)} method
                </span>
              </div>

              <div className={styles.serviceTree}>
                {result.services.map(svc => (
                  <ServiceRow key={svc.name} svc={svc} methodIcon={methodIcon} onMethodClick={(s, m) => {
                    const template = m.inputSchema ? schemaToTemplate(m.inputSchema) : {}
                    const body = Object.keys(template).length > 0 ? JSON.stringify(template, null, 2) : '{}'
                    useTabStore.getState().openFromMethod(s, m.name, body, activeConnectionId)
                    onClose()
                  }} />
                ))}
              </div>

              {/* Mode tabs */}
              <div className={styles.modeTabs}>
                <button
                  className={`${styles.modeTab} ${mode === 'new' ? styles.modeTabActive : ''}`}
                  onClick={() => setMode('new')}
                >
                  <PlugZap size={12} /> New connection
                </button>
                {connections.length > 0 && (
                  <button
                    className={`${styles.modeTab} ${mode === 'existing' ? styles.modeTabActive : ''}`}
                    onClick={() => setMode('existing')}
                  >
                    <Plug size={12} /> Use existing
                  </button>
                )}
              </div>

              {/* New connection form */}
              {mode === 'new' && (
                <div className={styles.connForm}>
                  <div className={styles.connRow}>
                    <button
                      className={`${styles.tlsBadge} ${tls ? styles.tlsOn : styles.tlsOff}`}
                      onClick={() => setTls(!tls)}
                    >
                      {tls ? 'TLS' : 'PLAIN'}
                    </button>
                    <input
                      className={styles.connInput}
                      value={address}
                      onChange={e => setAddress(e.target.value)}
                      onKeyDown={e => e.key === 'Enter' && connectAndApply()}
                      placeholder="host:port"
                      spellCheck={false}
                    />
                    <input
                      className={styles.nameInput}
                      value={connName}
                      onChange={e => setConnName(e.target.value)}
                      placeholder="label (optional)"
                      spellCheck={false}
                    />
                  </div>
                  {error && <div className={styles.errorBox}><AlertCircle size={13} /><span>{error}</span></div>}
                </div>
              )}

              {/* Existing connection picker */}
              {mode === 'existing' && (
                <div className={styles.connForm}>
                  <select
                    className={styles.connSelect}
                    value={targetConnId}
                    onChange={e => setTargetConnId(e.target.value)}
                  >
                    <option value="">— select connection —</option>
                    {connections.map(c => (
                      <option key={c.id} value={c.id}>{c.name} ({c.address})</option>
                    ))}
                  </select>
                </div>
              )}
            </>
          )}
        </div>

        {/* Footer */}
        <div className={styles.footer}>
          {step === 'result' && (
            <button className={styles.backBtn}
              onClick={() => { setStep('upload'); setResult(null); setError(null) }}>
              ← Upload another
            </button>
          )}
          <div className={styles.footerRight}>
            <button className={styles.cancelBtn} onClick={onClose}>Cancel</button>
            {step === 'result' && mode === 'new' && (
              <button
                className={styles.applyBtn}
                onClick={connectAndApply}
                disabled={connecting || !address.trim()}
              >
                {connecting
                  ? <><span className={styles.spinnerSm} /> Connecting…</>
                  : <><PlugZap size={13} /> Connect & Apply</>
                }
              </button>
            )}
            {step === 'result' && mode === 'existing' && (
              <button
                className={styles.applyBtn}
                onClick={applyToExisting}
                disabled={!targetConnId}
              >
                <Plug size={13} /> Apply to Explorer →
              </button>
            )}
          </div>
        </div>

      </div>
    </div>
  )
}


// Build JSON template from proto schema
function schemaToTemplate(schema: any, depth = 0): any {
  if (!schema || depth > 4) return {}
  const out: Record<string, any> = {}
  for (const [key, field] of Object.entries<any>(schema.fields || {})) {
    const type: string = field.type || ''
    if (field.repeated) out[key] = []
    else if (type === 'bool') out[key] = false
    else if (type === 'string') out[key] = ''
    else if (['int32','int64','uint32','uint64','float','double'].includes(type)) out[key] = 0
    else if (type === 'bytes') out[key] = ''
    else if (type.startsWith('enum:')) out[key] = 0
    else if (type.startsWith('message:') && field.nested) out[key] = schemaToTemplate(field.nested, depth + 1)
    else out[key] = null
  }
  return out
}

function ServiceRow({ svc, methodIcon, onMethodClick }: {
  svc: ServiceDescriptor
  methodIcon: (m: MethodDescriptor) => React.ReactNode
  onMethodClick: (svc: string, m: MethodDescriptor) => void
}) {
  const [open, setOpen] = useState(true)
  const shortName = svc.name.split('.').pop() || svc.name
  const pkg = svc.name.includes('.') ? svc.name.split('.').slice(0, -1).join('.') : ''

  return (
    <div className={styles.svcGroup}>
      <div className={styles.svcRow} onClick={() => setOpen(o => !o)}>
        <ChevronRight size={12} className={`${styles.chevron} ${open ? styles.chevronOpen : ''}`} />
        <span className={styles.svcName}>{shortName}</span>
        {pkg && <span className={styles.svcPkg}>{pkg}</span>}
        <span className={styles.svcCount}>{svc.methods.length}</span>
      </div>
      {open && (
        <div className={styles.methods}>
          {svc.methods.map(m => (
            <div key={m.name} className={styles.methodRow} onClick={() => onMethodClick(svc.name, m)} style={{ cursor: 'pointer' }}>
              {methodIcon(m)}
              <span className={styles.methodName}>{m.name}</span>
              <span className={styles.methodTypes}>
                <span className={styles.inputType}>{m.inputType.split('.').pop()}</span>
                <span className={styles.arrow}>→</span>
                <span className={styles.outputType}>{m.outputType.split('.').pop()}</span>
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
