import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useEditor } from '../store/editor'

interface Entry {
  id: number
  connection_id: number
  database_name: string
  sql_text: string
  duration_ms: number
  rows_affected: number
  error_message: string
  source: string
  executed_at: string
}

interface Props {
  onClose: () => void
}

export default function QueryHistory({ onClose }: Props) {
  const setDraft = useEditor((s) => s.setDraft)
  const [list, setList] = useState<Entry[]>([])
  const [error, setError] = useState<string | null>(null)

  async function load() {
    try {
      const r = await api.get<{ history: Entry[] }>('/api/history')
      setList(r.history ?? [])
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'load failed')
    }
  }

  useEffect(() => { void load() }, [])

  async function removeEntry(id: number) {
    try {
      await api.del(`/api/history/${id}`)
      await load()
    } catch (e) {
      alert(e instanceof ApiError ? e.message : 'delete failed')
    }
  }

  async function clearAll() {
    if (!confirm('Clear ALL history?')) return
    try {
      await api.del('/api/history')
      await load()
    } catch (e) {
      alert(e instanceof ApiError ? e.message : 'clear failed')
    }
  }

  function loadIntoEditor(sql: string) {
    setDraft(sql)
    onClose()
  }

  return (
    <div style={backdrop}>
      <div data-modal style={modal}>
        <header style={header}>
          <h2 style={{ margin: 0 }}>query history</h2>
          <div style={{ display: 'flex', gap: 8 }}>
            <button onClick={clearAll}>clear all</button>
            <button onClick={onClose}>close</button>
          </div>
        </header>
        {error && <div style={{ color: 'var(--danger)', padding: 8 }}>{error}</div>}
        <div style={{ overflow: 'auto', flex: 1 }}>
          {list.length === 0 ? (
            <div style={{ padding: 24, color: 'var(--text-muted)', textAlign: 'center' }}>(no history yet)</div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
              <thead style={{ background: 'var(--table-header-bg)', position: 'sticky', top: 0 }}>
                <tr>
                  <th style={th}>when</th>
                  <th style={th}>sql</th>
                  <th style={th}>ms</th>
                  <th style={th}>rows</th>
                  <th style={th}>error</th>
                  <th style={th}></th>
                </tr>
              </thead>
              <tbody>
                {list.map((e) => (
                  <tr key={e.id}>
                    <td style={td}>{new Date(e.executed_at).toLocaleString()}</td>
                    <td style={tdSql} onClick={() => loadIntoEditor(e.sql_text)}>
                      <code>{e.sql_text.length > 80 ? e.sql_text.slice(0, 80) + '…' : e.sql_text}</code>
                    </td>
                    <td style={td}>{e.duration_ms}</td>
                    <td style={td}>{e.rows_affected}</td>
                    <td style={td}>{e.error_message ? <span style={{ color: 'var(--danger)' }}>{e.error_message}</span> : ''}</td>
                    <td style={td}>
                      <button onClick={() => loadIntoEditor(e.sql_text)}>load</button>{' '}
                      <button onClick={() => removeEntry(e.id)}>delete</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}

const backdrop: CSSProperties = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
  display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
}
const modal: CSSProperties = {
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
  borderRadius: 8, minWidth: 700, maxWidth: '90vw',
  minHeight: 400, maxHeight: '80vh', display: 'flex', flexDirection: 'column',
  fontFamily: 'system-ui', border: '1px solid var(--border-color)',
}
const header: CSSProperties = {
  display: 'flex', justifyContent: 'space-between', alignItems: 'center',
  padding: '12px 16px', borderBottom: '1px solid var(--border-color)',
}
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border-color)' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid var(--table-border)' }
const tdSql: CSSProperties = { ...td, cursor: 'pointer', maxWidth: 400, overflow: 'hidden' }
