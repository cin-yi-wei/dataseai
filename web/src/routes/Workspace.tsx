import { useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import TopBar from '../components/TopBar'
import TopTabBar from '../components/TopTabBar'
import Sidebar from '../components/Sidebar'
import DataGrid from '../components/DataGrid'
import BottomTabs, { BottomTab } from '../components/BottomTabs'
import ConnectionsManager from '../components/ConnectionsManager'
import ConnectLanding from '../components/ConnectLanding'
import StructureView from '../components/StructureView'
import IndexesView from '../components/IndexesView'
import ForeignKeysView from '../components/ForeignKeysView'
import SqlEditor from '../components/SqlEditor'
import ResultPanel from '../components/ResultPanel'
import QueryHistory from '../components/QueryHistory'
import ImportExportDialog from '../components/ImportExportDialog'
import ChatPanel from '../components/ChatPanel'
import { useActiveConn } from '../store/activeConn'
import { useTabs } from '../store/tabs'
import { useT } from '../i18n'

interface Props {
  onOpenSettings: () => void
  onOpenAdmin?: () => void
}

export default function Workspace({ onOpenSettings, onOpenAdmin }: Props) {
  const t = useT()
  const [view, setView] = useState<'workspace' | 'connections'>('workspace')
  const [bottom, setBottom] = useState<BottomTab>('data')
  const [historyOpen, setHistoryOpen] = useState(false)
  const [importExportOpen, setImportExportOpen] = useState(false)
  const [importExportTarget, setImportExportTarget] = useState<{ db: string; table: string } | null>(null)
  const [refresh, setRefresh] = useState(0)
  // Desktop sidebar width, drag-resizable so long table names aren't clipped.
  // Mobile CSS forces the sidebar to 100% width, so this only affects desktop.
  const SIDEBAR_MIN = 160
  const SIDEBAR_MAX = 640
  const [sidebarWidth, setSidebarWidth] = useState<number>(() => {
    try {
      const v = parseInt(localStorage.getItem('dataseai.sidebar.width') || '', 10)
      if (!Number.isNaN(v) && v >= SIDEBAR_MIN && v <= SIDEBAR_MAX) return v
    } catch { /* ignore */ }
    return 220
  })
  const dragRef = useRef<{ startX: number; startW: number } | null>(null)
  function startSidebarResize(e: React.MouseEvent) {
    e.preventDefault()
    dragRef.current = { startX: e.clientX, startW: sidebarWidth }
    let latest = sidebarWidth
    const onMove = (ev: MouseEvent) => {
      if (!dragRef.current) return
      latest = Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, dragRef.current.startW + (ev.clientX - dragRef.current.startX)))
      setSidebarWidth(latest)
    }
    const onUp = () => {
      dragRef.current = null
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      try { localStorage.setItem('dataseai.sidebar.width', String(latest)) } catch { /* ignore */ }
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
  }
  const connId = useActiveConn((s) => s.activeId)
  const activeDB = useActiveConn((s) => s.activeDB)
  const tabs = useTabs((s) => s.tabs)
  const activeId = useTabs((s) => s.activeId)
  const openTab = useTabs((s) => s.open)
  const active = tabs.find((t) => t.id === activeId)
  const selected = active?.kind === 'table' && active.connId === connId ? { db: active.db!, table: active.table! } : null

  useEffect(() => {
    if (active?.kind === 'sql' && bottom !== 'sql') setBottom('sql')
    if (active?.kind === 'table' && (bottom === 'sql' || bottom === 'chat')) setBottom('data')
  }, [active?.id, active?.kind])

  if (view === 'connections') {
    return <ConnectionsManager onClose={() => setView('workspace')} />
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh', background: 'var(--bg-primary)', color: 'var(--text-primary)' }}>
      <TopBar onOpenConnections={() => setView('connections')} onOpenSettings={onOpenSettings} onOpenAdmin={onOpenAdmin} />
      <TopTabBar />
      <div data-workspace-main style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        <Sidebar
          onPickTable={(db, table) => {
            if (connId != null) openTab({ kind: 'table', connId, db, table })
          }}
          onOpenStructure={(db, table) => {
            if (connId == null) return
            openTab({ kind: 'table', connId, db, table })
            setBottom('structure')
          }}
          onOpenExport={(db, table) => {
            if (connId == null) return
            openTab({ kind: 'table', connId, db, table })
            setBottom('data')
            setImportExportTarget({ db, table })
            setImportExportOpen(true)
          }}
          selected={selected}
          width={sidebarWidth}
        />
        <div
          data-sidebar-resizer
          onMouseDown={startSidebarResize}
          onDoubleClick={() => { setSidebarWidth(220); try { localStorage.setItem('dataseai.sidebar.width', '220') } catch { /* ignore */ } }}
          title={t('sidebar.resize_hint')}
          style={{
            flexShrink: 0, width: 5, cursor: 'col-resize',
            background: 'var(--border-color)', alignSelf: 'stretch',
          }}
        />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <div style={{ flex: 1, overflow: 'hidden' }}>
            {connId == null && <ConnectLanding />}

            {connId != null && bottom === 'sql' && (
              <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                <div style={{ flex: 1, minHeight: 0 }}>
                  <SqlEditor onShowHistory={() => setHistoryOpen(true)} database={selected?.db ?? activeDB ?? undefined} />
                </div>
                <div style={{ flex: 1, minHeight: 0 }}>
                  <ResultPanel />
                </div>
              </div>
            )}

            {connId != null && bottom === 'chat' && (
              <ChatPanel database={activeDB ?? undefined} />
            )}

            {connId != null && selected == null && bottom !== 'sql' && bottom !== 'chat' && (
              <div style={center}>{t('workspace.pick_table')}</div>
            )}
            {connId != null && bottom === 'data' && tabs
              .filter((tb) => tb.kind === 'table' && tb.connId === connId && tb.db != null && tb.table != null)
              .map((tb) => (
                // Keep every open table tab mounted (hidden when inactive) so
                // switching back restores its rows/filters/scroll instantly
                // instead of re-running the query.
                <div
                  key={`${tb.id}-${refresh}`}
                  style={{ display: tb.id === activeId ? 'block' : 'none', height: '100%' }}
                >
                  <DataGrid
                    db={tb.db!}
                    table={tb.table!}
                    onWantImportExport={() => {
                      setImportExportTarget({ db: tb.db!, table: tb.table! })
                      setImportExportOpen(true)
                    }}
                  />
                </div>
              ))}
            {connId != null && selected != null && bottom === 'structure' && (
              <StructureView key={`${connId}-${selected.db}-${selected.table}-s`} db={selected.db} table={selected.table} />
            )}
            {connId != null && selected != null && bottom === 'indexes' && (
              <IndexesView key={`${connId}-${selected.db}-${selected.table}-i`} db={selected.db} table={selected.table} />
            )}
            {connId != null && selected != null && bottom === 'fks' && (
              <ForeignKeysView key={`${connId}-${selected.db}-${selected.table}-f`} db={selected.db} table={selected.table} />
            )}
          </div>
          <BottomTabs value={bottom} onChange={setBottom} hasTable={selected != null} />
        </main>
      </div>
      {historyOpen && <QueryHistory onClose={() => setHistoryOpen(false)} />}
      {importExportOpen && (importExportTarget ?? selected) && (
        <ImportExportDialog
          db={(importExportTarget ?? selected)!.db}
          table={(importExportTarget ?? selected)!.table}
          onClose={() => { setImportExportOpen(false); setImportExportTarget(null) }}
          onImported={() => setRefresh((n) => n + 1)}
        />
      )}
    </div>
  )
}

const center: CSSProperties = {
  display: 'flex', alignItems: 'center', justifyContent: 'center',
  height: '100%', color: 'var(--text-muted)', fontFamily: 'system-ui',
}
