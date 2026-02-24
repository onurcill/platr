import { useState, useRef, useEffect } from 'react'
import { Sparkles, Send, Copy, Check, ChevronDown, RotateCcw, Loader, Lock } from 'lucide-react'
import { useBillingStore } from '../../stores'
import type { MessageSchema, MethodDescriptor } from '../../types'
import styles from './AiAssistant.module.css'

interface AiAssistantProps {
  method: MethodDescriptor | null
  service: string
  currentPayload: string
  currentResponse: unknown
  lastStatus: string
  lastDurationMs: number
  onApplyPayload: (payload: string) => void
}

type Role = 'user' | 'assistant'
interface Message {
  role: Role
  content: string
  timestamp: number
}

// Quick action chip definitions
const QUICK_ACTIONS = [
  { id: 'explain_schema',     label: 'Explain schema',       icon: '📋' },
  { id: 'generate_payload',   label: 'Generate test payload', icon: '⚡' },
  { id: 'explain_error',      label: 'Explain this error',    icon: '🔍' },
  { id: 'suggest_tests',      label: 'Suggest test cases',    icon: '✅' },
  { id: 'edge_cases',         label: 'Find edge cases',       icon: '⚠️' },
]

export function AiAssistant({
  method, service, currentPayload, currentResponse, lastStatus, lastDurationMs, onApplyPayload
}: AiAssistantProps) {
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState<string | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  const canUse = useBillingStore(s => s.canUseFeature('aiAssistant'))
  const currentPlan = useBillingStore(s => s.currentPlan)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  function buildSystemPrompt(): string {
    const schemaDesc = method
      ? `Service: ${service}\nMethod: ${method.name} (${method.clientStreaming ? 'client-streaming' : method.serverStreaming ? 'server-streaming' : 'unary'})\nInput type: ${method.inputType}\nOutput type: ${method.outputType}${method.inputSchema ? `\nInput schema:\n${JSON.stringify(method.inputSchema, null, 2)}` : ''}${method.outputSchema ? `\nOutput schema:\n${JSON.stringify(method.outputSchema, null, 2)}` : ''}`
      : 'No method selected'

    return `You are an expert gRPC API testing assistant embedded in Platr, a gRPC inspection and testing tool.

Current context:
${schemaDesc}

Current request payload:
${currentPayload || '{}'}

${lastStatus ? `Last response status: ${lastStatus}` : ''}
${lastDurationMs ? `Last response time: ${lastDurationMs}ms` : ''}
${currentResponse ? `Last response:\n${JSON.stringify(currentResponse, null, 2).slice(0, 2000)}` : ''}

Your job is to help the developer test this gRPC endpoint. You can:
- Generate realistic, valid JSON payloads that match the proto schema
- Explain gRPC error codes and status messages in plain English
- Suggest edge cases and boundary values worth testing
- Explain what proto fields mean and how they relate
- Write test assertions to validate responses

When you generate a payload, wrap it in a JSON code block so it can be applied with one click:
\`\`\`json
{ ... }
\`\`\`

Be concise and practical. Prefer showing examples over explaining theory.`
  }

  async function sendMessage(text: string) {
    if (!text.trim() || loading || !canUse) return

    const userMsg: Message = { role: 'user', content: text.trim(), timestamp: Date.now() }
    const newMessages = [...messages, userMsg]
    setMessages(newMessages)
    setInput('')
    setLoading(true)

    try {
      const response = await fetch('https://api.anthropic.com/v1/messages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: 'claude-sonnet-4-20250514',
          max_tokens: 1000,
          system: buildSystemPrompt(),
          messages: newMessages.map(m => ({ role: m.role, content: m.content })),
        }),
      })

      const data = await response.json()
      const assistantText = data.content?.find((b: any) => b.type === 'text')?.text ?? 'No response'

      setMessages(prev => [...prev, {
        role: 'assistant',
        content: assistantText,
        timestamp: Date.now(),
      }])
    } catch (e: any) {
      setMessages(prev => [...prev, {
        role: 'assistant',
        content: `Error: ${e.message}`,
        timestamp: Date.now(),
      }])
    } finally {
      setLoading(false)
    }
  }

  function handleQuickAction(actionId: string) {
    const prompts: Record<string, string> = {
      explain_schema:   `Explain the schema for ${method?.name ?? 'this method'} — what does each field do and what values are valid?`,
      generate_payload: `Generate a realistic, complete JSON test payload for ${method?.name ?? 'this method'} with meaningful example values.`,
      explain_error:    `Explain this gRPC response: status="${lastStatus}" after ${lastDurationMs}ms. What does it mean and how can I fix it?`,
      suggest_tests:    `What are the most important test cases for ${method?.name ?? 'this method'}? List 5 scenarios with example payloads.`,
      edge_cases:       `What edge cases and boundary values should I test for ${method?.name ?? 'this method'}? Focus on values that commonly cause bugs.`,
    }
    sendMessage(prompts[actionId] ?? actionId)
  }

  function extractJsonFromMessage(content: string): string | null {
    const match = content.match(/```json\s*([\s\S]*?)```/)
    if (!match) return null
    try {
      JSON.parse(match[1])
      return match[1].trim()
    } catch {
      return null
    }
  }

  function copyText(text: string, key: string) {
    navigator.clipboard.writeText(text)
    setCopied(key)
    setTimeout(() => setCopied(null), 2000)
  }

  function renderMessage(msg: Message, idx: number) {
    const json = msg.role === 'assistant' ? extractJsonFromMessage(msg.content) : null

    // Render content with code block highlighting
    const parts = msg.content.split(/(```(?:json)?\s*[\s\S]*?```)/g)

    return (
      <div key={idx} className={`${styles.message} ${styles[`message_${msg.role}`]}`}>
        {msg.role === 'assistant' && (
          <div className={styles.messageIcon}>
            <Sparkles size={11} />
          </div>
        )}
        <div className={styles.messageBubble}>
          {parts.map((part, i) => {
            if (part.startsWith('```')) {
              const code = part.replace(/^```(?:json)?\s*/, '').replace(/```$/, '').trim()
              return (
                <div key={i} className={styles.codeBlock}>
                  <div className={styles.codeHeader}>
                    <span className={styles.codeLang}>JSON</span>
                    <button
                      className={styles.codeAction}
                      onClick={() => copyText(code, `code-${idx}-${i}`)}
                    >
                      {copied === `code-${idx}-${i}` ? <Check size={11} /> : <Copy size={11} />}
                    </button>
                  </div>
                  <pre className={styles.code}>{code}</pre>
                </div>
              )
            }
            return <p key={i} className={styles.messageText}>{part}</p>
          })}

          {/* Apply payload button */}
          {json && (
            <button className={styles.applyBtn} onClick={() => onApplyPayload(json)}>
              <ChevronDown size={11} />
              Apply this payload
            </button>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className={styles.container}>
      {/* Header */}
      <div className={styles.header}>
        <Sparkles size={13} className={styles.headerIcon} />
        <span className={styles.headerTitle}>AI Assistant</span>
        {!canUse && (
          <span className={styles.planBadge}>
            <Lock size={9} /> Professional+
          </span>
        )}
        {messages.length > 0 && (
          <button className={styles.resetBtn} onClick={() => setMessages([])} title="Clear conversation">
            <RotateCcw size={11} />
          </button>
        )}
      </div>

      {/* Plan gate */}
      {!canUse ? (
        <div className={styles.gateScreen}>
          <div className={styles.gateIcon}><Sparkles size={28} /></div>
          <div className={styles.gateTitle}>AI Assistant</div>
          <div className={styles.gateDesc}>
            Automatically generate payloads, explain errors, and get test suggestions from Claude AI.
          </div>
          <div className={styles.gateFeatures}>
            {['Generate realistic test payloads from schema', 'Explain gRPC errors in plain English', 'Suggest edge cases and test scenarios', 'Apply payload with one click'].map(f => (
              <div key={f} className={styles.gateFeature}>
                <span className={styles.gateFeatureCheck}>✓</span> {f}
              </div>
            ))}
          </div>
          <div className={styles.gatePlan}>Available on <strong>Professional</strong> and <strong>Enterprise</strong></div>
        </div>
      ) : (
        <>
          {/* Quick actions */}
          {messages.length === 0 && (
            <div className={styles.quickActions}>
              <div className={styles.quickActionsLabel}>Quick actions</div>
              <div className={styles.chipGrid}>
                {QUICK_ACTIONS.map(a => (
                  <button
                    key={a.id}
                    className={styles.chip}
                    onClick={() => handleQuickAction(a.id)}
                    disabled={loading || (!lastStatus && a.id === 'explain_error')}
                  >
                    <span>{a.icon}</span>
                    <span>{a.label}</span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Messages */}
          <div className={styles.messages}>
            {messages.map((m, i) => renderMessage(m, i))}
            {loading && (
              <div className={`${styles.message} ${styles.message_assistant}`}>
                <div className={styles.messageIcon}><Sparkles size={11} /></div>
                <div className={`${styles.messageBubble} ${styles.messageBubbleLoading}`}>
                  <Loader size={12} className={styles.loadingSpinner} />
                  <span>Thinking…</span>
                </div>
              </div>
            )}
            <div ref={bottomRef} />
          </div>

          {/* Input */}
          <div className={styles.inputRow}>
            <textarea
              className={styles.input}
              placeholder="Ask anything about this method…"
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  sendMessage(input)
                }
              }}
              rows={2}
              disabled={loading}
            />
            <button
              className={styles.sendBtn}
              onClick={() => sendMessage(input)}
              disabled={!input.trim() || loading}
            >
              <Send size={13} />
            </button>
          </div>
        </>
      )}
    </div>
  )
}
