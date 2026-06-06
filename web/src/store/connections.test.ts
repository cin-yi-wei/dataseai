import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { useConnections } from './connections'

describe('useConnections', () => {
  beforeEach(() => {
    useConnections.setState({ list: [], loading: false, error: null })
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('starts empty', () => {
    expect(useConnections.getState().list).toEqual([])
  })

  it('setList replaces items', () => {
    useConnections.getState().setList([
      { id: 1, name: 'prod', host: 'h', port: 3306, username: 'u', default_db: '', tls: 'disabled', color: '', created_at: '', updated_at: '' },
    ])
    expect(useConnections.getState().list).toHaveLength(1)
  })

  it('create forwards via_agent_id', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      connection: {
        id: 1,
        name: 'local',
        host: '127.0.0.1',
        port: 3306,
        username: 'root',
        default_db: '',
        tls: 'disabled',
        color: '',
        via_agent_id: 7,
        created_at: '',
        updated_at: '',
      },
    }), { status: 200 }))

    await useConnections.getState().create({
      name: 'local',
      host: '127.0.0.1',
      port: 3306,
      username: 'root',
      password: 'pw',
      via_agent_id: 7,
    })

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(JSON.parse(init.body as string)).toMatchObject({ via_agent_id: 7 })
    expect(useConnections.getState().list[0].via_agent_id).toBe(7)
  })
})
