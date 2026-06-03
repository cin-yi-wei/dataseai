import { useEffect } from 'react'
import { useConnections } from '../store/connections'
import { useActiveConn } from '../store/activeConn'

export default function ConnectionPicker() {
  const list = useConnections((s) => s.list)
  const load = useConnections((s) => s.load)
  const activeId = useActiveConn((s) => s.activeId)
  const setActive = useActiveConn((s) => s.setActive)

  useEffect(() => { void load() }, [load])

  return (
    <select
      value={activeId ?? ''}
      onChange={(e) => setActive(e.target.value === '' ? null : Number(e.target.value))}
      style={{ padding: '4px 6px' }}
    >
      <option value="">— pick connection —</option>
      {list.map((c) => (
        <option key={c.id} value={c.id}>● {c.name}</option>
      ))}
    </select>
  )
}
