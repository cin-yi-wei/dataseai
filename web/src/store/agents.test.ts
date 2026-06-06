import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAgents } from './agents'

describe('useAgents', () => {
  beforeEach(() => {
    useAgents.setState({ list: [], loading: false, error: null, lastToken: null })
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads agents', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      agents: [{ id: 1, name: 'windows', last_seen_at: null, last_os: '', last_version: '', created_at: '' }],
    }), { status: 200 }))

    await useAgents.getState().load()

    expect(useAgents.getState().list).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 1, name: 'windows' }),
    ]))
  })

  it('create stores one-time token', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      agent: { id: 2, name: 'mac', created_at: '' },
      token: 'ag_abc',
    }), { status: 200 }))

    await useAgents.getState().create('mac')

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(fetchMock.mock.calls[0][0]).toBe('/api/auth/agents')
    expect(JSON.parse(init.body as string)).toEqual({ name: 'mac' })
    expect(useAgents.getState().lastToken).toBe('ag_abc')
    expect(useAgents.getState().list[0]).toEqual(expect.objectContaining({ id: 2, name: 'mac' }))
  })
})
