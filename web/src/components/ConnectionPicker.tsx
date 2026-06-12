import { useEffect } from 'react'
import { useConnections } from '../store/connections'
import { useActiveConn } from '../store/activeConn'
import { useTabs } from '../store/tabs'

export default function ConnectionPicker() {
  const list = useConnections((s) => s.list)
  const load = useConnections((s) => s.load)
  const activeId = useActiveConn((s) => s.activeId)
  const setActive = useActiveConn((s) => s.setActive)
  const closeAll = useTabs((s) => s.closeAll)

  useEffect(() => { void load() }, [load])

  return (
    <select
      data-connection-picker
      value={activeId ?? ''}
      onChange={(e) => {
        const next = e.target.value === '' ? null : Number(e.target.value)
        // Switching the connection invalidates every open tab (they're scoped
        // to the previous connection). DB switches happen inside Sidebar and
        // leave tabs alone — only this picker triggers the wipe.
        if (next !== activeId) closeAll()
        setActive(next)
      }}
      style={{ padding: '4px 6px', maxWidth: '100%' }}
    >
      <option value="">— pick connection —</option>
      {list.map((c) => (
        <option key={c.id} value={c.id} style={{ color: c.color || undefined }}>
          ● {c.name}{(c.color || '').toLowerCase() === '#ff5b5b' ? ' ⚠' : ''}
        </option>
      ))}
    </select>
  )
}
