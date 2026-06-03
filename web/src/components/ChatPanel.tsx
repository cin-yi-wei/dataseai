import { FormEvent, useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { useChat } from '../store/chat'
import { useActiveConn } from '../store/activeConn'
import { chatStream } from '../lib/chatWs'

interface Props {
  database?: string
}

export default function ChatPanel({ database }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const messages = useChat((s) => s.messages)
  const busy = useChat((s) => s.busy)
  const error = useChat((s) => s.error)
  const pushUser = useChat((s) => s.pushUser)
  const appendText = useChat((s) => s.appendText)
  const addToolCall = useChat((s) => s.addToolCall)
  const setToolOutput = useChat((s) => s.setToolOutput)
  const reset = useChat((s) => s.reset)
  const setBusy = useChat((s) => s.setBusy)
  const setError = useChat((s) => s.setError)
  const [input, setInput] = useState('')
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const cancelRef = useRef<(() => void) | null>(null)

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [messages])

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!input.trim() || connId == null) return
    const text = input.trim()
    setInput('')
    pushUser(text)
    setBusy(true)
    setError(null)
    // Build the messages payload from store state. Tool results were already
    // captured at the previous turn, so we send the full transcript here.
    const transcript: any[] = []
    for (const m of [...messages, { role: 'user' as const, text, toolCalls: [] }]) {
      if (m.role === 'user') {
        transcript.push({ role: 'user', content: [{ type: 'text', text: m.text }] })
      } else {
        const content: any[] = []
        if (m.text) content.push({ type: 'text', text: m.text })
        for (const tc of m.toolCalls) {
          content.push({ type: 'tool_use', id: tc.id, name: tc.name, input: tc.input })
        }
        if (content.length) transcript.push({ role: 'assistant', content })
        const toolResults: any[] = m.toolCalls
          .filter((tc) => tc.output !== undefined)
          .map((tc) => ({ type: 'tool_result', tool_use_id: tc.id, output: tc.output! }))
        if (toolResults.length) transcript.push({ role: 'tool', content: toolResults })
      }
    }
    const token = localStorage.getItem('mysqlweb.token') ?? ''
    const s = chatStream({
      token,
      connId,
      db: database ?? '',
      messages: transcript,
      onEvent: (ev) => {
        if (ev.type === 'text') appendText(ev.text ?? '')
        else if (ev.type === 'tool_use') addToolCall({ id: ev.tool_use_id!, name: ev.tool_name ?? '', input: ev.tool_input })
        else if (ev.type === 'tool_result') setToolOutput(ev.tool_use_id!, ev.output ?? '')
        else if (ev.type === 'error') setError(ev.message ?? 'chat error')
      },
      onClose: () => {
        setBusy(false)
        cancelRef.current = null
      },
    })
    cancelRef.current = s.cancel
  }

  return (
    <div style={wrap}>
      <div style={bar}>
        <strong>🤖 AI Chat</strong>
        {database && <span style={{ fontSize: 12, color: '#666' }}>db: {database}</span>}
        <span style={{ flex: 1 }} />
        <button onClick={reset}>clear</button>
      </div>
      <div ref={scrollRef} style={msgList}>
        {messages.length === 0 && (
          <div style={{ color: '#999', padding: 16, textAlign: 'center' }}>
            Ask about your data. Try "list databases" or "show me the schema of users".
          </div>
        )}
        {messages.map((m, i) => (
          <div key={i} style={{ padding: 8, borderBottom: '1px solid #f0f0f0' }}>
            <div style={{ fontSize: 11, color: '#888', marginBottom: 2 }}>{m.role}</div>
            {m.text && <div style={{ whiteSpace: 'pre-wrap', fontSize: 14 }}>{m.text}</div>}
            {m.toolCalls.map((tc) => (
              <details key={tc.id} style={{ marginTop: 6, background: '#f5f7fa', borderRadius: 4, padding: 4 }}>
                <summary style={{ fontSize: 12 }}>🔧 {tc.name}({JSON.stringify(tc.input)})</summary>
                <pre style={{ fontSize: 11, margin: 4, whiteSpace: 'pre-wrap', maxHeight: 240, overflow: 'auto' }}>{tc.output ?? '(pending…)'}</pre>
              </details>
            ))}
          </div>
        ))}
        {busy && <div style={{ padding: 8, color: '#888', fontSize: 13 }}>thinking…</div>}
        {error && <div style={{ padding: 8, color: 'crimson', fontSize: 13 }}>{error}</div>}
      </div>
      <form onSubmit={submit} style={form}>
        <input
          autoFocus
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={connId == null ? 'pick a connection first' : 'Ask about your data…'}
          disabled={connId == null || busy}
          style={{ flex: 1, padding: '6px 8px' }}
        />
        <button disabled={connId == null || busy || !input.trim()}>send</button>
      </form>
    </div>
  )
}

const wrap: CSSProperties = { display: 'flex', flexDirection: 'column', height: '100%', fontFamily: 'system-ui' }
const bar: CSSProperties = { display: 'flex', alignItems: 'center', gap: 8, padding: 6, borderBottom: '1px solid #ddd', background: '#fafafa' }
const msgList: CSSProperties = { flex: 1, overflow: 'auto' }
const form: CSSProperties = { display: 'flex', gap: 8, padding: 8, borderTop: '1px solid #ddd' }
