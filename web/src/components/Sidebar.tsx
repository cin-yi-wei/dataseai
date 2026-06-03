import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface TableInfo {
  name: string
  rows_est: number
  size_mb: number
}

interface Props {
  onPickTable: (db: string, table: string) => void
  selected?: { db: string; table: string } | null
}

export default function Sidebar({ onPickTable, selected }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [databases, setDatabases] = useState<string[]>([])
  const [openDB, setOpenDB] = useState<string | null>(null)
  const [tables, setTables] = useState<Record<string, TableInfo[]>>({})
  const [filter, setFilter] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (connId == null) {
      setDatabases([])
      setTables({})
      setOpenDB(null)
      return
    }
    setError(null)
    api.get<{ databases: string[] }>(`/api/db/${connId}/databases`)
      .then((r) => setDatabases(r.databases ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : 'load failed'))
  }, [connId])

  async function expand(db: string) {
    if (openDB === db) {
      setOpenDB(null)
      return
    }
    setOpenDB(db)
    if (tables[db]) return
    try {
      const r = await api.get<{ tables: TableInfo[] }>(`/api/db/${connId}/databases/${encodeURIComponent(db)}/tables`)
      setTables((t) => ({ ...t, [db]: r.tables ?? [] }))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'tables failed')
    }
  }

  if (connId == null) {
    return (
      <aside style={sidebar}>
        <div style={{ color: '#999', fontSize: 13, padding: 16 }}>pick a connection in the top bar</div>
      </aside>
    )
  }

  return (
    <aside style={sidebar}>
      <input
        placeholder="filter tables…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        style={{ width: '100%', padding: '4px 6px', marginBottom: 8, boxSizing: 'border-box' }}
      />
      {error && <div style={{ color: 'crimson', fontSize: 12, marginBottom: 4 }}>{error}</div>}
      {databases.map((db) => {
        const isOpen = openDB === db
        const list = (tables[db] ?? []).filter((t) => !filter || t.name.toLowerCase().includes(filter.toLowerCase()))
        return (
          <div key={db}>
            <div
              onClick={() => void expand(db)}
              style={{ cursor: 'pointer', padding: '4px 0', fontWeight: 600, fontSize: 13 }}
            >
              {isOpen ? '▼' : '▶'} {db}
            </div>
            {isOpen && (
              <div style={{ paddingLeft: 12 }}>
                {list.map((t) => {
                  const active = selected && selected.db === db && selected.table === t.name
                  return (
                    <div
                      key={t.name}
                      onClick={() => onPickTable(db, t.name)}
                      style={{
                        cursor: 'pointer', padding: '2px 4px', fontSize: 12,
                        background: active ? '#cfe2ff' : 'transparent',
                      }}
                    >
                      📋 {t.name}
                    </div>
                  )
                })}
                {list.length === 0 && <div style={{ color: '#999', fontSize: 11, padding: '2px 4px' }}>(empty)</div>}
              </div>
            )}
          </div>
        )
      })}
    </aside>
  )
}

const sidebar: CSSProperties = {
  width: 220, borderRight: '1px solid #ddd', padding: 8, overflowY: 'auto',
  fontFamily: 'system-ui', boxSizing: 'border-box',
}
