export interface ChatEvent {
  type: 'text' | 'tool_use' | 'tool_result' | 'done' | 'error' | 'write_proposed'
  text?: string
  tool_use_id?: string
  tool_name?: string
  tool_input?: any
  output?: string
  message?: string

  // write_proposed
  proposal_id?: string
  database?: string
  table?: string
  operation?: string
  sql?: string
  explain_summary?: string
}

interface Message {
  role: 'user' | 'assistant' | 'tool'
  content: any[]
}

export function chatStream(args: {
  token: string
  connId: number
  db: string
  provider?: string
  messages: Message[]
  onEvent: (e: ChatEvent) => void
  onClose?: () => void
}): { cancel: () => void; send: (payload: any) => void } {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${proto}://${location.host}/ws/chat?token=${encodeURIComponent(args.token)}`
  const ws = new WebSocket(url)
  ws.onopen = () => {
    ws.send(JSON.stringify({
      type: 'exec',
      conn_id: args.connId,
      db: args.db,
      provider: args.provider ?? '',
      messages: args.messages,
    }))
  }
  // settled = a terminal event (done/error) was already delivered, so we don't
  // fire a second synthetic error from onerror/onclose.
  let settled = false
  ws.onmessage = (m) => {
    try {
      const ev = JSON.parse(m.data) as ChatEvent
      if (ev.type === 'done' || ev.type === 'error') settled = true
      args.onEvent(ev)
      if (ev.type === 'done' || ev.type === 'error') ws.close()
    } catch {
      settled = true
      args.onEvent({ type: 'error', message: 'bad stream message' })
      ws.close()
    }
  }
  ws.onerror = () => {
    if (settled) return
    settled = true
    args.onEvent({ type: 'error', message: 'chat connection error' })
    try { ws.close() } catch {}
  }
  ws.onclose = () => {
    // Closed mid-stream without a done/error — surface it instead of leaving
    // the UI silently stuck "thinking".
    if (!settled) {
      settled = true
      args.onEvent({ type: 'error', message: 'chat connection closed unexpectedly' })
    }
    args.onClose?.()
  }
  return {
    cancel: () => { try { ws.close() } catch {} },
    send: (payload) => { try { ws.send(JSON.stringify(payload)) } catch {} },
  }
}
