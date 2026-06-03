import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { api, setToken } from './api'

describe('api helpers', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  test('patch sends JSON body and bearer token', async () => {
    setToken('tok-123')
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{"ok":true}', { status: 200 }))

    await api.patch('/x', { a: 1 })

    expect(fetchMock).toHaveBeenCalledWith('/x', expect.objectContaining({
      method: 'PATCH',
      body: JSON.stringify({ a: 1 }),
    }))
    const init = fetchMock.mock.calls[0][1] as RequestInit
    const headers = init.headers as Headers
    expect(headers.get('Authorization')).toBe('Bearer tok-123')
    expect(headers.get('Content-Type')).toBe('application/json')
  })

  test('deleteWithBody sends JSON body', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}', { status: 200 }))

    await api.deleteWithBody('/x', { id: 1 })

    expect(fetchMock).toHaveBeenCalledWith('/x', expect.objectContaining({
      method: 'DELETE',
      body: JSON.stringify({ id: 1 }),
    }))
  })
})
