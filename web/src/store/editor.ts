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
  resultLimit: number
  setResultLimit: (n: number) => void
  error: string | null
  setError: (e: string | null) => void
  busy: boolean
  setBusy: (b: boolean) => void
  running: { queryId: string; cancel: () => void } | null
  setRunning: (r: { queryId: string; cancel: () => void } | null) => void
  appendRows: (cols: string[], rows: any[][]) => void
}

export const useEditor = create<State>((set) => ({
  draft: 'SELECT 1;\n',
  setDraft: (s) => set({ draft: s }),
  result: null,
  setResult: (r) => set({ result: r }),
  resultLimit: 100,
  setResultLimit: (n) => set({ resultLimit: n }),
  error: null,
  setError: (e) => set({ error: e }),
  busy: false,
  setBusy: (b) => set({ busy: b }),
  running: null,
  setRunning: (r) => set({ running: r }),
  appendRows: (cols, rows) => set((s) => ({
    result: s.result
      ? { ...s.result, rows: [...s.result.rows, ...rows] }
      : { columns: cols, rows, rows_affected: 0, duration_ms: 0, truncated: false },
  })),
}))
