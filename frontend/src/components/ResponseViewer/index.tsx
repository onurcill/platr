import { useState, useRef } from 'react'
import Editor, { DiffEditor } from '@monaco-editor/react'
import {
  CheckCircle, XCircle, Clock, Copy, Check, GitCompare,
  ChevronDown, ChevronRight, AlertTriangle, Ban, Wifi
} from 'lucide-react'
import { useTabStore, useThemeStore, useServiceStore } from '../../stores'
import styles from './ResponseViewer.module.css'

// gRPC status codes with colors
const GRPC_STATUSES: Record<string, { label: string; color: string; icon: 'ok' | 'err' | 'warn' }> = {
  'OK':                  { label: 'OK',                  color: '#4ade80', icon: 'ok'   },
  'CANCELLED':           { label: 'CANCELLED',           color: '#fb923c', icon: 'warn' },
  'UNKNOWN':             { label: 'UNKNOWN',             color: '#94a3b8', icon: 'err'  },
  'INVALID_ARGUMENT':    { label: 'INVALID_ARGUMENT',    color: '#f87171', icon: 'err'  },
  'DEADLINE_EXCEEDED':   { label: 'DEADLINE_EXCEEDED',   color: '#fbbf24', icon: 'warn' },
  'NOT_FOUND':           { label: 'NOT_FOUND',           color: '#94a3b8', icon: 'warn' },
  'ALREADY_EXISTS':      { label: 'ALREADY_EXISTS',      color: '#fb923c', icon: 'warn' },
  'PERMISSION_DENIED':   { label: 'PERMISSION_DENIED',   color: '#f87171', icon: 'err'  },
  'RESOURCE_EXHAUSTED':  { label: 'RESOURCE_EXHAUSTED',  color: '#fbbf24', icon: 'warn' },
  'FAILED_PRECONDITION': { label: 'FAILED_PRECONDITION', color: '#f87171', icon: 'err'  },
  'ABORTED':             { label: 'ABORTED',             color: '#fb923c', icon: 'warn' },
  'OUT_OF_RANGE':        { label: 'OUT_OF_RANGE',        color: '#f87171', icon: 'err'  },
  'UNIMPLEMENTED':       { label: 'UNIMPLEMENTED',       color: '#94a3b8', icon: 'err'  },
  'INTERNAL':            { label: 'INTERNAL',            color: '#f87171', icon: 'err'  },
  'UNAVAILABLE':         { label: 'UNAVAILABLE',         color: '#fbbf24', icon: 'warn' },
  'DATA_LOSS':           { label: 'DATA_LOSS',           color: '#f87171', icon: 'err'  },
  'UNAUTHENTICATED':     { label: 'UNAUTHENTICATED',     color: '#f87171', icon: 'err'  },
}

function StatusBadge({ status }: { status: string }) {
  const info = GRPC_STATUSES[status] ?? { label: status, color: '#94a3b8', icon: 'warn' as const }
  const Icon = info.icon === 'ok' ? CheckCircle : info.icon === 'warn' ? AlertTriangle : XCircle
  return (
    <span className={styles.statusBadge} style={{ color: info.color, borderColor: `${info.color}30`, background: `${info.color}12` }}>
      <Icon size={11} />
      {info.label}
    </span>
  )
}

