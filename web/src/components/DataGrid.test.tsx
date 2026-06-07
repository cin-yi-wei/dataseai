import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import DataGrid from './DataGrid'
import { useActiveConn } from '../store/activeConn'
import { useLang } from '../i18n'

describe('DataGrid cell quick-look editing', () => {
  beforeEach(() => {
    useLang.setState({ lang: 'en' })
    useActiveConn.setState({ activeId: 1, activeDB: 'appdb' })
    vi.stubGlobal('confirm', vi.fn(() => true))
    vi.stubGlobal('alert', vi.fn())
  })

  it('edits the first JSON leaf after left-clicking the first cell context-menu item', async () => {
    const patch = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      new Response(JSON.stringify({ affected: 1 }), { status: 200 }),
    )
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (init?.method === 'PATCH') return patch(input, init)
      if (url.includes('/structure')) {
        return new Response(JSON.stringify({
          columns: [
            { name: 'id', type: 'int', key: 'PRI', extra: '', nullable: false, default: '' },
            { name: 'payload', type: 'json', key: '', extra: '', nullable: true, default: '' },
          ],
        }), { status: 200 })
      }
      if (url.includes('/data?')) {
        return new Response(JSON.stringify({
          columns: ['id', 'payload'],
          rows: [[1, '{"name":"Alice","age":30}']],
          total: 1,
          page: 1,
          per_page: 50,
        }), { status: 200 })
      }
      return new Response(JSON.stringify({}), { status: 200 })
    }))

    render(<DataGrid db="appdb" table="users" />)

    await waitFor(() => expect(screen.getByText('{"name":"Alice","age":30}')).toBeInTheDocument())
    fireEvent.contextMenu(screen.getByText('{"name":"Alice","age":30}'), {
      clientX: 10,
      clientY: 10,
    })
    fireEvent.click(screen.getByText('Quick Look Editor'))

    await waitFor(() => expect(screen.getByText(/Quick Look payload/)).toBeInTheDocument())
    const quickLookModal = screen.getByText(/Quick Look payload/).closest('[data-modal]') as HTMLElement
    expect(within(quickLookModal).getByText('[object] - select a leaf node to edit')).toBeInTheDocument()
    fireEvent.click(screen.getByText('▶'))
    fireEvent.click(within(quickLookModal).getByText(/^name/))

    const editor = screen.getByDisplayValue('Alice')
    fireEvent.change(editor, { target: { value: 'Bob' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    const confirmModal = await screen.findByText('Confirm Edit')
    fireEvent.click(within(confirmModal.closest('[data-modal]') as HTMLElement).getByRole('button', { name: 'Confirm Save' }))

    await waitFor(() => expect(patch).toHaveBeenCalled())
    const init = patch.mock.calls[0][1]
    expect(init).toBeDefined()
    expect(JSON.parse(String(init?.body))).toEqual({
      pk_values: { id: 1 },
      column: 'payload',
      new_value: '{"name":"Bob","age":30}',
    })
  })
})
