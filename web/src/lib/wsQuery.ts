export interface WSEvent {
  type: 'columns' | 'rows' | 'done' | 'error'
  queryId?: string
  cols?: string[]
  batch?: any[][]
  offset?: number
  total?: number
  durationMs?: number
  message?: string
}

export function streamQuery(args: {
  token: string
  connId: number
  db: string
  sql: string
  onEvent: (e: WSEvent) => void
  onClose?: () => void
}): { cancel: () => void; queryId: string } {
  const queryId = crypto.randomUUID()
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${proto}://${location.host}/ws/query?token=${encodeURIComponent(args.token)}`
  const ws = new WebSocket(url)
  ws.onopen = () => {
    ws.send(JSON.stringify({
      type: 'exec',
      queryId,
      connId: args.connId,
      db: args.db,
      sql: args.sql,
    }))
  }
  ws.onmessage = (m) => {
    try {
      const ev = JSON.parse(m.data) as WSEvent
      args.onEvent(ev)
      if (ev.type === 'done' || ev.type === 'error') ws.close()
    } catch {
      args.onEvent({ type: 'error', queryId, message: 'invalid stream message' })
      ws.close()
    }
  }
  ws.onclose = () => args.onClose?.()
  return {
    queryId,
    cancel: () => {
      try {
        ws.send(JSON.stringify({ type: 'cancel', queryId }))
      } catch {
        // socket may already be closed
      }
      try {
        ws.close()
      } catch {
        // socket may already be closed
      }
    },
  }
}
