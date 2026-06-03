import { create } from 'zustand'

interface State {
  activeId: number | null
  setActive: (id: number | null) => void
}

export const useActiveConn = create<State>((set) => ({
  activeId: null,
  setActive: (id) => set({ activeId: id }),
}))
