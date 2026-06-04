import { create } from 'zustand'

interface State {
  activeId: number | null
  activeDB: string | null
  setActive: (id: number | null) => void
  setActiveDB: (db: string | null) => void
}

export const useActiveConn = create<State>((set) => ({
  activeId: null,
  activeDB: null,
  setActive: (id) => set({ activeId: id, activeDB: null }),
  setActiveDB: (db) => set({ activeDB: db }),
}))
