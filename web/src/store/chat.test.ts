import { describe, it, expect, beforeEach } from 'vitest'
import { useChat } from './chat'

describe('useChat', () => {
  beforeEach(() => {
    useChat.setState({ messages: [], busy: false, error: null })
  })

  it('appends user message', () => {
    useChat.getState().pushUser('hello')
    expect(useChat.getState().messages).toHaveLength(1)
    expect(useChat.getState().messages[0].role).toBe('user')
  })

  it('appends assistant text incrementally', () => {
    useChat.getState().pushAssistant()
    useChat.getState().appendText('hi ')
    useChat.getState().appendText('there')
    const m = useChat.getState().messages
    expect(m[m.length - 1].role).toBe('assistant')
    expect(m[m.length - 1].text).toBe('hi there')
  })
})
