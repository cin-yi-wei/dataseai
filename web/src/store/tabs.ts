import { create } from 'zustand'

export type TabKind = 'table' | 'sql'

export interface Tab {
  id: string
  kind: TabKind
  connId: number
  db?: string
  table?: string
  title: string
}

interface State {
  tabs: Tab[]
  activeId: string | null
  open: (init: Omit<Tab, 'id' | 'title'>) => string
  close: (id: string) => void
  setActive: (id: string) => void
}

export const useTabs = create<State>((set, get) => ({
  tabs: [],
  activeId: null,
  open: (init) => {
    const id = crypto.randomUUID()
    const title = init.kind === 'sql' ? 'SQL' : init.table ?? '?'
    set({ tabs: [...get().tabs, { id, title, ...init }], activeId: id })
    return id
  },
  close: (id) => {
    const tabs = get().tabs.filter((t) => t.id !== id)
    let activeId = get().activeId
    if (activeId === id) activeId = tabs.length ? tabs[tabs.length - 1].id : null
    set({ tabs, activeId })
  },
  setActive: (id) => set({ activeId: id }),
}))
