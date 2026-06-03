import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface Index {
  name: string
  columns: string[]
  unique: boolean
  index_type: string
}

interface Props {
  db: string
  table: string
}

export default function IndexesView({ db, table }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [list, setList] = useState<Index[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (connId == null) return
    setError(null)
    setList(null)
    api.get<{ indexes: Index[] }>(`/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}/indexes`)
      .then((r) => setList(r.indexes ?? []))
      .catch((e) => setError(e instanceof ApiError ? e.message : 'load failed'))
  }, [connId, db, table])

  if (error) return <div style={err}>{error}</div>
  if (!list) return <div style={muted}>loading…</div>
  if (list.length === 0) return <div style={muted}>(no indexes)</div>
  return (
    <div style={{ height: '100%', overflow: 'auto', padding: 12, fontFamily: 'system-ui' }}>
      <table style={{ borderCollapse: 'collapse', width: '100%', fontSize: 13 }}>
        <thead>
          <tr>
            <th style={th}>name</th>
            <th style={th}>columns</th>
            <th style={th}>unique</th>
            <th style={th}>type</th>
          </tr>
        </thead>
        <tbody>
          {list.map((i) => (
            <tr key={i.name}>
              <td style={td}><b>{i.name}</b></td>
              <td style={td}>{i.columns.join(', ')}</td>
              <td style={td}>{i.unique ? 'YES' : 'no'}</td>
              <td style={td}>{i.index_type}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3' }
const muted: CSSProperties = { padding: 12, color: '#999', fontFamily: 'system-ui' }
const err: CSSProperties = { padding: 12, color: 'crimson', fontFamily: 'monospace', fontSize: 13 }
