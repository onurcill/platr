import type { Connection, ServiceDescriptor, HistoryEntry, InvokeResponse } from '../types'

const BASE = '/api'

function authHeaders(): Record<string, string> {
  try {
    const raw = localStorage.getItem('grpc-inspector-auth')
    if (raw) {
      const state = JSON.parse(raw)
      const token = state?.state?.token
      if (token) return { Authorization: `Bearer ${token}` }
    }
  } catch {}
  return {}
}

export async function apiFetch(path: string, options?: RequestInit): Promise<Response> {
  return fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    ...options,
  })
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    ...options,
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
  return data as T
}

function ws(workspaceId: string | null) {
  return workspaceId ? `?workspaceId=${workspaceId}` : ''
}

export const api = {
  connections: {
    create: (body: { address: string; name?: string; tls?: boolean; insecure?: boolean; metadata?: Record<string, string> }) =>
      request<Connection>('/connections', { method: 'POST', body: JSON.stringify(body) }),
    list: () => request<Connection[]>('/connections'),
    delete: (id: string) => request<void>(`/connections/${id}`, { method: 'DELETE' }),
    test: (id: string) => request<{ id: string; ok: boolean; state: string }>(`/connections/${id}/test`, { method: 'POST' }),
  },

  reflect: {
    services: (connId: string) =>
      request<{ connectionId: string; services: string[] }>(`/connections/${connId}/reflect/services`),
    service: (connId: string, service: string) =>
      request<ServiceDescriptor>(`/connections/${connId}/reflect/service/${encodeURIComponent(service)}`),
  },

  invoke: {
    unary: (connId: string, body: unknown, workspaceId?: string | null, envId?: string | null) => {
      const params = new URLSearchParams()
      if (workspaceId) params.set('workspaceId', workspaceId)
      if (envId) params.set('envId', envId)
      const qs = params.toString() ? `?${params}` : ''
      return request<InvokeResponse>(`/connections/${connId}/invoke${qs}`, {
        method: 'POST',
        body: JSON.stringify(body),
      })
    },
  },

  history: {
    list: (workspaceId: string) =>
      request<HistoryEntry[]>(`/history${ws(workspaceId)}`),
    delete: (id: string, workspaceId: string) =>
      request<void>(`/history/${id}${ws(workspaceId)}`, { method: 'DELETE' }),
    clear: (workspaceId: string) =>
      request<void>(`/history${ws(workspaceId)}`, { method: 'DELETE' }),
  },

  environments: {
    list: (workspaceId: string) =>
      request<import('../stores/environmentStore').Environment[]>(`/environments${ws(workspaceId)}`),
    create: (workspaceId: string, body: { name: string; color: string }) =>
      request<import('../stores/environmentStore').Environment>(`/environments${ws(workspaceId)}`, {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    get: (id: string) =>
      request<import('../stores/environmentStore').Environment>(`/environments/${id}`),
    update: (id: string, body: { name?: string; color?: string; variables?: unknown[] }) =>
      request<import('../stores/environmentStore').Environment>(`/environments/${id}`, {
        method: 'PUT',
        body: JSON.stringify(body),
      }),
    delete: (id: string) =>
      request<void>(`/environments/${id}`, { method: 'DELETE' }),
    duplicate: (id: string) =>
      request<import('../stores/environmentStore').Environment>(`/environments/${id}/duplicate`, { method: 'POST' }),
  },
  billing: {
    getSubscription: () =>
      request<{ subscription: import('../stores/billingStore').Subscription; usage: import('../stores/billingStore').UsageSummary; plan: import('../stores/billingStore').PlanConfig; plans: import('../stores/billingStore').PlanConfig[] }>('/billing/subscription'),
    listPlans: () =>
      request<import('../stores/billingStore').PlanConfig[]>('/billing/plans'),
    createCheckout: (plan: string, interval: 'monthly' | 'yearly') =>
      request<{ url: string; sessionId?: string; mock?: string }>('/billing/checkout', {
        method: 'POST',
        body: JSON.stringify({ plan, interval }),
      }),
    createPortal: () =>
      request<{ url: string }>('/billing/portal', { method: 'POST' }),
  },

  loadTest: {
    run: (connId: string, body: {
      service: string; method: string; payload: unknown
      metadata?: Record<string,string>; concurrency: number
      totalCalls: number; warmupCalls?: number
    }) =>
      request<{
        service: string; method: string; concurrency: number; totalCalls: number
        totalDurationMs: number; throughput: number
        successCount: number; errorCount: number; errorRate: number
        latencyMin: number; latencyMean: number; latencyP50: number
        latencyP90: number; latencyP95: number; latencyP99: number
        latencyMax: number; latencyStdDev: number
        buckets: Array<{ second: number; requests: number; errors: number; latencyP50: number; latencyP99: number; throughput: number }>
        errors: Record<string, number>
      }>(`/connections/${connId}/loadtest`, { method: 'POST', body: JSON.stringify(body) }),
  },
}

// ── WebSocket streaming ──────────────────────────────────────

export function createStream(
  connId: string,
  init: { service: string; method: string; metadata?: Record<string, string> },
  onMessage: (msg: unknown) => void,
  onEnd: (trailers?: Record<string, string>) => void,
  onError: (err: string) => void,
): { send: (payload: unknown) => void; close: () => void } {
  const wsUrl = `ws://localhost:8080/api/connections/${connId}/stream`
  const ws = new WebSocket(wsUrl)

  ws.onopen = () => ws.send(JSON.stringify(init))

  ws.onmessage = (e) => {
    const msg = JSON.parse(e.data)
    switch (msg.type) {
      case 'message': onMessage(msg.payload); break
      case 'end': onEnd(); ws.close(); break
      case 'trailer': onEnd(msg.meta); break
      case 'error': onError(msg.error); ws.close(); break
    }
  }

  ws.onerror = () => onError('WebSocket error')

  return {
    send: (payload) => ws.send(JSON.stringify({ type: 'message', payload })),
    close: () => { ws.send(JSON.stringify({ type: 'end' })); ws.close() },
  }
}
