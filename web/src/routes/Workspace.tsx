import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import TopBar from '../components/TopBar'
import TopTabBar from '../components/TopTabBar'
import Sidebar from '../components/Sidebar'
import DataGrid from '../components/DataGrid'
import BottomTabs, { BottomTab } from '../components/BottomTabs'
import ConnectionsManager from '../components/ConnectionsManager'
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
  const [refresh, setRefresh] = useState(0)
  const connId = useActiveConn((s) => s.activeId)
  const activeDB = useActiveConn((s) => s.activeDB)
  const tabs = useTabs((s) => s.tabs)
  const activeId = useTabs((s) => s.activeId)
  const openTab = useTabs((s) => s.open)
  const active = tabs.find((t) => t.id === activeId)
  const selected = active?.kind === 'table' && active.connId === connId ? { db: active.db!, table: active.table! } : null

  useEffect(() => {
    if (active?.kind === 'sql' && bottom !== 'sql') setBottom('sql')
    if (active?.kind === 'table' && bottom === 'sql') setBottom('data')
  }, [active?.kind])

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
          selected={selected}
        />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <div style={{ flex: 1, overflow: 'hidden' }}>
            {connId == null && <div style={center}>{t('sidebar.pick_connection')}</div>}

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
              <ChatPanel database={selected?.db ?? activeDB ?? undefined} />
            )}

            {connId != null && selected == null && bottom !== 'sql' && bottom !== 'chat' && (
              <div style={center}>{t('workspace.pick_table')}</div>
            )}
            {connId != null && selected != null && bottom === 'data' && (
              <DataGrid
                key={`${connId}-${selected.db}-${selected.table}-${refresh}`}
                db={selected.db}
                table={selected.table}
                onWantImportExport={() => setImportExportOpen(true)}
              />
            )}
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
      {importExportOpen && selected && (
        <ImportExportDialog
          db={selected.db}
          table={selected.table}
          onClose={() => setImportExportOpen(false)}
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
