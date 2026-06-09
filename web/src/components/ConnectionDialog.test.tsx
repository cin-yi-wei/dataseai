import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import ConnectionDialog from './ConnectionDialog'
import { useConnections } from '../store/connections'
import { useAgents } from '../store/agents'
import { useLang } from '../i18n'

describe('ConnectionDialog', () => {
  beforeEach(() => {
    useLang.setState({ lang: 'en' })
    useAgents.setState({
      list: [{ id: 7, name: 'Windows PC', created_at: '' }],
      loading: false,
      error: null,
      lastToken: null,
    })
  })

  it('submits selected agent id with connection input', async () => {
    const create = vi.fn().mockResolvedValue({
      id: 1,
      name: 'local',
      engine: 'mysql',
      host: '127.0.0.1',
      port: 3306,
      username: 'root',
      default_db: '',
      tls: 'disabled',
      color: '',
      via_agent_id: 7,
      created_at: '',
      updated_at: '',
    })
    useConnections.setState({
      create,
      update: vi.fn(),
      test: vi.fn(),
    })

    render(<ConnectionDialog mode="create" onClose={vi.fn()} onSaved={vi.fn()} />)
    fireEvent.change(screen.getByLabelText('name'), { target: { value: 'local' } })
    fireEvent.change(screen.getByLabelText('user'), { target: { value: 'root' } })
    fireEvent.change(screen.getByLabelText('password'), { target: { value: 'pw' } })
    fireEvent.change(screen.getByLabelText('via agent'), { target: { value: '7' } })
    fireEvent.click(screen.getByText('Save'))

    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(create.mock.calls[0][0]).toMatchObject({ via_agent_id: 7 })
  })
})
