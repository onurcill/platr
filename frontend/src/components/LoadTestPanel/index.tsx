import { useState } from 'react'
import {
  Zap, Play, AlertTriangle, CheckCircle,
  ChevronDown, ChevronUp, BarChart2, Clock, TrendingUp, XCircle
} from 'lucide-react'
import { useBillingStore } from '../../stores'
import { api } from '../../api/client'
import styles from './LoadTestPanel.module.css'

interface LoadTestPanelProps {
  connId: string
  service: string
  method: string
  payload: string
  metadata: Record<string, string>
  onClose: () => void
}

export interface LoadTestResult {
  service: string; method: string; concurrency: number; totalCalls: number
  totalDurationMs: number; throughput: number
  successCount: number; errorCount: number; errorRate: number
  latencyMin: number; latencyMean: number; latencyP50: number
  latencyP90: number; latencyP95: number; latencyP99: number
  latencyMax: number; latencyStdDev: number
  buckets: Array<{ second: number; requests: number; errors: number; latencyP50: number; latencyP99: number; throughput: number }>
  errors: Record<string, number>
}

export function LoadTestPanel({ connId, service, method, payload, metadata, onClose }: LoadTestPanelProps) {
  const [concurrency, setConcurrency] = useState(5)
  const [totalCalls, setTotalCalls] = useState(100)
  const [warmupCalls, setWarmupCalls] = useState(10)
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<LoadTestResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [showErrors, setShowErrors] = useState(false)

  const canUse = useBillingStore(s => s.canUseFeature('loadTesting'))
  const currentPlan = useBillingStore(s => s.currentPlan)

  async function run() {
    if (!canUse) return
    setRunning(true)
    setError(null)
    setResult(null)
    try {
      const res = await api.loadTest.run(connId, {
        service, method,
        payload: (() => { try { return JSON.parse(payload) } catch { return {} } })(),
        metadata,
        concurrency,
        totalCalls,
        warmupCalls,
      })
      setResult(res)
    } catch (e: any) {
      setError(e.message)
    } finally {
      setRunning(false)
    }
  }

  const maxBucketReqs = result ? Math.max(...result.buckets.map(b => b.requests), 1) : 1
  const maxP99 = result ? Math.max(...result.buckets.map(b => b.latencyP99), 1) : 1

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <BarChart2 size={14} className={styles.headerIcon} />
          <span className={styles.title}>Load Test</span>
          <span className={styles.target}>{service}/{method}</span>
        </div>
        <button className={styles.closeBtn} onClick={onClose}>×</button>
      </div>

      {/* Plan gate */}
      {!canUse && (
        <div className={styles.planGate}>
          <Zap size={14} />
          <span>Load testing requires <strong>Basic</strong> or higher. Current: <strong>{currentPlan?.displayName ?? 'Free'}</strong></span>
        </div>
      )}

      {/* Config */}
      <div className={styles.config}>
        <div className={styles.configRow}>
          <label>Concurrency</label>
          <input
            type="number" min={1} max={50} value={concurrency}
            onChange={e => setConcurrency(Number(e.target.value))}
            className={styles.numInput}
            disabled={running || !canUse}
          />
          <span className={styles.hint}>parallel workers (max 50)</span>
        </div>
        <div className={styles.configRow}>
          <label>Total requests</label>
          <input
            type="number" min={1} max={5000} value={totalCalls}
            onChange={e => setTotalCalls(Number(e.target.value))}
            className={styles.numInput}
            disabled={running || !canUse}
          />
          <span className={styles.hint}>max 5000</span>
        </div>
        <div className={styles.configRow}>
          <label>Warmup</label>
          <input
            type="number" min={0} value={warmupCalls}
            onChange={e => setWarmupCalls(Number(e.target.value))}
            className={styles.numInput}
            disabled={running || !canUse}
          />
          <span className={styles.hint}>discarded before measuring</span>
        </div>

        <button
          className={`${styles.runBtn} ${running ? styles.runBtnRunning : ''}`}
          onClick={run}
          disabled={running || !canUse}
        >
          {running
            ? <><span className={styles.spinner} /> Running {totalCalls} calls…</>
            : <><Play size={13} /> Run Load Test</>
          }
        </button>
      </div>

      {error && (
        <div className={styles.errorBar}>
          <AlertTriangle size={12} />
          <span>{error}</span>
        </div>
      )}

      {/* Results */}
      {result && (
        <div className={styles.results}>

          {/* Summary row */}
          <div className={styles.summaryRow}>
            <StatCard
              label="Throughput"
              value={`${result.throughput.toFixed(1)}`}
              unit="req/s"
              icon={<TrendingUp size={13} />}
              color="accent"
            />
            <StatCard
              label="Success"
              value={`${result.successCount}`}
              unit={`/ ${result.totalCalls}`}
              icon={<CheckCircle size={13} />}
              color={result.errorCount === 0 ? 'ok' : 'warning'}
            />
            <StatCard
              label="Error rate"
              value={`${(result.errorRate * 100).toFixed(1)}%`}
              icon={<XCircle size={13} />}
              color={result.errorRate === 0 ? 'ok' : result.errorRate > 0.05 ? 'error' : 'warning'}
            />
            <StatCard
              label="Duration"
              value={`${(result.totalDurationMs / 1000).toFixed(2)}s`}
              icon={<Clock size={13} />}
              color="muted"
            />
          </div>

          {/* Latency percentiles */}
          <div className={styles.section}>
            <div className={styles.sectionTitle}>Latency Percentiles</div>
            <div className={styles.percentileGrid}>
              {[
                { label: 'min',    val: result.latencyMin },
                { label: 'mean',   val: result.latencyMean },
                { label: 'p50',    val: result.latencyP50 },
                { label: 'p90',    val: result.latencyP90 },
                { label: 'p95',    val: result.latencyP95,  highlight: true },
                { label: 'p99',    val: result.latencyP99,  highlight: true },
                { label: 'max',    val: result.latencyMax },
                { label: 'σ',      val: result.latencyStdDev },
              ].map(({ label, val, highlight }) => (
                <div key={label} className={`${styles.pctItem} ${highlight ? styles.pctHighlight : ''}`}>
                  <div className={styles.pctLabel}>{label}</div>
                  <div className={styles.pctValue}>{val.toFixed(2)}<span className={styles.pctUnit}>ms</span></div>
                </div>
              ))}
            </div>

            {/* Distribution bar showing p50/p95/p99 relative to max */}
            <div className={styles.distBar}>
              <div className={styles.distBarLabel}>Latency distribution</div>
              <div className={styles.distBarTrack}>
                <div className={styles.distBarFill} style={{ width: `${(result.latencyP50 / result.latencyMax) * 100}%` }}>
                  <span className={styles.distBarTick}>p50</span>
                </div>
                <div className={`${styles.distBarFill} ${styles.distBarP95}`} style={{ width: `${(result.latencyP95 / result.latencyMax) * 100}%` }}>
                  <span className={styles.distBarTick}>p95</span>
                </div>
                <div className={`${styles.distBarFill} ${styles.distBarP99}`} style={{ width: `100%` }}>
                  <span className={styles.distBarTick}>p99={result.latencyP99.toFixed(0)}ms</span>
                </div>
              </div>
            </div>
          </div>

          {/* Time-series chart (SVG sparkline) */}
          {result.buckets.length > 1 && (
            <div className={styles.section}>
              <div className={styles.sectionTitle}>Requests over time</div>
              <div className={styles.chartWrap}>
                <svg width="100%" height="80" viewBox={`0 0 ${result.buckets.length * 20} 80`} preserveAspectRatio="none" className={styles.chart}>
                  {/* Throughput bars */}
                  {result.buckets.map((b, i) => (
                    <rect
                      key={i}
                      x={i * 20 + 1}
                      y={80 - (b.requests / maxBucketReqs) * 70}
                      width={18}
                      height={(b.requests / maxBucketReqs) * 70}
                      className={styles.chartBar}
                    />
                  ))}
                  {/* Error overlay */}
                  {result.buckets.map((b, i) => b.errors > 0 && (
                    <rect
                      key={`e${i}`}
                      x={i * 20 + 1}
                      y={80 - (b.errors / maxBucketReqs) * 70}
                      width={18}
                      height={(b.errors / maxBucketReqs) * 70}
                      className={styles.chartBarError}
                    />
                  ))}
                  {/* P99 line */}
                  {result.buckets.length > 1 && (
                    <polyline
                      points={result.buckets
                        .map((b, i) => `${i * 20 + 10},${80 - (b.latencyP99 / maxP99) * 70}`)
                        .join(' ')}
                      className={styles.chartLine}
                      fill="none"
                    />
                  )}
                </svg>
                <div className={styles.chartLegend}>
                  <span className={styles.legendBar}>■ req/s</span>
                  <span className={styles.legendLine}>— p99 latency</span>
                  {result.errorCount > 0 && <span className={styles.legendError}>■ errors</span>}
                </div>
              </div>
            </div>
          )}

          {/* Error breakdown */}
          {result.errorCount > 0 && (
            <div className={styles.section}>
              <button
                className={styles.sectionTitle}
                onClick={() => setShowErrors(s => !s)}
                style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}
              >
                Error breakdown ({result.errorCount})
                {showErrors ? <ChevronUp size={11} /> : <ChevronDown size={11} />}
              </button>
              {showErrors && (
                <div className={styles.errorList}>
                  {Object.entries(result.errors).map(([msg, count]) => (
                    <div key={msg} className={styles.errorItem}>
                      <span className={styles.errorCount}>{count}×</span>
                      <code className={styles.errorMsg}>{msg}</code>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function StatCard({ label, value, unit, icon, color }: {
  label: string; value: string; unit?: string
  icon: React.ReactNode; color: 'accent' | 'ok' | 'warning' | 'error' | 'muted'
}) {
  return (
    <div className={`${styles.statCard} ${styles[`statCard_${color}`]}`}>
      <div className={styles.statIcon}>{icon}</div>
      <div className={styles.statValue}>{value}{unit && <span className={styles.statUnit}>{unit}</span>}</div>
      <div className={styles.statLabel}>{label}</div>
    </div>
  )
}
