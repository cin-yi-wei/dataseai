import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import AIWritePolicyTable from './AIWritePolicyTable'

const baseProps = {
  connId: 1,
  db: 'db1',
  configured: [
    { table: 't1', policy: { insert: true,  update: false, delete: false, ddl: false } },
    { table: 't2', policy: { insert: false, update: true,  delete: true,  ddl: false } },
  ],
  unconfigured: ['t3', 't4'],
  onUpsert: vi.fn(),
  onBatch:  vi.fn(),
}

describe('AIWritePolicyTable', () => {
  it('renders configured rows', () => {
    render(<AIWritePolicyTable {...baseProps} />)
    expect(screen.getByText('t1')).toBeInTheDocument()
    expect(screen.getByText('t2')).toBeInTheDocument()
  })

  it('select-all toggles all four checkboxes in a row', () => {
    const onUpsert = vi.fn()
    render(<AIWritePolicyTable {...baseProps} onUpsert={onUpsert} />)
    const row = screen.getByText('t1').closest('tr')!
    const all = row.querySelector('[data-testid="select-all"]')! as HTMLInputElement
    fireEvent.click(all)
    expect(onUpsert).toHaveBeenCalledWith(1, 'db1', 't1',
      { insert: true, update: true, delete: true, ddl: true })
  })

  it('batch-applies to selected unconfigured tables', () => {
    const onBatch = vi.fn()
    render(<AIWritePolicyTable {...baseProps} onBatch={onBatch} />)
    fireEvent.click(screen.getByLabelText('select t3'))
    fireEvent.click(screen.getByTestId('batch-ins'))
    fireEvent.click(screen.getByText(/Apply to 1 selected/))
    expect(onBatch).toHaveBeenCalledWith(1, 'db1', ['t3'],
      { insert: true, update: false, delete: false, ddl: false })
  })
})
