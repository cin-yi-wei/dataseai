import { describe, it, expect, beforeEach } from 'vitest'
import { useConnections } from './connections'

describe('useConnections', () => {
  beforeEach(() => {
    useConnections.setState({ list: [], loading: false, error: null })
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
})
