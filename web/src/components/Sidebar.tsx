import { useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'
import { useConnections } from '../store/connections'
import { useTabs } from '../store/tabs'
import { useT, tr } from '../i18n'
import { toast } from '../lib/toast'
import { getPinned, togglePinned } from '../lib/pinnedTables'
import { TableContextMenu, TableMenuAction } from './TableContextMenu'
import ConfirmModal from './ConfirmModal'

interface TableInfo {
  name: string
  rows_est: number
  size_mb: number
}

interface Props {
  onPickTable: (db: string, table: string) => void
  onOpenStructure?: (db: string, table: string) => void
  onOpenExport?: (db: string, table: string) => void
  selected?: { db: string; table: string } | null
  /** Desktop width in px, controlled by the workspace resizer. Mobile CSS forces 100%. */
  width?: number
}

export default function Sidebar({ onPickTable, onOpenStructure, onOpenExport, selected, width }: Props) {
  const t = useT()
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
  // Persist the collapse state so it survives any re-render triggered by
  // a sibling action (e.g. clicking a chat DB chip that also calls
  // setActiveDB). On mobile, default to collapsed so the user doesn't
  // get a wall of table rows pushing the chat off-screen every time the
  // AI lists tables.
  const [collapsed, setCollapsedState] = useState<boolean>(() => {
    // On mobile, always start collapsed on a fresh page load — the sidebar
    // takes 50vh and squeezes everything else. User can still expand
    // within the session via the toolbar toggle.
    if (typeof window !== 'undefined' && window.matchMedia('(max-width: 768px)').matches) {
      return true
    }
    try { return localStorage.getItem('dataseai.sidebar.collapsed') === '1' } catch { return false }
  })
  const setCollapsed = (next: boolean | ((v: boolean) => boolean)) => {
    setCollapsedState((prev) => {
      const v = typeof next === 'function' ? next(prev) : next
      try { localStorage.setItem('dataseai.sidebar.collapsed', v ? '1' : '0') } catch { /* ignore */ }
      return v
    })
  }

  const conn = connections.find((c) => c.id === connId)
  const connDefaultDB = conn?.default_db ?? ''

  const [pinned, setPinned] = useState<Set<string>>(new Set())
  const [menu, setMenu] = useState<{ x: number; y: number; table: string } | null>(null)
  const [confirmOp, setConfirmOp] = useState<{ kind: 'truncate' | 'drop'; table: string } | null>(null)
  const [confirmBusy, setConfirmBusy] = useState(false)
  const openTab = useTabs((s) => s.open)

  useEffect(() => {
    if (connId == null || !activeDB) { setPinned(new Set()); return }
    setPinned(getPinned(connId, activeDB))
  }, [connId, activeDB])

  function handleTableContext(e: React.MouseEvent, table: string) {
    e.preventDefault()
    setMenu({ x: e.clientX, y: e.clientY, table })
  }

  async function handleMenuAction(action: TableMenuAction) {
    if (!menu || connId == null || !activeDB) return
    const table = menu.table
    setMenu(null)
    switch (action) {
      case 'open-new-tab':
        openTab({ kind: 'table', connId, db: activeDB, table })
        break
      case 'open-structure':
        onOpenStructure?.(activeDB, table)
        break
      case 'copy-name':
        try { await navigator.clipboard.writeText(table); toast(tr('common.copied')) } catch { /* ignore */ }
        break
      case 'toggle-pin':
        setPinned(togglePinned(connId, activeDB, table))
        break
      case 'export':
        onOpenExport?.(activeDB, table)
        break
      case 'truncate':
        setConfirmOp({ kind: 'truncate', table })
        break
      case 'drop':
        setConfirmOp({ kind: 'drop', table })
        break
    }
  }

  async function executeConfirmOp() {
    if (!confirmOp || connId == null || !activeDB) return
    setConfirmBusy(true)
    const { kind, table } = confirmOp
    const base = `/api/db/${connId}/databases/${encodeURIComponent(activeDB)}/tables/${encodeURIComponent(table)}`
    try {
      if (kind === 'truncate') {
        await api.post(base + '/truncate', null)
      } else {
        await api.del(base)
      }
      setConfirmOp(null)
      // Refresh tables list
      api.get<{ tables: TableInfo[] }>(`/api/db/${connId}/databases/${encodeURIComponent(activeDB)}/tables`)
        .then((r) => setTables(r.tables ?? []))
        .catch(() => {})
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : `${kind} failed`)
    } finally {
      setConfirmBusy(false)
    }
  }

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
        // If activeDB not set AND the connection has a default_db, apply it.
        // When there's no default we leave activeDB null on purpose — the
        // chat scope then falls back to "ask which DB" mode and the picker
        // forces an explicit choice rather than silently grabbing dbs[0].
        if (!activeDB && connDefaultDB && dbs.includes(connDefaultDB)) {
          setActiveDB(connDefaultDB)
        }
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'load failed'))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connId, showSystem])

  // React to the connection's default_db being edited (NOT initial mount —
  // the first effect handles that). When the user clears default_db on an
  // existing connection, drop activeDB so the chat scope goes back to
  // "ask first" and the picker forces a manual choice. When they set it to
  // a new non-empty value, snap activeDB to the new default.
  const closeTabsForConnDB = useTabs((s) => s.closeForConnDB)
  const lastDefaultRef = useRef<{ connId: number | null; value: string } | null>(null)
  useEffect(() => {
    if (connId == null) {
      lastDefaultRef.current = null
      return
    }
    const prev = lastDefaultRef.current
    lastDefaultRef.current = { connId, value: connDefaultDB }
    // Skip the first observation for this connId — that's the initial mount.
    if (!prev || prev.connId !== connId) return
    if (prev.value === connDefaultDB) return
    // Only snap to the new default if it's an entry the sidebar actually lists.
    // Schema-based engines (MSSQL/PG) list schemas here, so a database name
    // like "dev_db" won't match — don't force an invalid activeDB (it would
    // query TABLE_SCHEMA='dev_db', return nothing, and crash the grid).
    setActiveDB(connDefaultDB && databases.includes(connDefaultDB) ? connDefaultDB : null)
    // Close any tab still anchored to the OLD default DB so it can't keep
    // feeding stale scope into the chat (chat reads selected?.db first).
    if (prev.value) {
      closeTabsForConnDB(connId, prev.value)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connId, connDefaultDB])

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

  const asideStyle: CSSProperties = width != null ? { ...sidebar, width } : sidebar

  if (connId == null) {
    return (
      <aside data-sidebar style={asideStyle}>
        <div style={{ color: '#999', fontSize: 13, padding: 16 }}>{t('sidebar.pick_connection')}</div>
      </aside>
    )
  }

  const filtered = tables.filter((tbl) => !filter || tbl.name.toLowerCase().includes(filter.toLowerCase()))
  const list = [...filtered].sort((a, b) => {
    const pa = pinned.has(a.name) ? 0 : 1
    const pb = pinned.has(b.name) ? 0 : 1
    if (pa !== pb) return pa - pb
    return 0
  })

  return (
    <aside data-sidebar data-collapsed={collapsed} style={asideStyle}>
      {/* Always-visible toolbar row: DB picker + sys checkbox + expand/collapse toggle.
          DB picker and sys flag live here (not buried inside the collapsed body)
          so you can switch DB / toggle system DBs without expanding. */}
      <div
        style={{
          display: 'flex', alignItems: 'center', gap: 6, marginBottom: collapsed ? 0 : 8,
          flexWrap: 'wrap',
        }}
      >
        <select
          value={activeDB ?? ''}
          onChange={(e) => setActiveDB(e.target.value || null)}
          style={{
            flex: '1 1 100%', minWidth: 0, padding: '6px 6px', fontSize: 13,
            border: '1px solid var(--border-strong)', borderRadius: 3, boxSizing: 'border-box',
          }}
        >
          <option value="">{t('sidebar.select_database')}</option>
          {databases.map((db) => (
            <option key={db} value={db}>{db}</option>
          ))}
        </select>
        <label style={{ fontSize: 11, color: 'var(--text-muted)', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 3, flexShrink: 0 }}>
          <input
            type="checkbox"
            checked={showSystem}
            onChange={(e) => setShowSystem(e.target.checked)}
            style={{ margin: 0 }}
          />
          {t('sidebar.sys')}
        </label>
        <button
          type="button"
          onClick={() => setCollapsed((v) => !v)}
          style={{
            fontSize: 13, padding: '6px 12px', fontWeight: 600, flexShrink: 0, marginLeft: 'auto',
            background: collapsed ? 'var(--accent)' : undefined,
            color: collapsed ? 'white' : undefined,
            borderColor: collapsed ? 'var(--accent)' : undefined,
          }}
          title={collapsed ? t('sidebar.tap_to_pick_table') : t('sidebar.tap_to_collapse')}
        >
          {collapsed ? t('sidebar.expand_tables') : t('sidebar.collapse')}
        </button>
      </div>

      {/* Full sidebar contents — hidden when collapsed */}
      {!collapsed && (<div>

      <input
        placeholder={t('sidebar.filter_tables')}
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        style={{ width: '100%', padding: '4px 6px', marginBottom: 8, boxSizing: 'border-box' }}
      />
      {error && <div style={{ color: 'crimson', fontSize: 12, marginBottom: 4 }}>{error}</div>}
      {loadingTables && <div style={{ color: '#999', fontSize: 12, padding: 4 }}>{t('common.loading')}</div>}
      {!loadingTables && activeDB && list.length === 0 && (
        <div style={{ color: '#999', fontSize: 12, padding: 4 }}>{t('sidebar.no_tables')}</div>
      )}
      {activeDB && list.map((tbl) => {
        const active = selected && selected.db === activeDB && selected.table === tbl.name
        return (
          <div
            key={tbl.name}
            data-table-row
            onClick={() => {
              onPickTable(activeDB, tbl.name)
              // Auto-collapse on mobile so the user can see the data immediately.
              if (window.matchMedia('(max-width: 768px)').matches) {
                setCollapsed(true)
              }
            }}
            onContextMenu={(e) => handleTableContext(e, tbl.name)}
            title={tbl.name}
            style={{
              cursor: 'pointer', padding: '3px 6px', fontSize: 12,
              background: active ? 'var(--bg-active)' : 'transparent',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              borderRadius: 3,
              display: 'flex', alignItems: 'center',
            }}
          >
            <span style={{ color: 'var(--text-muted)', marginRight: 6, fontFamily: 'monospace', fontSize: 11 }}>▦</span>
            <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis' }}>{tbl.name}</span>
            {pinned.has(tbl.name) && <span style={{ color: 'var(--accent)', fontSize: 11, marginLeft: 4 }}>📌</span>}
          </div>
        )
      })}
      </div>)}
      {menu && (
        <TableContextMenu
          position={{ x: menu.x, y: menu.y }}
          tableName={menu.table}
          isPinned={pinned.has(menu.table)}
          onAction={handleMenuAction}
          onClose={() => setMenu(null)}
        />
      )}
      {confirmOp && (
        <ConfirmModal
          title={confirmOp.kind === 'truncate' ? t('table_menu.truncate') : t('table_menu.drop')}
          body={
            confirmOp.kind === 'truncate'
              ? t('table_menu.truncate_confirm', { table: confirmOp.table })
              : t('table_menu.drop_confirm', { table: confirmOp.table })
          }
          detail={
            confirmOp.kind === 'truncate'
              ? `TRUNCATE TABLE \`${activeDB}\`.\`${confirmOp.table}\`;`
              : `DROP TABLE \`${activeDB}\`.\`${confirmOp.table}\`;`
          }
          confirmLabel={t('common.confirm')}
          cancelLabel={t('common.cancel')}
          danger
          busy={confirmBusy}
          onConfirm={() => void executeConfirmOp()}
          onCancel={() => setConfirmOp(null)}
        />
      )}
    </aside>
  )
}

const sidebar: CSSProperties = {
  width: 220, flexShrink: 0, borderRight: '1px solid var(--border-color)', padding: 8, overflowY: 'auto',
  fontFamily: 'system-ui', boxSizing: 'border-box',
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
}
