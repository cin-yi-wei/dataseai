import { describe, it, expect, beforeEach } from 'vitest'
import { useChat } from './chat'

describe('useChat', () => {
  beforeEach(() => {
    useChat.setState({ messages: [], busy: false, error: null })
  })

  it('appends user message', () => {
    useChat.getState().pushUser('hello')
    const msgs = useChat.getState().messages
    expect(msgs).toHaveLength(1)
    expect(msgs[0].role).toBe('user')
    expect(msgs[0].blocks[0]).toMatchObject({ type: 'text', text: 'hello' })
  })

  it('appends assistant text incrementally', () => {
    useChat.getState().appendText('hi ')
    useChat.getState().appendText('there')
    const msgs = useChat.getState().messages
    const last = msgs[msgs.length - 1]
    expect(last.role).toBe('assistant')
    expect(last.blocks).toHaveLength(1)
    expect(last.blocks[0]).toMatchObject({ type: 'text', text: 'hi there' })
  })

  it('preserves order between text and tool calls', () => {
    useChat.getState().appendText('let me check ')
    useChat.getState().addToolCall({ id: 't1', name: 'list_databases', input: {} })
    useChat.getState().appendText('done')
    useChat.getState().setToolOutput('t1', '["db1"]')
    const msgs = useChat.getState().messages
    const blocks = msgs[msgs.length - 1].blocks
    expect(blocks).toHaveLength(3)
    expect(blocks[0]).toMatchObject({ type: 'text', text: 'let me check ' })
    expect(blocks[1]).toMatchObject({ type: 'tool_call', id: 't1', output: '["db1"]' })
    expect(blocks[2]).toMatchObject({ type: 'text', text: 'done' })
  })
})
