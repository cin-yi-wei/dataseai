import { afterEach, describe, expect, it, vi } from 'vitest'
import { streamQuery } from './wsQuery'

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((m: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  sent: string[] = []

  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
  }

  send(data: string) {
    this.sent.push(data)
  }

  close() {
    this.onclose?.()
  }
}

describe('streamQuery', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    FakeWebSocket.instances = []
  })

  it('sends maxRows in the websocket exec message', () => {
    vi.stubGlobal('WebSocket', FakeWebSocket)

    streamQuery({
      token: 'tok',
      connId: 7,
      db: 'appdb',
      sql: 'SELECT * FROM users',
      maxRows: 200,
      onEvent: vi.fn(),
    })
    const ws = FakeWebSocket.instances[0]
    ws.onopen?.()

    expect(JSON.parse(ws.sent[0])).toMatchObject({
      type: 'exec',
      connId: 7,
      db: 'appdb',
      sql: 'SELECT * FROM users',
      maxRows: 200,
    })
  })
})
