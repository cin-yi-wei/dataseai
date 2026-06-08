import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useEditor } from './editor'

describe('useEditor', () => {
  beforeEach(() => {
    useEditor.setState({ result: null, running: null, busy: false, error: null })
  })

  it('tracks running query cancel handle', () => {
    const cancel = vi.fn()
    useEditor.getState().setRunning({ queryId: 'q1', cancel })
    expect(useEditor.getState().running?.queryId).toBe('q1')
    useEditor.getState().running?.cancel()
    expect(cancel).toHaveBeenCalledOnce()
  })

  it('appends streamed rows to the current result', () => {
    useEditor.getState().setResult({ columns: ['n'], rows: [[1]], rows_affected: 0, duration_ms: 0, truncated: false })
    useEditor.getState().appendRows(['n'], [[2], [3]])
    expect(useEditor.getState().result?.rows).toEqual([[1], [2], [3]])
  })

  it('stores the selected result row limit', () => {
    expect(useEditor.getState().resultLimit).toBe(100)
    useEditor.getState().setResultLimit(200)
    expect(useEditor.getState().resultLimit).toBe(200)
  })
})
