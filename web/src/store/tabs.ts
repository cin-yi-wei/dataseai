import { create } from 'zustand'
import { persist } from 'zustand/middleware'

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
  closeAll: () => void
  closeForConnDB: (connId: number, db: string) => void
  setActive: (id: string) => void
}

export const useTabs = create<State>()(
  persist(
    (set, get) => ({
      tabs: [],
      activeId: null,
      open: (init) => {
        // Reuse an existing table tab instead of opening a duplicate — clicking
        // a table in the sidebar should jump to its open tab if there is one.
        if (init.kind === 'table') {
          const existing = get().tabs.find(
            (t) => t.kind === 'table' && t.connId === init.connId && t.db === init.db && t.table === init.table,
          )
          if (existing) {
            set({ activeId: existing.id })
            return existing.id
          }
        }
        const id = randomID()
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
      closeAll: () => set({ tabs: [], activeId: null }),
      closeForConnDB: (connId, db) => {
        const tabs = get().tabs.filter((t) => !(t.connId === connId && t.db === db))
        let activeId = get().activeId
        if (activeId && !tabs.some((t) => t.id === activeId)) {
          activeId = tabs.length ? tabs[tabs.length - 1].id : null
        }
        set({ tabs, activeId })
      },
      setActive: (id) => set({ activeId: id }),
    }),
    { name: 'dataseai.tabs' },
  ),
)

// randomID generates a unique-enough id without requiring a secure context.
// crypto.randomUUID() throws on http://<lan-ip>/ (non-secure context), which
// was breaking sidebar table clicks under VPN access.
function randomID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    try { return crypto.randomUUID() } catch { /* fall through */ }
  }
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 10)
}
