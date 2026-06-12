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

  it('renders engine dropdown with MySQL and PostgreSQL options', () => {
    useConnections.setState({ create: vi.fn(), update: vi.fn(), test: vi.fn(), testDraft: vi.fn() })
    render(<ConnectionDialog mode="create" onClose={vi.fn()} onSaved={vi.fn()} />)
    const sel = screen.getByTestId('engine-select') as HTMLSelectElement
    const opts = Array.from(sel.options).map((o) => o.value)
    expect(opts).toContain('mysql')
    expect(opts).toContain('postgres')
    expect(sel.value).toBe('mysql')
    expect(screen.getByText('test')).toBeTruthy()
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
      testDraft: vi.fn(),
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

  it('tests the current unsaved edit draft', async () => {
    const testDraft = vi.fn().mockResolvedValue({ ok: true, message: 'connected' })
    useConnections.setState({
      create: vi.fn(),
      update: vi.fn(),
      test: vi.fn(),
      testDraft,
    })

    render(
      <ConnectionDialog
        mode="edit"
        initial={{
          id: 42,
          name: 'prod',
          engine: 'mysql',
          host: 'old-host',
          port: 3306,
          username: 'old-user',
          default_db: '',
          tls: 'disabled',
          color: '',
          created_at: '',
          updated_at: '',
        }}
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />,
    )

    fireEvent.change(screen.getByLabelText('host'), { target: { value: 'new-host' } })
    fireEvent.change(screen.getByLabelText('user'), { target: { value: 'new-user' } })
    fireEvent.change(screen.getByLabelText('password'), { target: { value: 'draft-pw' } })
    fireEvent.click(screen.getByText('test'))

    await waitFor(() => expect(testDraft).toHaveBeenCalled())
    expect(testDraft.mock.calls[0][0]).toMatchObject({
      id: 42,
      host: 'new-host',
      username: 'new-user',
      password: 'draft-pw',
    })
  })
})
