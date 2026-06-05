import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import WriteProposalCard from './WriteProposalCard'

const base = {
  proposalId: 'p1',
  db: 'fatgame_development',
  table: 'users',
  op: 'UPDATE',
  sql: 'UPDATE `users` SET is_admin=0 WHERE id=42',
  explainSummary: '',
  status: 'proposed' as const,
}

describe('WriteProposalCard', () => {
  it('renders SQL + Execute/Cancel', () => {
    render(<WriteProposalCard {...base} onDecision={vi.fn()} />)
    expect(screen.getByText(/UPDATE `users`/)).toBeInTheDocument()
    expect(screen.getByText(/Execute/)).toBeInTheDocument()
    expect(screen.getByText(/Cancel/)).toBeInTheDocument()
  })

  it('calls onDecision(true) on Execute click', () => {
    const onDecision = vi.fn()
    render(<WriteProposalCard {...base} onDecision={onDecision} />)
    fireEvent.click(screen.getByText(/Execute/))
    expect(onDecision).toHaveBeenCalledWith('p1', true)
  })

  it('shows executed chip when status flips', () => {
    render(<WriteProposalCard {...base} status="executed" rowsAffected={3} onDecision={vi.fn()} />)
    expect(screen.getByText(/✓ Executed/)).toBeInTheDocument()
  })

  it('shows EXPLAIN error banner when explainSummary has error', () => {
    render(<WriteProposalCard {...base} explainSummary='{"error":"syntax"}' onDecision={vi.fn()} />)
    expect(screen.getByText(/EXPLAIN failed/)).toBeInTheDocument()
  })
})
