import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useTabs } from './tabs'

describe('useTabs', () => {
  beforeEach(() => {
    vi.spyOn(crypto, 'randomUUID')
      .mockReturnValueOnce('00000000-0000-4000-8000-00000000000a')
      .mockReturnValueOnce('00000000-0000-4000-8000-00000000000b')
    useTabs.setState({ tabs: [], activeId: null })
  })

  it('opens a new tab and makes it active', () => {
    const id = useTabs.getState().open({ kind: 'table', connId: 1, db: 'd', table: 't' })
    expect(id).toBe('00000000-0000-4000-8000-00000000000a')
    expect(useTabs.getState().tabs).toHaveLength(1)
    expect(useTabs.getState().activeId).toBe('00000000-0000-4000-8000-00000000000a')
  })

  it('closes a tab and picks a neighbour as active', () => {
    const a = useTabs.getState().open({ kind: 'table', connId: 1, db: 'd', table: 'a' })
    const b = useTabs.getState().open({ kind: 'table', connId: 1, db: 'd', table: 'b' })
    useTabs.getState().close(b)
    expect(useTabs.getState().tabs).toHaveLength(1)
    expect(useTabs.getState().activeId).toBe(a)
  })

  it('closeAll empties tabs and clears activeId', () => {
    useTabs.getState().open({ kind: 'table', connId: 1, db: 'd', table: 'a' })
    useTabs.getState().open({ kind: 'table', connId: 1, db: 'd', table: 'b' })
    useTabs.getState().closeAll()
    expect(useTabs.getState().tabs).toHaveLength(0)
    expect(useTabs.getState().activeId).toBeNull()
  })
})
