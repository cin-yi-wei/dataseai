import { create } from 'zustand'

export interface QueryResult {
  columns: string[]
  rows: any[][]
  rows_affected: number
  duration_ms: number
  truncated: boolean
}

interface State {
  draft: string
  setDraft: (s: string) => void
  result: QueryResult | null
  setResult: (r: QueryResult | null) => void
  error: string | null
  setError: (e: string | null) => void
  busy: boolean
  setBusy: (b: boolean) => void
}

export const useEditor = create<State>((set) => ({
  draft: 'SELECT 1;\n',
  setDraft: (s) => set({ draft: s }),
  result: null,
  setResult: (r) => set({ result: r }),
  error: null,
  setError: (e) => set({ error: e }),
  busy: false,
  setBusy: (b) => set({ busy: b }),
}))
