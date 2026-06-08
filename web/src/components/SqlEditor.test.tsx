import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import SqlEditor from './SqlEditor'
import { useActiveConn } from '../store/activeConn'
import { useEditor } from '../store/editor'
import { useLang } from '../i18n'

vi.mock('@codemirror/lang-sql', () => ({
  MySQL: {},
  keywordCompletionSource: () => () => null,
  schemaCompletionSource: () => () => null,
  sql: () => [],
}))

vi.mock('@codemirror/autocomplete', () => ({
  autocompletion: () => [],
}))

vi.mock('@uiw/codemirror-theme-vscode', () => ({
  vscodeDark: [],
  vscodeLight: [],
}))

vi.mock('@uiw/react-codemirror', async () => {
  const React = await import('react')
  return {
    default: React.forwardRef(function MockCodeMirror(props: any, ref: any) {
      React.useImperativeHandle(ref, () => ({
        view: {
          state: {
            selection: { main: { from: 0, to: 0 } },
            doc: { sliceString: () => '' },
          },
        },
      }))
      return (
        <textarea
          aria-label="sql editor"
          value={props.value}
          onChange={(e) => props.onChange?.((e.target as HTMLTextAreaElement).value)}
        />
      )
    }),
  }
})

describe('SqlEditor', () => {
  beforeEach(() => {
    useLang.setState({ lang: 'en' })
    useActiveConn.setState({ activeId: 5, activeDB: 'appdb' })
    useEditor.setState({
      draft: 'SELECT 1 UNION ALL SELECT 2',
      resultLimit: 200,
      result: null,
      error: null,
      busy: false,
      running: null,
    })
  })

  it('sends the selected result limit to REST query execution', async () => {
    const fetchMock = vi.fn(async (...args: [RequestInfo | URL, RequestInit?]) => {
      const [input] = args
      if (String(input).includes('/schema')) {
        return new Response(JSON.stringify({ tables: {} }), { status: 200 })
      }
      return new Response(JSON.stringify({
        columns: ['n'],
        rows: [[1]],
        rows_affected: 0,
        duration_ms: 1,
        truncated: true,
      }), { status: 200 })
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<SqlEditor database="appdb" onShowHistory={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: /run/i }))

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST')
      expect(postCall).toBeDefined()
      expect(JSON.parse(String(postCall?.[1]?.body))).toMatchObject({ max_rows: 200 })
    })
  })
})
