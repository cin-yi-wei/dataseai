import { beforeEach, describe, expect, it } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import ResultPanel from './ResultPanel'
import { useEditor } from '../store/editor'

describe('ResultPanel', () => {
  beforeEach(() => {
    useEditor.setState({
      resultLimit: 100,
      result: { columns: ['n'], rows: [[1]], rows_affected: 0, duration_ms: 3, truncated: false },
      error: null,
    })
  })

  it('changes the row limit used by SQL editor queries', () => {
    render(<ResultPanel />)

    fireEvent.change(screen.getByLabelText('row limit'), { target: { value: '200' } })

    expect(useEditor.getState().resultLimit).toBe(200)
  })
})
