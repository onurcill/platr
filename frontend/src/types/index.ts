export interface Connection {
  id: string
  name: string
  address: string
  tls: boolean
  state: string
  metadata: Record<string, string>
  createdAt: string
}

export interface FieldSchema {
  type: string
  repeated: boolean
  optional: boolean
  nested?: MessageSchema
}

export interface MessageSchema {
  name: string
  fields: Record<string, FieldSchema>
}

export interface MethodDescriptor {
  name: string
  fullName: string
  clientStreaming: boolean
  serverStreaming: boolean
  inputType: string
  outputType: string
  inputSchema?: MessageSchema
  outputSchema?: MessageSchema
}

export interface ServiceDescriptor {
  name: string
  package: string
  methods: MethodDescriptor[]
}

export interface HistoryEntry {
  id: string
  workspaceId: string
  userId: string
  userName?: string
  connAddress: string
  service: string
  method: string
  requestBody: unknown
  responseBody: unknown
  status: string
  durationMs: number
  createdAt: string
  // legacy compat
  connectionId?: string
  timestamp?: string
}

export interface InvokeResponse {
  historyId: string
  status: string
  headers: Record<string, string>
  trailers: Record<string, string>
  payload: unknown
  durationMs: number
}

export type StreamMessageType = 'message' | 'end' | 'error' | 'trailer'

export interface StreamMessage {
  type: StreamMessageType
  payload?: unknown
  meta?: Record<string, string>
  error?: string
}
