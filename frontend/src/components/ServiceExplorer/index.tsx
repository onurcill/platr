import { useEffect, useState } from 'react'
import { ChevronRight, Zap, Radio, ArrowRightLeft, RefreshCw, Loader } from 'lucide-react'
import { useConnectionStore, useServiceStore, useTabStore } from '../../stores'
import { api } from '../../api/client'
import type { MethodDescriptor } from '../../types'
import styles from './ServiceExplorer.module.css'

function methodIcon(m: MethodDescriptor) {
  if (m.clientStreaming && m.serverStreaming) return <ArrowRightLeft size={11} className={styles.iconBidi} />
  if (m.serverStreaming) return <Radio size={11} className={styles.iconStream} />
  if (m.clientStreaming) return <Radio size={11} className={styles.iconClientStream} />
  return <Zap size={11} className={styles.iconUnary} />
}

function methodType(m: MethodDescriptor) {
  if (m.clientStreaming && m.serverStreaming) return 'BIDI'
  if (m.serverStreaming) return 'S-STREAM'
  if (m.clientStreaming) return 'C-STREAM'
  return 'UNARY'
}

interface Props { onServicesLoaded?: (count: number) => void }


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

export function ServiceExplorer({ onServicesLoaded }: Props = {}) {
  const { activeConnectionId } = useConnectionStore()
  const { services, serviceNames, selectedMethod, expandedServices, setServiceNames, setService, selectMethod, toggleService } = useServiceStore()
  const openFromMethod = useTabStore(s => s.openFromMethod)

  const [loading, setLoading] = useState(false)
  const [loadingService, setLoadingService] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const names = activeConnectionId ? (serviceNames[activeConnectionId] || []) : []
  const connServices = activeConnectionId ? (services[activeConnectionId] || {}) : {}

  async function loadServices() {
    if (!activeConnectionId) return
    setLoading(true)
    setError(null)
    try {
      const res = await api.reflect.services(activeConnectionId)
      if ((res as any).code === 'REFLECTION_UNAVAILABLE' || (res as any).error) {
        setError('REFLECTION_DISABLED')
        onServicesLoaded?.(0)
        return
      }
      const names: string[] = res.services ?? []
      setServiceNames(activeConnectionId, names)
      onServicesLoaded?.(names.length)
      // If services came from cache (proto uploaded), show info instead of error
      if (names.length > 0 && !(res as any).fromReflection) {
        setError(null) // clear any previous error, cache is enough
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Failed'
      if (
        msg.includes('REFLECTION_UNAVAILABLE') ||
        msg.includes('reflection') ||
        msg.includes('Unimplemented') ||
        msg.includes('not supported') ||
        msg.includes('502')
      ) {
        setError('REFLECTION_DISABLED')
        onServicesLoaded?.(0)
      } else {
        setError(msg)
      }
    } finally {
      setLoading(false)
    }
  }

  async function expandService(name: string) {
    if (!activeConnectionId) return
    toggleService(name)
    if (connServices[name]) return // already loaded
    setLoadingService(name)
    try {
      const desc = await api.reflect.service(activeConnectionId, name)
      setService(activeConnectionId, name, desc)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to describe service')
    } finally {
      setLoadingService(null)
    }
  }

  useEffect(() => {
    if (activeConnectionId) {
      setError(null)
      loadServices()
    }
  }, [activeConnectionId])

  if (!activeConnectionId) {
    return (
      <div className={styles.empty}>
        <div className={styles.emptyIcon}>⬡</div>
        <p>Connect to a gRPC server<br />to explore services</p>
      </div>
    )
  }

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <span className={styles.headerTitle}>Services</span>
        <button className={styles.refreshBtn} onClick={loadServices} title="Refresh" disabled={loading}>
          <RefreshCw size={12} className={loading ? styles.spinning : ''} />
        </button>
      </div>

      {error && (
        error === 'REFLECTION_DISABLED' ? (
          <div className={styles.reflectionDisabled}>
            <div className={styles.reflectionDisabledIcon}>⚡</div>
            <p className={styles.reflectionDisabledTitle}>Reflection not available</p>
            <p className={styles.reflectionDisabledDesc}>
              This server doesn't support gRPC reflection. Upload a <code>.proto</code> file to explore services.
            </p>
          </div>
        ) : (
          <div className={styles.error}>{error}</div>
        )
      )}

      {loading && names.length === 0 ? (
        <div className={styles.loadingState}>
          <Loader size={14} className={styles.spinning} />
          <span>Loading services…</span>
        </div>
      ) : (
        <div className={styles.tree}>
          {names.map((name) => {
            const isExpanded = expandedServices.has(name)
            const desc = connServices[name]
            const isLoadingThis = loadingService === name
            const shortName = name.split('.').pop() || name
            const pkg = name.includes('.') ? name.split('.').slice(0, -1).join('.') : ''

            return (
              <div key={name} className={styles.serviceGroup}>
                <div
                  className={`${styles.serviceRow} ${isExpanded ? styles.serviceExpanded : ''}`}
                  onClick={() => expandService(name)}
                >
                  <ChevronRight
                    size={13}
                    className={`${styles.chevron} ${isExpanded ? styles.chevronOpen : ''}`}
                  />
                  <span className={styles.serviceName}>{shortName}</span>
                  {pkg && <span className={styles.servicePkg}>{pkg}</span>}
                  {isLoadingThis && <Loader size={11} className={`${styles.spinning} ${styles.loadingDot}`} />}
                </div>

                {isExpanded && desc && (
                  <div className={styles.methods}>
                    {desc.methods.map((m) => (
                      <div
                        key={m.name}
                        className={`${styles.methodRow} ${selectedMethod?.fullName === m.fullName ? styles.methodActive : ''}`}
                        onClick={() => {
                          selectMethod(m)
                          const template = m.inputSchema ? schemaToTemplate(m.inputSchema) : {}
                          const body = Object.keys(template).length > 0 ? JSON.stringify(template, null, 2) : '{}'
                          openFromMethod(name, m.name, body, activeConnectionId)
                        }}
                      >
                        {methodIcon(m)}
                        <span className={styles.methodName}>{m.name}</span>
                        <span className={`${styles.methodBadge} ${styles[`badge${methodType(m).replace('-', '')}`]}`}>
                          {methodType(m)}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
