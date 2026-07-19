import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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

    // 新流程：Apply 只把修改暫存成橘色（不直接寫 DB），Ctrl+S 才跳批次審核視窗，
    // 按「確認送出」才真的 PATCH。
    fireEvent.keyDown(window, { key: 's', ctrlKey: true })
    const review = await screen.findByText('確認送出變更')
    fireEvent.click(within(review.closest('[data-modal]') as HTMLElement).getByRole('button', { name: '確認送出' }))

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

describe('DataGrid pagination limit', () => {
  beforeEach(() => {
    useLang.setState({ lang: 'en' })
    useActiveConn.setState({ activeId: 1, activeDB: 'appdb' })
    vi.stubGlobal('confirm', vi.fn(() => true))
    vi.stubGlobal('alert', vi.fn())
  })

  it('lets users change the table row limit and reloads the first page', async () => {
    const dataRequests: URL[] = []
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input), 'http://localhost')
      if (url.pathname.endsWith('/structure')) {
        return new Response(JSON.stringify({
          columns: [
            { name: 'id', type: 'int', key: 'PRI', extra: '', nullable: false, default: '' },
            { name: 'name', type: 'varchar(255)', key: '', extra: '', nullable: true, default: '' },
          ],
        }), { status: 200 })
      }
      if (url.pathname.endsWith('/data')) {
        dataRequests.push(url)
        return new Response(JSON.stringify({
          columns: ['id', 'name'],
          rows: [[1, 'Alice']],
          total: 125,
          page: Number(url.searchParams.get('page') ?? '1'),
          per_page: Number(url.searchParams.get('per_page') ?? '50'),
        }), { status: 200 })
      }
      return new Response(JSON.stringify({}), { status: 200 })
    }))

    render(<DataGrid db="appdb" table="users" />)

    await waitFor(() => expect(screen.getByText('Alice')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'next ›' }))
    await waitFor(() => {
      expect(dataRequests.some((url) => url.searchParams.get('page') === '2')).toBe(true)
    })

    fireEvent.change(screen.getByLabelText('table row limit'), { target: { value: '100' } })

    await waitFor(() => {
      expect(dataRequests.some((url) =>
        url.searchParams.get('page') === '1' && url.searchParams.get('per_page') === '100',
      )).toBe(true)
    })
    expect(screen.getByText(/100\/page/)).toBeInTheDocument()
    expect(screen.getByText('100/page · 125 rows')).toBeInTheDocument()
  })

  it('cycles column header sorting through asc, desc, and unsorted', async () => {
    const dataRequests: URL[] = []
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input), 'http://localhost')
      if (url.pathname.endsWith('/structure')) {
        return new Response(JSON.stringify({
          columns: [
            { name: 'id', type: 'int', key: 'PRI', extra: '', nullable: false, default: '' },
            { name: 'name', type: 'varchar(255)', key: '', extra: '', nullable: true, default: '' },
          ],
        }), { status: 200 })
      }
      if (url.pathname.endsWith('/data')) {
        dataRequests.push(url)
        return new Response(JSON.stringify({
          columns: ['id', 'name'],
          rows: [[1, 'Alice']],
          total: 1,
          page: 1,
          per_page: 50,
        }), { status: 200 })
      }
      return new Response(JSON.stringify({}), { status: 200 })
    }))

    render(<DataGrid db="appdb" table="users" />)

    await waitFor(() => expect(screen.getByText('Alice')).toBeInTheDocument())

    fireEvent.click(screen.getByText('name'))
    await waitFor(() => {
      const latest = dataRequests.at(-1)
      expect(latest?.searchParams.get('sort_col')).toBe('name')
      expect(latest?.searchParams.get('sort_dir')).toBe('asc')
    })

    fireEvent.click(screen.getByText('name ▲'))
    await waitFor(() => {
      const latest = dataRequests.at(-1)
      expect(latest?.searchParams.get('sort_col')).toBe('name')
      expect(latest?.searchParams.get('sort_dir')).toBe('desc')
    })

    fireEvent.click(screen.getByText('name ▼'))
    await waitFor(() => {
      const latest = dataRequests.at(-1)
      expect(latest?.searchParams.has('sort_col')).toBe(false)
      expect(latest?.searchParams.has('sort_dir')).toBe(false)
    })
  })

  it('keeps the latest page-size response when an older page request finishes later', async () => {
    const dataRequests: URL[] = []
    let resolveStalePage: (() => void) | undefined

    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input), 'http://localhost')
      if (url.pathname.endsWith('/structure')) {
        return new Response(JSON.stringify({
          columns: [
            { name: 'id', type: 'int', key: 'PRI', extra: '', nullable: false, default: '' },
            { name: 'name', type: 'varchar(255)', key: '', extra: '', nullable: true, default: '' },
          ],
        }), { status: 200 })
      }
      if (url.pathname.endsWith('/data')) {
        dataRequests.push(url)
        const page = Number(url.searchParams.get('page') ?? '1')
        const perPage = Number(url.searchParams.get('per_page') ?? '50')
        if (page === 2 && perPage === 50) {
          return new Promise<Response>((resolve) => {
            resolveStalePage = () => resolve(new Response(JSON.stringify({
              columns: ['id', 'name'],
              rows: [[51, 'Stale tail page']],
              total: 1125,
              page,
              per_page: perPage,
            }), { status: 200 }))
          })
        }
        return new Response(JSON.stringify({
          columns: ['id', 'name'],
          rows: [[1, perPage === 1000 ? 'Fresh page size' : 'Initial page']],
          total: 1125,
          page,
          per_page: perPage,
        }), { status: 200 })
      }
      return new Response(JSON.stringify({}), { status: 200 })
    }))

    render(<DataGrid db="appdb" table="users" />)

    await waitFor(() => expect(screen.getByText('Initial page')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: 'next ›' }))
    await waitFor(() => {
      expect(dataRequests.some((url) =>
        url.searchParams.get('page') === '2' && url.searchParams.get('per_page') === '50',
      )).toBe(true)
    })

    fireEvent.change(screen.getByLabelText('table row limit'), { target: { value: '1000' } })

    await waitFor(() => expect(screen.getByText('Fresh page size')).toBeInTheDocument())
    await act(async () => {
      resolveStalePage?.()
    })

    expect(screen.getByText('Fresh page size')).toBeInTheDocument()
    expect(screen.queryByText('Stale tail page')).not.toBeInTheDocument()
  })
})
