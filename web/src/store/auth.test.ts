import { describe, it, expect, beforeEach } from 'vitest'
import { useAuth } from './auth'
import { setToken } from '../lib/api'

describe('useAuth', () => {
  beforeEach(() => {
    localStorage.clear()
    useAuth.setState({ user: null, ready: false })
  })

  it('starts logged out', () => {
    expect(useAuth.getState().user).toBeNull()
  })

  it('login sets token and user', () => {
    useAuth.getState().login('tok-xyz', { id: 1, username: 'alice' })
    expect(localStorage.getItem('dataseai.token')).toBe('tok-xyz')
    expect(useAuth.getState().user?.username).toBe('alice')
  })

  it('logout clears token + user', async () => {
    setToken('tok-xyz')
    useAuth.setState({ user: { id: 1, username: 'alice' } })
    await useAuth.getState().logout()
    expect(localStorage.getItem('dataseai.token')).toBeNull()
    expect(useAuth.getState().user).toBeNull()
  })
})