export function ResponseViewer() {
  const tabs = useTabStore(s => s.tabs)
  const activeTabId = useTabStore(s => s.activeTabId)
  const tab = tabs.find(t => t.id === activeTabId) ?? null
  const { selectedMethod } = useServiceStore()
  const response = tab?.response ?? null
  const responseStatus = tab?.responseStatus ?? null
  const responseDuration = tab?.responseDuration ?? null
  const responseHeaders = tab?.responseHeaders ?? {}
  const responseTrailers = tab?.responseTrailers ?? {}
  const isLoading = tab?.isLoading ?? false
  const streamMessages = tab?.streamMessages ?? []
  const isStreaming = tab?.isStreaming ?? false
  const { theme } = useThemeStore()

  const [activeSection, setActiveSection] = useState<'response' | 'headers' | 'trailers'>('response')
  const [copied, setCopied] = useState(false)
  const [diffMode, setDiffMode] = useState(false)
  const [diffBase, setDiffBase] = useState<string | null>(null)

  const monacoTheme = theme === 'dark' ? 'vs-dark' : 'vs'
  const isStream = selectedMethod?.clientStreaming || selectedMethod?.serverStreaming
  const isOk = responseStatus === 'OK'
  const hasData = response !== null || streamMessages.length > 0 || (responseStatus !== null && responseStatus !== 'OK')
  const currentJson = response ? JSON.stringify(response, null, 2) : '{}'

  function copyResponse() {
    const text = isStream
      ? JSON.stringify(streamMessages, null, 2)
      : currentJson
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  function toggleDiff() {
    if (!diffMode) {
      setDiffBase(currentJson)
      setDiffMode(true)
    } else {
      setDiffMode(false)
      setDiffBase(null)
    }
  }

  if (isLoading) {
    return (
      <div className={styles.loading}>
        <span className={styles.spinner} />
        <span>Waiting for response…</span>
      </div>
    )
  }

  if (!hasData && !isStreaming) {
    return (
      <div className={styles.empty}>
        <Wifi size={28} className={styles.emptyIcon} />
        <p>Response will appear here</p>
      </div>
    )
  }

  return (
    <div className={styles.panel}>
      {/* Status bar */}
      <div className={styles.statusBar}>
        <div className={styles.statusLeft}>
          {isStreaming ? (
            <>
              <span className={styles.streamDot} />
              <span className={styles.streamLabel}>Streaming</span>
              <span className={styles.msgCount}>{streamMessages.length} msg</span>
            </>
          ) : responseStatus ? (
            <>
              <StatusBadge status={responseStatus} />
              {responseDuration !== null && (
                <span className={styles.duration}>
                  <Clock size={11} /> {responseDuration}ms
                </span>
              )}
            </>
          ) : null}
        </div>

        <div className={styles.statusCenter}>
          <button
            className={`${styles.sectionTab} ${activeSection === 'response' ? styles.sectionTabActive : ''}`}
            onClick={() => setActiveSection('response')}>
            Response
          </button>
          {!isStream && (
            <>
              <button
                className={`${styles.sectionTab} ${activeSection === 'headers' ? styles.sectionTabActive : ''}`}
                onClick={() => setActiveSection('headers')}>
                Headers
                {Object.keys(responseHeaders).length > 0 && (
                  <span className={styles.sectionBadge}>{Object.keys(responseHeaders).length}</span>
                )}
              </button>
              <button
                className={`${styles.sectionTab} ${activeSection === 'trailers' ? styles.sectionTabActive : ''}`}
                onClick={() => setActiveSection('trailers')}>
                Trailers
                {Object.keys(responseTrailers).length > 0 && (
                  <span className={styles.sectionBadge}>{Object.keys(responseTrailers).length}</span>
                )}
              </button>
            </>
          )}
        </div>

        <div className={styles.statusRight}>
          {!isStream && activeSection === 'response' && hasData && response !== null && (
            <button
              className={`${styles.actionBtn} ${diffMode ? styles.actionBtnActive : ''}`}
              onClick={toggleDiff}
              title={diffMode ? 'Exit diff' : 'Compare with previous response'}
            >
              <GitCompare size={13} />
            </button>
          )}
          {hasData && (
            <button className={styles.actionBtn} onClick={copyResponse} title="Copy response">
              {copied ? <Check size={13} className={styles.copyOk} /> : <Copy size={13} />}
            </button>
          )}
        </div>
      </div>

      {/* Content */}
      <div className={styles.content}>
        {activeSection === 'response' && (
          isStream ? (
            <StreamView messages={streamMessages} />
          ) : response === null && responseStatus && responseStatus !== 'OK' ? (
            <ErrorView status={responseStatus} />
          ) : diffMode && diffBase ? (
            <DiffEditor
              height="100%"
              language="json"
              original={diffBase}
              modified={currentJson}
              theme={monacoTheme}
              options={{
                readOnly: true,
                minimap: { enabled: false },
                fontSize: 13,
                fontFamily: 'JetBrains Mono, monospace',
                scrollBeyondLastLine: false,
                renderSideBySide: true,
                padding: { top: 12 },
              }}
            />
          ) : (
            <Editor
              height="100%"
              language="json"
              value={currentJson}
              theme={monacoTheme}
              options={{
                readOnly: true,
                minimap: { enabled: false },
                fontSize: 13,
                fontFamily: 'JetBrains Mono, monospace',
                lineNumbers: 'on',
                scrollBeyondLastLine: false,
                wordWrap: 'on',
                padding: { top: 12, bottom: 12 },
              }}
            />
          )
        )}
        {activeSection === 'headers' && <MetaTable data={responseHeaders} label="headers" />}
        {activeSection === 'trailers' && <MetaTable data={responseTrailers} label="trailers" />}
      </div>
    </div>
  )
}

function ErrorView({ status }: { status: string }) {
  // Parse "rpc error: code = X desc = Y" format
  const codeMatch = status.match(/code\s*=\s*(\w+)/)
  const descMatch = status.match(/desc\s*=\s*(.+)/)
  const code = codeMatch?.[1] ?? null
  const desc = descMatch?.[1] ?? status
  const info = code ? (GRPC_STATUSES[code] ?? null) : null

  return (
    <div className={styles.errorView}>
      {info && (
        <div className={styles.errorCode} style={{ color: info.color }}>
          {code}
        </div>
      )}
      <div className={styles.errorDesc}>{desc}</div>
      {!codeMatch && <div className={styles.errorRaw}>{status}</div>}
    </div>
  )
}

function StreamView({ messages }: { messages: unknown[] }) {
  return (
    <div className={styles.streamView}>
      {messages.length === 0 && (
        <div className={styles.streamEmpty}>Waiting for messages…</div>
      )}
      {messages.map((msg, i) => {
        const isError = msg && typeof msg === 'object' && '_error' in msg
        const isTrailer = msg && typeof msg === 'object' && '_trailers' in msg
        return (
          <div key={i} className={`${styles.streamMsg} ${isError ? styles.streamMsgError : ''} ${isTrailer ? styles.streamMsgTrailer : ''}`}>
            <span className={styles.streamIdx}>#{i + 1}</span>
            <pre className={styles.streamPre}>{JSON.stringify(msg, null, 2)}</pre>
          </div>
        )
      })}
    </div>
  )
}

function MetaTable({ data, label }: { data: Record<string, string>; label: string }) {
  if (Object.keys(data).length === 0) {
    return <div className={styles.metaEmpty}>No {label}</div>
  }
  return (
    <div className={styles.metaTable}>
      {Object.entries(data).map(([k, v]) => (
        <div key={k} className={styles.metaRow}>
          <span className={styles.metaKey}>{k}</span>
          <span className={styles.metaVal}>{v}</span>
        </div>
      ))}
    </div>
  )
}
