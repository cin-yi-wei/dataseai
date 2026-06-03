import { useState } from 'react'
import type { CSSProperties } from 'react'
import TopBar from '../components/TopBar'
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
import { useActiveConn } from '../store/activeConn'

interface Props {
  onOpenSettings: () => void
}

export default function Workspace({ onOpenSettings }: Props) {
  const [view, setView] = useState<'workspace' | 'connections'>('workspace')
  const [selected, setSelected] = useState<{ db: string; table: string } | null>(null)
  const [bottom, setBottom] = useState<BottomTab>('data')
  const [historyOpen, setHistoryOpen] = useState(false)
  const [importExportOpen, setImportExportOpen] = useState(false)
  const [refresh, setRefresh] = useState(0)
  const connId = useActiveConn((s) => s.activeId)

  if (view === 'connections') {
    return <ConnectionsManager onClose={() => setView('workspace')} />
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TopBar onOpenConnections={() => setView('connections')} onOpenSettings={onOpenSettings} />
      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        <Sidebar onPickTable={(db, table) => setSelected({ db, table })} selected={selected} />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <div style={{ flex: 1, overflow: 'hidden' }}>
            {connId == null && <div style={center}>pick a connection in the top bar</div>}

            {connId != null && bottom === 'sql' && (
              <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                <div style={{ flex: 1, minHeight: 0 }}>
                  <SqlEditor onShowHistory={() => setHistoryOpen(true)} database={selected?.db} />
                </div>
                <div style={{ flex: 1, minHeight: 0 }}>
                  <ResultPanel />
                </div>
              </div>
            )}

            {connId != null && selected == null && bottom !== 'sql' && (
              <div style={center}>pick a table in the sidebar</div>
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
  height: '100%', color: '#999', fontFamily: 'system-ui',
}
