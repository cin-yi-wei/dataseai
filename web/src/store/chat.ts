import { create } from 'zustand'

export interface ToolCall {
  id: string
  name: string
  input: any
  output?: string
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  text: string
  toolCalls: ToolCall[]
}

interface State {
  messages: ChatMessage[]
  busy: boolean
  error: string | null
  pushUser: (text: string) => void
  pushAssistant: () => void
  appendText: (chunk: string) => void
  addToolCall: (tc: ToolCall) => void
  setToolOutput: (id: string, output: string) => void
  reset: () => void
  setBusy: (b: boolean) => void
  setError: (e: string | null) => void
}

export const useChat = create<State>((set, get) => ({
  messages: [],
  busy: false,
  error: null,
  pushUser: (text) => set({ messages: [...get().messages, { role: 'user', text, toolCalls: [] }] }),
  pushAssistant: () => set({ messages: [...get().messages, { role: 'assistant', text: '', toolCalls: [] }] }),
  appendText: (chunk) => {
    const msgs = get().messages.slice()
    if (msgs.length === 0 || msgs[msgs.length - 1].role !== 'assistant') {
      msgs.push({ role: 'assistant', text: '', toolCalls: [] })
    }
    msgs[msgs.length - 1] = { ...msgs[msgs.length - 1], text: msgs[msgs.length - 1].text + chunk }
    set({ messages: msgs })
  },
  addToolCall: (tc) => {
    const msgs = get().messages.slice()
    if (msgs.length === 0 || msgs[msgs.length - 1].role !== 'assistant') {
      msgs.push({ role: 'assistant', text: '', toolCalls: [] })
    }
    const last = msgs[msgs.length - 1]
    msgs[msgs.length - 1] = { ...last, toolCalls: [...last.toolCalls, tc] }
    set({ messages: msgs })
  },
  setToolOutput: (id, output) => {
    const msgs = get().messages.map((m) => ({
      ...m,
      toolCalls: m.toolCalls.map((tc) => (tc.id === id ? { ...tc, output } : tc)),
    }))
    set({ messages: msgs })
  },
  reset: () => set({ messages: [], busy: false, error: null }),
  setBusy: (b) => set({ busy: b }),
  setError: (e) => set({ error: e }),
}))
