import { create } from 'zustand'
import { api, ApiError } from '../lib/api'

export interface Agent {
  id: number
  user_id?: number
  name: string
  last_seen_at?: string | null
  last_ip?: string
  last_os?: string
  last_version?: string
  online?: boolean
  created_at: string
}

interface State {
  list: Agent[]
  loading: boolean
  error: string | null
  lastToken: string | null
  load: () => Promise<void>
  create: (name: string) => Promise<{ agent: Agent; token: string }>
  remove: (id: number) => Promise<void>
  clearLastToken: () => void
}

export const useAgents = create<State>((set, get) => ({
  list: [],
  loading: false,
  error: null,
  lastToken: null,
  load: async () => {
    set({ loading: true, error: null })
    try {
      const r = await api.get<{ agents: Agent[] }>('/api/auth/agents')
      set({ list: r.agents ?? [], loading: false })
    } catch (err) {
      set({ loading: false, error: err instanceof ApiError ? err.message : 'load failed' })
    }
  },
  create: async (name) => {
    const r = await api.post<{ agent: Agent; token: string }>('/api/auth/agents', { name })
    set({ list: [r.agent, ...get().list], lastToken: r.token })
    return r
  },
  remove: async (id) => {
    await api.del(`/api/auth/agents/${id}`)
    set({ list: get().list.filter((a) => a.id !== id) })
  },
  clearLastToken: () => set({ lastToken: null }),
}))
