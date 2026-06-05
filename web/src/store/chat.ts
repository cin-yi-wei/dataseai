import { create } from 'zustand'

export interface ToolCallBlock {
  type: 'tool_call'
  id: string
  name: string
  input: any
  output?: string
}

export interface TextBlock {
  type: 'text'
  text: string
}

export type ChatBlock = TextBlock | ToolCallBlock

export interface ChatMessage {
  role: 'user' | 'assistant'
  blocks: ChatBlock[]
}

interface State {
  messages: ChatMessage[]
  busy: boolean
  error: string | null
  pushUser: (text: string) => void
  appendText: (chunk: string) => void
  addToolCall: (tc: Omit<ToolCallBlock, 'type'>) => void
  setToolOutput: (id: string, output: string) => void
  reset: () => void
  setBusy: (b: boolean) => void
  setError: (e: string | null) => void
}

function ensureAssistantLast(msgs: ChatMessage[]): ChatMessage[] {
  if (msgs.length === 0 || msgs[msgs.length - 1].role !== 'assistant') {
    return [...msgs, { role: 'assistant', blocks: [] }]
  }
  return msgs
}

export const useChat = create<State>((set, get) => ({
  messages: [],
  busy: false,
  error: null,
  pushUser: (text) =>
    set({ messages: [...get().messages, { role: 'user', blocks: [{ type: 'text', text }] }] }),
  appendText: (chunk) => {
    const msgs = ensureAssistantLast(get().messages).slice()
    const last = msgs[msgs.length - 1]
    const lastBlock = last.blocks[last.blocks.length - 1]
    if (lastBlock && lastBlock.type === 'text') {
      const blocks = last.blocks.slice()
      blocks[blocks.length - 1] = { type: 'text', text: lastBlock.text + chunk }
      msgs[msgs.length - 1] = { ...last, blocks }
    } else {
      msgs[msgs.length - 1] = { ...last, blocks: [...last.blocks, { type: 'text', text: chunk }] }
    }
    set({ messages: msgs })
  },
  addToolCall: (tc) => {
    const msgs = ensureAssistantLast(get().messages).slice()
    const last = msgs[msgs.length - 1]
    msgs[msgs.length - 1] = {
      ...last,
      blocks: [...last.blocks, { type: 'tool_call', ...tc }],
    }
    set({ messages: msgs })
  },
  setToolOutput: (id, output) => {
    const msgs = get().messages.map((m) => ({
      ...m,
      blocks: m.blocks.map((b) =>
        b.type === 'tool_call' && b.id === id ? { ...b, output } : b,
      ),
    }))
    set({ messages: msgs })
  },
  reset: () => set({ messages: [], busy: false, error: null }),
  setBusy: (b) => set({ busy: b }),
  setError: (e) => set({ error: e }),
}))
