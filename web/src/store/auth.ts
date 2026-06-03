import { create } from 'zustand'
import { api, getToken, setToken } from '../lib/api'

export interface User {
  id: number
  username: string
}

interface AuthState {
  user: User | null
  ready: boolean
  login: (token: string, user: User) => void
  logout: () => Promise<void>
  bootstrap: () => Promise<void>
}

export const useAuth = create<AuthState>((set) => ({
  user: null,
  ready: false,
  login: (token, user) => {
    setToken(token)
    set({ user })
  },
  logout: async () => {
    try {
      await api.post('/api/auth/logout', null)
    } catch {
      // ignore — server may have already revoked
    }
    setToken(null)
    set({ user: null })
  },
  bootstrap: async () => {
    if (!getToken()) {
      set({ ready: true })
      return
    }
    try {
      const me = await api.get<{ user: User }>('/api/auth/me')
      set({ user: me.user, ready: true })
    } catch {
      setToken(null)
      set({ user: null, ready: true })
    }
  },
}))
