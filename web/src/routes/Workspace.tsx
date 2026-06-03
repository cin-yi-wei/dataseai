import { useState } from 'react'
import type { CSSProperties } from 'react'
import TopBar from '../components/TopBar'
import Sidebar from '../components/Sidebar'
import DataGrid from '../components/DataGrid'
import BottomTabs, { BottomTab } from '../components/BottomTabs'
import ConnectionsManager from '../components/ConnectionsManager'
import { useActiveConn } from '../store/activeConn'

interface Props {
  onOpenSettings: () => void
}

export default function Workspace({ onOpenSettings }: Props) {
  const [view, setView] = useState<'workspace' | 'connections'>('workspace')
  const [selected, setSelected] = useState<{ db: string; table: string } | null>(null)
  const [bottom, setBottom] = useState<BottomTab>('data')
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
            {connId == null && (
              <div style={center}>pick a connection in the top bar</div>
            )}
            {connId != null && selected == null && (
              <div style={center}>pick a table in the sidebar</div>
            )}
            {connId != null && selected != null && bottom === 'data' && (
              <DataGrid key={`${connId}-${selected.db}-${selected.table}`} db={selected.db} table={selected.table} />
            )}
          </div>
          <BottomTabs value={bottom} onChange={setBottom} />
        </main>
      </div>
    </div>
  )
}

const center: CSSProperties = {
  display: 'flex', alignItems: 'center', justifyContent: 'center',
  height: '100%', color: '#999', fontFamily: 'system-ui',
}
