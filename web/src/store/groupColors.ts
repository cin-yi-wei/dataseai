import { create } from 'zustand'
import { persist } from 'zustand/middleware'

// Per-group colour for the connection sidebar. Groups are derived from each
// connection's group_name (no group entity in the DB), so the colour is kept
// client-side keyed by group name.
interface State {
  colors: Record<string, string>
  setColor: (group: string, color: string) => void
}

export const useGroupColors = create<State>()(
  persist(
    (set, get) => ({
      colors: {},
      setColor: (group, color) => set({ colors: { ...get().colors, [group]: color } }),
    }),
    { name: 'dataseai.groupColors' },
  ),
)
