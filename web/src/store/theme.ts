import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type Theme = 'light' | 'dark'

interface State {
  theme: Theme
  setTheme: (t: Theme) => void
  toggle: () => void
}

export const useTheme = create<State>()(
  persist(
    (set, get) => ({
      theme: 'light',
      setTheme: (t) => {
        set({ theme: t })
        applyTheme(t)
      },
      toggle: () => {
        const next = get().theme === 'light' ? 'dark' : 'light'
        set({ theme: next })
        applyTheme(next)
      },
    }),
    {
      name: 'dataseai-theme',
      onRehydrateStorage: () => (state) => {
        if (state) applyTheme(state.theme)
      },
    },
  ),
)

function applyTheme(t: Theme) {
  document.documentElement.setAttribute('data-theme', t)
}
