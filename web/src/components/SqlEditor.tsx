import { useCallback } from 'react'
import type { CSSProperties } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { sql } from '@codemirror/lang-sql'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'
import { useEditor, QueryResult } from '../store/editor'

interface Props {
  onShowHistory: () => void
  database?: string
}

export default function SqlEditor({ onShowHistory, database }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const draft = useEditor((s) => s.draft)
  const setDraft = useEditor((s) => s.setDraft)
  const setResult = useEditor((s) => s.setResult)
  const setError = useEditor((s) => s.setError)
  const busy = useEditor((s) => s.busy)
  const setBusy = useEditor((s) => s.setBusy)

  const run = useCallback(async () => {
    if (connId == null || !draft.trim()) return
    setBusy(true)
    setError(null)
    try {
      const res = await api.post<QueryResult>('/api/query', {
        conn_id: connId,
        database_name: database ?? '',
        sql: draft,
      })
      setResult(res)
    } catch (err) {
      setResult(null)
      setError(err instanceof ApiError ? err.message : 'query failed')
    } finally {
      setBusy(false)
    }
  }, [connId, draft, database, setBusy, setError, setResult])

  return (
    <div style={wrap}>
      <div style={bar}>
        <button onClick={() => void run()} disabled={busy || connId == null}>
          {busy ? '⏳ running…' : '▶ run (Ctrl+↵)'}
        </button>
        <button onClick={onShowHistory}>📜 history</button>
        <span style={{ flex: 1 }} />
        {database && <span style={{ fontSize: 12, color: '#666' }}>db: {database}</span>}
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>
        <CodeMirror
          value={draft}
          height="100%"
          extensions={[sql()]}
          onChange={setDraft}
          basicSetup={{ lineNumbers: true, foldGutter: true }}
          onKeyDown={(e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
              e.preventDefault()
              void run()
            }
          }}
        />
      </div>
    </div>
  )
}

const wrap: CSSProperties = {
  display: 'flex', flexDirection: 'column', height: '100%',
  fontFamily: 'system-ui',
}
const bar: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, padding: 6,
  borderBottom: '1px solid #ddd', background: '#fafafa',
}
