import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'
import { useConnections } from '../store/connections'

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
  const activeDB = useActiveConn((s) => s.activeDB)
  const setActiveDB = useActiveConn((s) => s.setActiveDB)
  const connections = useConnections((s) => s.list)
  const [databases, setDatabases] = useState<string[]>([])
  const [tables, setTables] = useState<TableInfo[]>([])
  const [filter, setFilter] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loadingTables, setLoadingTables] = useState(false)
  const [showSystem, setShowSystem] = useState(false)

  // Load databases when connection changes
  useEffect(() => {
    if (connId == null) {
      setDatabases([])
      setTables([])
      setActiveDB(null)
      return
    }
    setError(null)
    const url = `/api/db/${connId}/databases${showSystem ? '?system=1' : ''}`
    api.get<{ databases: string[] }>(url)
      .then((r) => {
        const dbs = r.databases ?? []
        setDatabases(dbs)
        // If activeDB not set, try to use connection's default_db
        if (!activeDB) {
          const conn = connections.find((c) => c.id === connId)
          const defaultDB = conn?.default_db || ''
          if (defaultDB && dbs.includes(defaultDB)) {
            setActiveDB(defaultDB)
          } else if (dbs.length > 0) {
            setActiveDB(dbs[0])
          }
        }
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'load failed'))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connId, showSystem])

  // Load tables when activeDB changes
  useEffect(() => {
    if (connId == null || !activeDB) {
      setTables([])
      return
    }
    setLoadingTables(true)
    setError(null)
    api.get<{ tables: TableInfo[] }>(`/api/db/${connId}/databases/${encodeURIComponent(activeDB)}/tables`)
      .then((r) => setTables(r.tables ?? []))
      .catch((err) => setError(err instanceof ApiError ? err.message : 'tables failed'))
      .finally(() => setLoadingTables(false))
  }, [connId, activeDB])

  if (connId == null) {
    return (
      <aside style={sidebar}>
        <div style={{ color: '#999', fontSize: 13, padding: 16 }}>pick a connection in the top bar</div>
      </aside>
    )
  }

  const list = tables.filter((t) => !filter || t.name.toLowerCase().includes(filter.toLowerCase()))

  return (
    <aside style={sidebar}>
      {/* Database selector */}
      <div style={{ marginBottom: 8 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 2 }}>
          <label style={{ fontSize: 11, color: 'var(--text-muted)' }}>Database:</label>
          <label style={{ fontSize: 10, color: 'var(--text-muted)', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 3 }}>
            <input
              type="checkbox"
              checked={showSystem}
              onChange={(e) => setShowSystem(e.target.checked)}
              style={{ margin: 0 }}
            />
            sys
          </label>
        </div>
        <select
          value={activeDB ?? ''}
          onChange={(e) => setActiveDB(e.target.value || null)}
          style={{
            width: '100%', padding: '4px 6px', fontSize: 13,
            border: '1px solid var(--border-strong)', borderRadius: 3, boxSizing: 'border-box',
          }}
        >
          {!activeDB && <option value="">— select database —</option>}
          {databases.map((db) => (
            <option key={db} value={db}>{db}</option>
          ))}
        </select>
      </div>

      <input
        placeholder="filter tables…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        style={{ width: '100%', padding: '4px 6px', marginBottom: 8, boxSizing: 'border-box' }}
      />
      {error && <div style={{ color: 'crimson', fontSize: 12, marginBottom: 4 }}>{error}</div>}
      {loadingTables && <div style={{ color: '#999', fontSize: 12, padding: 4 }}>loading…</div>}
      {!loadingTables && activeDB && list.length === 0 && (
        <div style={{ color: '#999', fontSize: 12, padding: 4 }}>(no tables)</div>
      )}
      {activeDB && list.map((t) => {
        const active = selected && selected.db === activeDB && selected.table === t.name
        return (
          <div
            key={t.name}
            onClick={() => onPickTable(activeDB, t.name)}
            title={t.name}
            style={{
              cursor: 'pointer', padding: '3px 6px', fontSize: 12,
              background: active ? 'var(--bg-active)' : 'transparent',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              borderRadius: 3,
            }}
          >
            <span style={{ color: 'var(--text-muted)', marginRight: 6, fontFamily: 'monospace', fontSize: 11 }}>▦</span>{t.name}
          </div>
        )
      })}
    </aside>
  )
}

const sidebar: CSSProperties = {
  width: 220, borderRight: '1px solid var(--border-color)', padding: 8, overflowY: 'auto',
  fontFamily: 'system-ui', boxSizing: 'border-box',
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
}
