import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface FK {
  name: string
  column: string
  ref_table: string
  ref_column: string
  on_delete: string
  on_update: string
}

interface Props {
  db: string
  table: string
}

export default function ForeignKeysView({ db, table }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [list, setList] = useState<FK[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (connId == null) return
    setError(null)
    setList(null)
    api.get<{ fks: FK[] }>(`/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}/fks`)
      .then((r) => setList(r.fks ?? []))
      .catch((e) => setError(e instanceof ApiError ? e.message : 'load failed'))
  }, [connId, db, table])

  if (error) return <div style={err}>{error}</div>
  if (!list) return <div style={muted}>loading…</div>
  if (list.length === 0) return <div style={muted}>(no foreign keys)</div>
  return (
    <div style={{ height: '100%', overflow: 'auto', padding: 12, fontFamily: 'system-ui' }}>
      <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 13 }}>
        <thead>
          <tr>
            <th style={th}>name</th>
            <th style={th}>column</th>
            <th style={th}>references</th>
            <th style={th}>on delete</th>
            <th style={th}>on update</th>
          </tr>
        </thead>
        <tbody>
          {list.map((f) => (
            <tr key={f.name}>
              <td style={td}><b>{f.name}</b></td>
              <td style={td}>{f.column}</td>
              <td style={td}>{f.ref_table}.{f.ref_column}</td>
              <td style={td}>{f.on_delete}</td>
              <td style={td}>{f.on_update}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border-color)' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid var(--table-border)' }
const muted: CSSProperties = { padding: 12, color: 'var(--text-muted)', fontFamily: 'system-ui' }
const err: CSSProperties = { padding: 12, color: 'var(--danger)', fontFamily: 'monospace', fontSize: 13 }
