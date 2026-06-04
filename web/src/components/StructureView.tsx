import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface Column {
  name: string
  type: string
  nullable: boolean
  default: string
  extra: string
  comment: string
  key: string
}

interface Structure {
  columns: Column[]
  create_sql: string
}

interface Props {
  db: string
  table: string
}

export default function StructureView({ db, table }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [data, setData] = useState<Structure | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (connId == null) return
    setError(null)
    setData(null)
    api.get<Structure>(`/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}/structure`)
      .then(setData)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'load failed'))
  }, [connId, db, table])

  if (error) return <div style={err}>{error}</div>
  if (!data) return <div style={muted}>loading…</div>
  return (
    <div style={{ height: '100%', overflow: 'auto', padding: 12, fontFamily: 'system-ui' }}>
      <h3 style={{ margin: '0 0 8px' }}>columns</h3>
      <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 13 }}>
        <thead>
          <tr>
            <th style={th}>name</th>
            <th style={th}>type</th>
            <th style={th}>null</th>
            <th style={th}>default</th>
            <th style={th}>key</th>
            <th style={th}>extra</th>
            <th style={th}>comment</th>
          </tr>
        </thead>
        <tbody>
          {data.columns.map((c) => (
            <tr key={c.name}>
              <td style={td}><b>{c.name}</b></td>
              <td style={td}><code>{c.type}</code></td>
              <td style={td}>{c.nullable ? 'YES' : 'NO'}</td>
              <td style={td}>{c.default || <span style={{ color: 'var(--text-disabled)' }}>—</span>}</td>
              <td style={td}>{c.key}</td>
              <td style={td}>{c.extra}</td>
              <td style={td}>{c.comment}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <h3 style={{ marginTop: 16 }}>create sql</h3>
      <pre style={pre}>{data.create_sql}</pre>
    </div>
  )
}

const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border-color)' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid var(--table-border)', fontSize: 13 }
const muted: CSSProperties = { padding: 12, color: 'var(--text-muted)', fontFamily: 'system-ui' }
const err: CSSProperties = { padding: 12, color: 'var(--danger)', fontFamily: 'monospace', fontSize: 13 }
const pre: CSSProperties = { background: 'var(--bg-secondary)', color: 'var(--text-primary)', padding: 12, borderRadius: 6, fontSize: 12, overflow: 'auto', whiteSpace: 'pre' }
