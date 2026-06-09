import { create } from 'zustand'
import { api, ApiError } from '../lib/api'

export type ConnectionEngine = 'mysql' | 'postgres' | 'mssql' | 'bytehouse'

export const ENGINE_DEFAULT_PORTS: Record<ConnectionEngine, number> = {
  mysql: 3306,
  postgres: 5432,
  mssql: 1433,
  bytehouse: 9000,
}

export interface Connection {
  id: number
  name: string
  engine: ConnectionEngine
  host: string
  port: number
  username: string
  default_db: string
  tls: 'disabled' | 'preferred' | 'required' | 'skip-verify'
  color: string
  ssh_enabled?: boolean
  ssh_host?: string
  ssh_port?: number
  ssh_user?: string
  ssh_key_set?: boolean
  via_agent_id?: number | null
  created_at: string
  updated_at: string
}

export interface ConnectionInput {
  name: string
  engine?: ConnectionEngine
  host: string
  port: number
  username: string
  password: string
  default_db?: string
  tls?: 'disabled' | 'preferred' | 'required' | 'skip-verify'
  color?: string
  ssh_enabled?: boolean
  ssh_host?: string
  ssh_port?: number
  ssh_user?: string
  ssh_password?: string
  ssh_key?: string
  ssh_key_passphrase?: string
  via_agent_id?: number | null
}

interface State {
  list: Connection[]
  loading: boolean
  error: string | null
  setList: (l: Connection[]) => void
  load: () => Promise<void>
  create: (in_: ConnectionInput) => Promise<Connection>
  update: (id: number, in_: ConnectionInput) => Promise<Connection>
  remove: (id: number) => Promise<void>
  test: (id: number) => Promise<{ ok: boolean; message: string }>
}

export const useConnections = create<State>((set, get) => ({
  list: [],
  loading: false,
  error: null,
  setList: (l) => set({ list: l }),
  load: async () => {
    set({ loading: true, error: null })
    try {
      const r = await api.get<{ connections: Connection[] }>('/api/connections')
      set({ list: r.connections ?? [], loading: false })
    } catch (err) {
      set({ loading: false, error: err instanceof ApiError ? err.message : 'load failed' })
    }
  },
  create: async (in_) => {
    const r = await api.post<{ connection: Connection }>('/api/connections', in_)
    set({ list: [...get().list, r.connection] })
    return r.connection
  },
  update: async (id, in_) => {
    const r = await api.put<{ connection: Connection }>(`/api/connections/${id}`, in_)
    set({ list: get().list.map((c) => (c.id === id ? r.connection : c)) })
    return r.connection
  },
  remove: async (id) => {
    await api.del(`/api/connections/${id}`)
    set({ list: get().list.filter((c) => c.id !== id) })
  },
  test: async (id) => api.post(`/api/connections/${id}/test`, null),
}))
