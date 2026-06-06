import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import AgentsSection from './AgentsSection'
import { useAgents } from '../store/agents'
import { useLang } from '../i18n'

describe('AgentsSection', () => {
  beforeEach(() => {
    useLang.setState({ lang: 'en' })
    useAgents.setState({
      list: [],
      loading: false,
      error: null,
      lastToken: null,
      load: vi.fn(),
      create: vi.fn().mockResolvedValue({
        agent: { id: 1, name: 'Windows PC', created_at: '' },
        token: 'ag_secret',
      }),
      remove: vi.fn(),
      clearLastToken: vi.fn(),
    })
  })

  it('creates an agent and shows the one-time token', async () => {
    render(<AgentsSection />)
    fireEvent.change(screen.getByPlaceholderText('agent name'), { target: { value: 'Windows PC' } })
    fireEvent.click(screen.getByText('Create agent'))

    await waitFor(() => expect(useAgents.getState().create).toHaveBeenCalledWith('Windows PC'))
    expect(screen.getByText('ag_secret')).toBeInTheDocument()
  })
})
