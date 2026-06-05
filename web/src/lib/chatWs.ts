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
  ws.onmessage = (m) => {
    try {
      const ev = JSON.parse(m.data) as ChatEvent
      args.onEvent(ev)
      if (ev.type === 'done' || ev.type === 'error') ws.close()
    } catch {
      args.onEvent({ type: 'error', message: 'bad stream message' })
      ws.close()
    }
  }
  ws.onclose = () => args.onClose?.()
  return {
    cancel: () => { try { ws.close() } catch {} },
    send: (payload) => { try { ws.send(JSON.stringify(payload)) } catch {} },
  }
}
