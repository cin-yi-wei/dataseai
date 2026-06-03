import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { Connection, useConnections } from '../store/connections'
import ConnectionDialog from './ConnectionDialog'

interface Props {
  onClose: () => void
}

export default function ConnectionsManager({ onClose }: Props) {
  const list = useConnections((s) => s.list)
  const load = useConnections((s) => s.load)
  const remove = useConnections((s) => s.remove)
  const [editing, setEditing] = useState<Connection | 'new' | null>(null)

  useEffect(() => { void load() }, [load])

  return (
    <main style={{ fontFamily: 'system-ui', padding: 24, maxWidth: 800, margin: '0 auto' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <h1 style={{ margin: 0 }}>connections</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={() => setEditing('new')}>+ new</button>
          <button onClick={onClose}>back</button>
        </div>
      </header>
      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead>
          <tr>
            <th style={th}>name</th>
            <th style={th}>host</th>
            <th style={th}>port</th>
            <th style={th}>user</th>
            <th style={th}>tls</th>
            <th style={th}></th>
          </tr>
        </thead>
        <tbody>
          {list.map((c) => (
            <tr key={c.id}>
              <td style={td}>{c.name}</td>
              <td style={td}>{c.host}</td>
              <td style={td}>{c.port}</td>
              <td style={td}>{c.username}</td>
              <td style={td}>{c.tls}</td>
              <td style={td}>
                <button onClick={() => setEditing(c)}>edit</button>{' '}
                <button onClick={() => { if (confirm(`delete ${c.name}?`)) void remove(c.id) }}>delete</button>
              </td>
            </tr>
          ))}
          {list.length === 0 && (
            <tr><td colSpan={6} style={{ ...td, textAlign: 'center', color: '#999', padding: 24 }}>no connections yet — click + new</td></tr>
          )}
        </tbody>
      </table>
      {editing && (
        <ConnectionDialog
          initial={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={() => void load()}
        />
      )}
    </main>
  )
}

const th: CSSProperties = { textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #ddd', fontSize: 13 }
const td: CSSProperties = { padding: '6px 8px', borderBottom: '1px solid #f3f3f3', fontSize: 13 }
